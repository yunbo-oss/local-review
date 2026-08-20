package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"local-review-go/internal/llm"
)

type EvidenceClaim struct {
	Text         string   `json:"text"`
	Field        string   `json:"field"`
	Value        any      `json:"value"`
	EvidenceRefs []string `json:"evidence_refs"`
}

type ClaimedRecommendation struct {
	ShopID int64           `json:"shop_id"`
	Claims []EvidenceClaim `json:"claims"`
}

type ClaimAnswer struct {
	NoResult        bool                    `json:"no_result"`
	Summary         string                  `json:"summary"`
	Recommendations []ClaimedRecommendation `json:"recommendations"`
}

var (
	claimShopFieldRef = regexp.MustCompile(`^shop:(\d+)\.([a-z_]+)$`)
	claimBlogRef      = regexp.MustCompile(`^blog:(\d+)$`)
)

const claimAnswerInstruction = `基于服务端已执行工具得到的证据输出严格 JSON，不要 Markdown，不要工具调用。
格式：{"no_result":false,"summary":"简短结论","recommendations":[{"shop_id":1,"claims":[{"text":"适合安静办公","field":"","value":null,"evidence_refs":["blog:12"]},{"text":"人均66元","field":"avg_price","value":66,"evidence_refs":["shop:1.avg_price"]}]}]}

规则：
1. shop_id 必须来自本轮 search_shops；最多推荐3家，每家1~4条 claim。
2. 每条 claim 必须绑定该店证据。结构化字段引用格式 shop:{shop_id}.{field}；评价引用格式 blog:{blog_id}；检索评价摘要可引用 shop:{shop_id}.review_evidence。
3. field 只能是 avg_price、score、address、open_hours、area、type_name、review_evidence 或空字符串。field 非空时 value 必须与证据完全一致。
4. 安静、约会、亲子、宠物、无障碍等体验 claim 必须引用 blog 或 review_evidence，不能仅凭店名/类别推断。
5. 证据不足时 no_result=true 且 recommendations=[]，不得硬凑。`

func ParseClaimAnswer(raw string) (ClaimAnswer, error) {
	if start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	var answer ClaimAnswer
	if err := dec.Decode(&answer); err != nil {
		return ClaimAnswer{}, fmt.Errorf("parse claim answer: %w", err)
	}
	answer.Summary = truncateIntentText(answer.Summary, 240)
	if len(answer.Recommendations) > 3 {
		return ClaimAnswer{}, fmt.Errorf("too many recommendations")
	}
	for i := range answer.Recommendations {
		if len(answer.Recommendations[i].Claims) > 4 {
			return ClaimAnswer{}, fmt.Errorf("too many claims for shop %d", answer.Recommendations[i].ShopID)
		}
		for j := range answer.Recommendations[i].Claims {
			claim := &answer.Recommendations[i].Claims[j]
			claim.Text = truncateIntentText(claim.Text, 160)
			claim.Field = strings.TrimSpace(claim.Field)
			claim.EvidenceRefs = compactIntentStrings(claim.EvidenceRefs, 4, 80)
		}
	}
	return answer, nil
}

