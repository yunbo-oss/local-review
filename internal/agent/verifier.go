package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	avgPriceRe  = regexp.MustCompile(`人均\s*[约]?(\d{1,6})`)
	scoreRe     = regexp.MustCompile(`(?:评分|打分)\s*[:：为是]?\s*[⭐*\s]*(\d+(?:\.\d+)?)`)
	addressRe   = regexp.MustCompile(`地址\s*[:：]\s*([^\n；。]+)`)
	openHoursRe = regexp.MustCompile(`(?:营业时间|营业时段)\s*[:：]\s*([0-2]?\d:\d{2}\s*[-–—至到]\s*[0-2]?\d:\d{2})`)
	markdownRe  = regexp.MustCompile(`[*_` + "`" + `]+`)
)

// VerifyOptions 终答校验选项
type VerifyOptions struct {
	// AllowNoResult 明确无结果路径：允许无 [shop:id]
	AllowNoResult       bool
	SemanticEvidenceIDs []int64
}

// VerifyAnswer 成功推荐前校验：引用 ⊆ 证据；非无结果须有引用；强事实不冲突
func VerifyAnswer(answer string, ledger *EvidenceLedger, opts VerifyOptions) error {
	cited := ParseCitedShopIDs(answer)
	citeable := map[int64]struct{}{}
	if ledger != nil {
		for _, id := range ledger.CiteableIDs() {
			citeable[id] = struct{}{}
		}
	}

	if len(cited) == 0 {
		if opts.AllowNoResult || len(citeable) == 0 {
			return nil
		}
		return NewPublicError(ErrGroundingNoCitation, "recommendation must cite at least one [shop:id]")
	}

	for _, id := range cited {
		if _, ok := citeable[id]; !ok {
			return NewPublicError(ErrGroundingUnknownShop, fmt.Sprintf("cited shop %d not in evidence", id))
		}
	}
	if len(opts.SemanticEvidenceIDs) > 0 {
		semantic := map[int64]struct{}{}
		for _, id := range opts.SemanticEvidenceIDs {
			semantic[id] = struct{}{}
		}
		semanticClaims := cited
		if recommended, headerFound := ParseRecommendedShopIDs(answer); headerFound {
			semanticClaims = recommended
		}
		for _, id := range semanticClaims {
			if _, ok := semantic[id]; !ok {
				return NewPublicError(ErrGroundingSemanticUnsupported,
					fmt.Sprintf("recommended shop %d lacks fetched review evidence for the requested semantic preference", id))
			}
		}
	}

	if err := checkFactConflicts(answer, cited, ledger); err != nil {
		return err
	}
	return nil
}

// NeutralizeUnknownCitations treats citation syntax originating from
// untrusted review text as data, not authority. Known ledger citations remain
// untouched; unknown ids are rendered as plain, explicitly unverified text so
// they cannot become clickable recommendations or poison groundedness.
func NeutralizeUnknownCitations(answer string, ledger *EvidenceLedger) string {
	citeable := map[int64]struct{}{}
	if ledger != nil {
		for _, id := range ledger.CiteableIDs() {
			citeable[id] = struct{}{}
		}
	}
	lines := strings.Split(answer, "\n")
	untrustedContext := regexp.MustCompile(`评价|点评|评论|恶意|伪造|注入|不存在的店铺`)
	for i, line := range lines {
		if !untrustedContext.MatchString(line) {
			continue
		}
		lines[i] = shopCiteRe.ReplaceAllStringFunc(line, func(token string) string {
			match := shopCiteRe.FindStringSubmatch(token)
			if len(match) != 2 {
				return token
			}
			id, err := strconv.ParseInt(match[1], 10, 64)
			if err != nil {
				return "未验证店铺ID"
			}
			if _, ok := citeable[id]; ok {
				return token
			}
			return fmt.Sprintf("未验证店铺ID：%d", id)
		})
	}
	return strings.Join(lines, "\n")
}