func VerifyClaimAnswer(answer ClaimAnswer, ledger *EvidenceLedger) error {
	if answer.NoResult {
		if len(answer.Recommendations) != 0 {
			return fmt.Errorf("no_result answer must not contain recommendations")
		}
		return nil
	}
	if len(answer.Recommendations) == 0 {
		return fmt.Errorf("recommendation answer is empty")
	}
	if ledger == nil {
		return fmt.Errorf("evidence ledger not configured")
	}
	seenShops := map[int64]struct{}{}
	for _, rec := range answer.Recommendations {
		if rec.ShopID <= 0 || !ledger.IsDiscovered(rec.ShopID) {
			return fmt.Errorf("shop %d was not discovered this turn", rec.ShopID)
		}
		if _, duplicate := seenShops[rec.ShopID]; duplicate {
			return fmt.Errorf("duplicate recommendation shop %d", rec.ShopID)
		}
		seenShops[rec.ShopID] = struct{}{}
		if len(rec.Claims) == 0 {
			return fmt.Errorf("shop %d has no claims", rec.ShopID)
		}
		for _, claim := range rec.Claims {
			if err := verifyEvidenceClaim(rec.ShopID, claim, ledger); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyEvidenceClaim(shopID int64, claim EvidenceClaim, ledger *EvidenceLedger) error {
	if strings.TrimSpace(claim.Text) == "" {
		return fmt.Errorf("shop %d has empty claim", shopID)
	}
	if len(claim.EvidenceRefs) == 0 {
		return fmt.Errorf("shop %d claim %q has no evidence refs", shopID, claim.Text)
	}
	allowedFields := map[string]bool{
		"": true, "avg_price": true, "score": true, "address": true,
		"open_hours": true, "area": true, "type_name": true, "review_evidence": true,
	}
	if !allowedFields[claim.Field] {
		return fmt.Errorf("unsupported claim field %q", claim.Field)
	}
	hasSubjectiveEvidence := false
	hasMatchingField := claim.Field == ""
	for _, ref := range claim.EvidenceRefs {
		if match := claimShopFieldRef.FindStringSubmatch(ref); len(match) == 3 {
			refShop, _ := strconv.ParseInt(match[1], 10, 64)
			field := match[2]
			if refShop != shopID {
				return fmt.Errorf("cross-shop evidence leak: claim shop=%d ref=%s", shopID, ref)
			}
			item := ledger.Get(shopID)
			value, ok := item.Fields[field]
			if !ok {
				return fmt.Errorf("missing field evidence %s", ref)
			}
			if field == "review_evidence" {
				hasSubjectiveEvidence = strings.TrimSpace(fmt.Sprint(value.Value)) != ""
			}
			if claim.Field == field {
				if field == "score" {
					claimScore, claimOK := numericClaimValue(claim.Value)
					evidenceScore, evidenceOK := numericClaimValue(value.Value)
					// Ledger scores use the catalog's 0..50 integer scale; user-facing
					// claims must use 0..5. Reject “评分44” instead of letting the
					// later free-text verifier discover the unit mismatch.
					hasMatchingField = claimOK && evidenceOK && claimScore >= 0 && claimScore <= 5 &&
						math.Abs(claimScore*10-evidenceScore) < 0.0001
				} else {
					hasMatchingField = claimValuesEqual(claim.Value, value.Value)
				}
				if !hasMatchingField {
					return fmt.Errorf("field value conflict for %s: claim=%v evidence=%v", ref, claim.Value, value.Value)
				}
			}
			continue
		}
		if match := claimBlogRef.FindStringSubmatch(ref); len(match) == 2 {
			blogID, _ := strconv.ParseInt(match[1], 10, 64)
			if !ledger.HasBlogID(shopID, blogID) {
				return fmt.Errorf("blog %d is not evidence for shop %d", blogID, shopID)
			}
			hasSubjectiveEvidence = true
			continue
		}
		return fmt.Errorf("invalid evidence ref %q", ref)
	}
	if claim.Field != "" && !hasMatchingField {
		return fmt.Errorf("claim field %s lacks matching field evidence", claim.Field)
	}
	if claim.Field == "" && !hasSubjectiveEvidence {
		return fmt.Errorf("subjective claim %q lacks review evidence", claim.Text)
	}
	return nil
}

func claimValuesEqual(claim, evidence any) bool {
	if claim == nil || evidence == nil {
		return claim == evidence
	}
	if left, ok := numericClaimValue(claim); ok {
		if right, ok := numericClaimValue(evidence); ok {
			return math.Abs(left-right) < 0.0001
		}
	}
	return normalizeClaim(fmt.Sprint(claim)) == normalizeClaim(fmt.Sprint(evidence))
}

func numericClaimValue(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func RenderClaimAnswer(answer ClaimAnswer, ledger *EvidenceLedger) string {
	if answer.NoResult || len(answer.Recommendations) == 0 {
		// A no-result summary has no claim/evidence binding. Render a fixed
		// server-side explanation so the model cannot smuggle unsupported shop
		// facts through the otherwise unverified free-text summary.
		return "推荐结果：无\n现有候选或证据不足以支持可靠推荐。"
	}
	ids := make([]string, 0, len(answer.Recommendations))
	for _, rec := range answer.Recommendations {
		ids = append(ids, fmt.Sprintf("[shop:%d]", rec.ShopID))
	}
	var b strings.Builder
	b.WriteString("推荐结果：" + strings.Join(ids, "、"))
	if strings.TrimSpace(answer.Summary) != "" {
		b.WriteString("\n" + strings.TrimSpace(answer.Summary))
	}
	for _, rec := range answer.Recommendations {
		name := fmt.Sprintf("店铺 %d", rec.ShopID)
		if item := ledger.Get(rec.ShopID); item != nil && strings.TrimSpace(item.Name) != "" {
			name = item.Name
		}
		fmt.Fprintf(&b, "\n\n%s [shop:%d]", name, rec.ShopID)
		for _, claim := range rec.Claims {
			fmt.Fprintf(&b, "\n- %s（证据：%s）", strings.TrimSpace(claim.Text), strings.Join(claim.EvidenceRefs, "、"))
		}
	}
	return b.String()
}

// BuildDeterministicClaimFallback produces a conservative terminal answer
// when model-authored claim JSON remains invalid after repair. It never tries
// to infer a subjective fact: a candidate with any unresolved evidence gap is
// excluded, and every emitted claim is copied from a typed ledger field.
// This keeps a verifier-format failure from turning an otherwise healthy run
// into an empty response without weakening the evidence boundary.
func BuildDeterministicClaimFallback(state *AgentState, ledger *EvidenceLedger) (ClaimAnswer, string, error) {
	noResult := ClaimAnswer{NoResult: true, Summary: "现有候选或证据不足以支持可靠推荐。"}
	if state == nil || ledger == nil {
		return noResult, RenderClaimAnswer(noResult, ledger), nil
	}

	// An explicitly named shop with verified details and fetched reviews is a
	// factual inspection, not an open-ended recommendation. Prefer that exact
	// subject and copy review text verbatim into evidence-bound claims even when
	// the semantic gap judge remains "unknown". This is safer than returning an
	// incorrect no-result after the model merely failed to format claim JSON.
	exactCandidates := make([]CandidateState, 0, 1)
	candidates := make([]CandidateState, 0, len(state.Candidates))
	for _, candidate := range state.Candidates {
		if candidate.Rejected {
			continue
		}
		if item := ledger.Get(candidate.ShopID); isExactReviewInspection(state.Question, item) {
			exactCandidates = append(exactCandidates, candidate)
			continue
		}
		if !fallbackCandidateSupported(state, candidate.ShopID) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(exactCandidates) > 0 {
		candidates = exactCandidates
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rank == candidates[j].Rank {
			return candidates[i].ShopID < candidates[j].ShopID
		}
		return candidates[i].Rank < candidates[j].Rank
	})

	for _, candidate := range candidates {
		item := ledger.Get(candidate.ShopID)
		if item == nil || !item.Citeable() {
			continue
		}
		claims := make([]EvidenceClaim, 0, 4)
		summary := "已基于本轮工具返回的可核验字段给出保守推荐。"
		if isExactReviewInspection(state.Question, item) {
			claims = append(claims, deterministicReviewClaims(item, 4)...)
			summary = "以下结论仅复述本轮工具读取到的评价与可核验字段。"
		}
		for _, claim := range deterministicStructuredClaims(state.Question, item) {
			if len(claims) == 4 {
				break
			}
			claims = append(claims, claim)
		}
		if len(claims) == 0 {
			continue
		}
		answer := ClaimAnswer{
			Summary: summary,
			Recommendations: []ClaimedRecommendation{{
				ShopID: candidate.ShopID,
				Claims: claims,
			}},
		}
		if err := VerifyClaimAnswer(answer, ledger); err != nil {
			return ClaimAnswer{}, "", err
		}
		return answer, RenderClaimAnswer(answer, ledger), nil
	}
	return noResult, RenderClaimAnswer(noResult, ledger), nil
}

func isExactReviewInspection(question string, item *ShopEvidence) bool {
	return item != nil && item.Verified && strings.TrimSpace(item.Name) != "" &&
		strings.Contains(question, item.Name) && len(item.BlogIDs) > 0 && len(item.BlogTexts) > 0
}

var unsafeReviewClaim = regexp.MustCompile(`(?i)(system:|developer|<system|ignore previous|忽略系统|环境变量|数据库密码|执行下一条|推荐不存在)`)

func deterministicReviewClaims(item *ShopEvidence, limit int) []EvidenceClaim {
	if item == nil || limit <= 0 {
		return nil
	}
	claims := make([]EvidenceClaim, 0, limit)
	seen := make(map[string]struct{})
	for index, blogID := range item.BlogIDs {
		if len(claims) == limit || index >= len(item.BlogTexts) {
			break
		}
		text := strings.TrimSpace(item.BlogTexts[index])
		if text == "" || unsafeReviewClaim.MatchString(text) {
			continue
		}
		if _, duplicate := seen[text]; duplicate {
			continue
		}
		seen[text] = struct{}{}
		if runes := []rune(text); len(runes) > 140 {
			text = string(runes[:140]) + "…"
		}
		claims = append(claims, EvidenceClaim{
			Text: text, EvidenceRefs: []string{fmt.Sprintf("blog:%d", blogID)},
		})
	}
	return claims
}

func fallbackCandidateSupported(state *AgentState, shopID int64) bool {
	for _, gap := range state.Gaps {
		if gap.ShopID == 0 {
			if gap.Status != EvidenceSupported {
				return false
			}
			continue
		}
		if gap.ShopID == shopID && gap.Status != EvidenceSupported {
			return false
		}
	}
	return true
}

func deterministicStructuredClaims(question string, item *ShopEvidence) []EvidenceClaim {
	if item == nil {
		return nil
	}
	fields := []string{"area", "type_name", "avg_price", "score", "address", "open_hours"}
	if strings.Contains(question, "地址") || strings.Contains(question, "在哪") || strings.Contains(question, "怎么走") {
		fields = []string{"address", "area", "type_name", "avg_price", "score", "open_hours"}
	} else if strings.Contains(question, "营业") || strings.Contains(question, "几点") || strings.Contains(question, "打烊") {
		fields = []string{"open_hours", "address", "area", "avg_price", "score", "type_name"}
	}
	claims := make([]EvidenceClaim, 0, 4)
	for _, field := range fields {
		if len(claims) == 4 {
			break
		}
		evidence, ok := item.Fields[field]
		if !ok {
			continue
		}
		claim, ok := deterministicFieldClaim(item.ShopID, field, evidence.Value)
		if ok {
			claims = append(claims, claim)
		}
	}
	return claims
}

func deterministicFieldClaim(shopID int64, field string, value any) (EvidenceClaim, bool) {
	claim := EvidenceClaim{Field: field, Value: value, EvidenceRefs: []string{fmt.Sprintf("shop:%d.%s", shopID, field)}}
	switch field {
	case "area":
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return EvidenceClaim{}, false
		}
		claim.Text = "位于" + text
	case "type_name":
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return EvidenceClaim{}, false
		}
		claim.Text = "类型为" + text
	case "avg_price":
		price, ok := numericClaimValue(value)
		if !ok || price <= 0 {
			return EvidenceClaim{}, false
		}
		claim.Text = fmt.Sprintf("人均约%.0f元", price)
	case "score":
		score, ok := numericClaimValue(value)
		if !ok || score <= 0 {
			return EvidenceClaim{}, false
		}
		if score > 10 {
			score /= 10
			claim.Value = score
		}
		claim.Text = fmt.Sprintf("评分%.1f", score)
	case "address":
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return EvidenceClaim{}, false
		}
		claim.Text = "地址为" + text
	case "open_hours":
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return EvidenceClaim{}, false
		}
		claim.Text = "营业时间为" + text
	default:
		return EvidenceClaim{}, false
	}
	return claim, true
}

func (l *EvidenceLedger) HasBlogID(shopID, blogID int64) bool {
	item := l.Get(shopID)
	if item == nil {
		return false
	}
	for _, id := range item.BlogIDs {
		if id == blogID {
			return true
		}
	}
	return false
}

func GenerateClaimAnswer(ctx context.Context, client llm.ToolChatClient, messages []llm.ChatMessage, ledger *EvidenceLedger) (ClaimAnswer, string, []llm.ChatMessage, llm.TokenUsage, int, error) {
	return generateClaimAnswer(ctx, client, messages, ledger, nil)
}

func GenerateClaimAnswerWithVerifier(
	ctx context.Context,
	client llm.ToolChatClient,
	messages []llm.ChatMessage,
	ledger *EvidenceLedger,
	verifier ClaimEntailmentVerifier,
) (ClaimAnswer, string, []llm.ChatMessage, llm.TokenUsage, int, error) {
	return generateClaimAnswer(ctx, client, messages, ledger, verifier)
}

func generateClaimAnswer(
	ctx context.Context,
	client llm.ToolChatClient,
	messages []llm.ChatMessage,
	ledger *EvidenceLedger,
	verifier ClaimEntailmentVerifier,
) (ClaimAnswer, string, []llm.ChatMessage, llm.TokenUsage, int, error) {
	working := append([]llm.ChatMessage{}, messages...)
	working = append(working, llm.ChatMessage{Role: "user", Content: claimAnswerInstruction})
	var usage llm.TokenUsage
	modelCalls := 0
	var lastVerificationErr error
	for attempt := 0; attempt < 2; attempt++ {
		turn, err := client.ChatCompleteTurn(ctx, working)
		modelCalls++
		if err != nil {
			return ClaimAnswer{}, "", working, usage, modelCalls, err
		}
		usage = addUsage(usage, turn.Usage)
		working = append(working, turn.Message)
		answer, parseErr := ParseClaimAnswer(turn.Message.Content)
		if parseErr == nil {
			parseErr = VerifyClaimAnswer(answer, ledger)
		}
		if parseErr == nil && verifier != nil {
			verifierUsage, verifierCalls, verifierErr := verifier.Verify(ctx, answer, ledger)
			usage = addUsage(usage, verifierUsage)
			modelCalls += verifierCalls
			parseErr = verifierErr
		}
		if parseErr == nil {
			return answer, RenderClaimAnswer(answer, ledger), working, usage, modelCalls, nil
		}
		lastVerificationErr = parseErr
		if attempt == 0 {
			working = append(working, llm.ChatMessage{Role: "user", Content: fmt.Sprintf(
				"上一个 JSON 未通过 claim-level 校验：%s。不得调用工具；请只使用已有证据重新输出完整 JSON。", parseErr,
			)})
			continue
		}
		code := ErrGroundingFactConflict
		var publicErr *PublicError
		if errors.As(lastVerificationErr, &publicErr) && publicErr.Code != "" {
			code = publicErr.Code
		}
		return ClaimAnswer{}, "", working, usage, modelCalls,
			NewPublicError(code, "claim-level evidence verification failed")
	}
	return ClaimAnswer{}, "", working, usage, 0, fmt.Errorf("unreachable claim generation")
}