func checkFactConflicts(answer string, cited []int64, ledger *EvidenceLedger) error {
	if ledger == nil {
		return nil
	}
	// 人均：答案中的每个明确价格都必须出现在被引用店铺的价格证据集合中。
	// 旧实现把第一个价格与所有引用店逐一比较，会误杀“店 A 42 元、店 B
	// 35 元”这类正常多店对比。
	evidencePrices := map[int64]struct{}{}
	for _, id := range cited {
		ev := ledger.Get(id)
		if ev == nil {
			continue
		}
		fv, ok := ev.Fields["avg_price"]
		if !ok {
			continue
		}
		if got, ok := asInt64(fv.Value); ok {
			evidencePrices[got] = struct{}{}
		}
	}
	for _, m := range avgPriceRe.FindAllStringSubmatch(answer, -1) {
		want, _ := strconv.ParseInt(m[1], 10, 64)
		if _, ok := evidencePrices[want]; !ok {
			return NewPublicError(ErrGroundingFactConflict,
				fmt.Sprintf("avg_price conflict: answer=%d not present in cited evidence", want))
		}
	}
	if err := checkScoreConflicts(answer, cited, ledger); err != nil {
		return err
	}
	if err := checkStringFieldConflicts(answer, cited, ledger, "address", addressRe, "address"); err != nil {
		return err
	}
	if err := checkStringFieldConflicts(answer, cited, ledger, "open_hours", openHoursRe, "open_hours"); err != nil {
		return err
	}
	return nil
}

func checkScoreConflicts(answer string, cited []int64, ledger *EvidenceLedger) error {
	evidenceScores := map[string]struct{}{}
	for _, id := range cited {
		ev := ledger.Get(id)
		if ev == nil {
			continue
		}
		fv, ok := ev.Fields["score"]
		if !ok {
			continue
		}
		if raw, ok := asInt64(fv.Value); ok {
			evidenceScores[strconv.FormatFloat(float64(raw)/10, 'f', 1, 64)] = struct{}{}
		}
	}
	for _, m := range scoreRe.FindAllStringSubmatch(answer, -1) {
		claimed, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		key := strconv.FormatFloat(claimed, 'f', 1, 64)
		if _, ok := evidenceScores[key]; !ok {
			return NewPublicError(ErrGroundingFactConflict,
				fmt.Sprintf("score conflict: answer=%s not present in cited evidence", m[1]))
		}
	}
	return nil
}

func checkStringFieldConflicts(answer string, cited []int64, ledger *EvidenceLedger, field string, re *regexp.Regexp, label string) error {
	claims := re.FindAllStringSubmatch(answer, -1)
	if len(claims) == 0 {
		return nil
	}
	evidence := map[string]struct{}{}
	for _, id := range cited {
		ev := ledger.Get(id)
		if ev == nil {
			continue
		}
		if fv, ok := ev.Fields[field]; ok {
			value := normalizeClaim(fmt.Sprint(fv.Value))
			if value != "" {
				evidence[value] = struct{}{}
			}
		}
	}
	for _, m := range claims {
		claim := normalizeClaim(m[1])
		matched := false
		for value := range evidence {
			if claim == value || strings.Contains(claim, value) {
				matched = true
				break
			}
		}
		if !matched {
			return NewPublicError(ErrGroundingFactConflict,
				fmt.Sprintf("%s conflict: answer=%q not present in cited evidence", label, claim))
		}
	}
	return nil
}

func normalizeClaim(value string) string {
	value = shopCiteRe.ReplaceAllString(value, "")
	value = markdownRe.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "–", "-")
	value = strings.ReplaceAll(value, "—", "-")
	value = strings.ReplaceAll(value, "至", "-")
	value = strings.ReplaceAll(value, "到", "-")
	return strings.TrimSpace(value)
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case string:
		x, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return x, err == nil
	default:
		return 0, false
	}
}

// InferAllowNoResult 无候选且无引用 → 视为无结果路径
func InferAllowNoResult(answer string, ledger *EvidenceLedger) bool {
	if len(ParseCitedShopIDs(answer)) > 0 {
		return false
	}
	if ledger == nil || len(ledger.CiteableIDs()) == 0 {
		return true
	}
	return false
}
