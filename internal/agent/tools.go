package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"local-review-go/internal/llm"
	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"

	"go.opentelemetry.io/otel/codes"
)

const (
	ToolSearchShops   = "search_shops"
	ToolGetShop       = "get_shop"
	ToolListShopBlogs = "list_shop_blogs"
)

// ToolStatus 供 SSE status 事件
type ToolStatus string

const (
	StatusSearching    ToolStatus = "searching"
	StatusReadingShop  ToolStatus = "reading_shop"
	StatusReadingBlogs ToolStatus = "reading_blogs"
)

// ShopHit 工具返回的精简店铺
type ShopHit struct {
	ShopID         int64
	Name           string
	Area           string
	TypeName       string
	AvgPrice       int64
	Score          int
	ReviewEvidence string
}

// ShopSearcher 窄接口，避免 agent→logic 循环依赖
type ShopSearcher interface {
	SearchShops(ctx context.Context, query, area, typeName string, maxPrice, minPrice *int64, topK int) ([]ShopHit, error)
}

// ToolExecutor 执行领域工具
type ToolExecutor struct {
	mu       sync.Mutex
	statusMu sync.Mutex
	Search   ShopSearcher
	ShopRepo repoInterfaces.ShopRepo
	BlogRepo repoInterfaces.BlogRepo
	MaxChars int
	OnStatus func(ToolStatus)
	Ledger   *EvidenceLedger
	// RequiredSemantics are derived deterministically from the original user
	// question and must be backed by fetched review text before recommendation.
	RequiredSemantics []string
	// RequiredTools is a deterministic minimum evidence plan. The loop gets one
	// bounded chance to request any missing tool before accepting a final answer.
	RequiredTools []string
	// TargetShopName is populated for explicit-name lookups so deterministic
	// evidence prefetch never chooses a semantically similar neighbour.
	TargetShopName string
	// FactualLookup distinguishes an explicitly named shop being inspected from
	// a shop being recommended. Its citations are evidence, not recommendations.
	FactualLookup  bool
	CandidateOrder []int64
	// Observed 与 Ledger 可引用集同步（兼容旧 groundedness / graders）
	Observed map[int64]struct{}
}

// ToolDefinitions OpenAI-compatible schemas
func ToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        ToolSearchShops,
			Description: "按条件搜索候选店铺（Hybrid 检索）",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"area":{"type":"string"},"type_name":{"type":"string"},"max_price":{"type":"integer"},"min_price":{"type":"integer"}},"required":["query"],"additionalProperties":false}`),
		},
		{
			Name:        ToolGetShop,
			Description: "读取单店权威结构化详情（地址、价格、营业时间）；不能证明安静、无障碍、宠物友好等体验",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"shop_id":{"type":"integer"}},"required":["shop_id"],"additionalProperties":false}`),
		},
		{
			Name:        ToolListShopBlogs,
			Description: "列出店铺真实评价；判断安静办公、约会、亲子、宠物友好、无障碍、商务宴请、深夜体验等语义适配性的必需工具",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"shop_id":{"type":"integer"},"limit":{"type":"integer"},"cursor":{"type":"string"},"sort":{"type":"string","enum":["liked","recent"]},"freshness_days":{"type":"integer","minimum":0,"maximum":3650}},"required":["shop_id"],"additionalProperties":false}`),
		},
	}
}

// ExecuteStructured is the V2 tool boundary. It preserves failures as typed
// ToolResult values so the runtime can retry, replan and audit them reliably.
func (e *ToolExecutor) ExecuteStructured(ctx context.Context, name, argsJSON string) (result ToolResult) {
	started := time.Now()
	result = ToolResult{
		Tool: name, ArgsHash: toolArgsHash(argsJSON), Status: ActionRunning,
	}
	ctx, span := StartToolSpan(ctx, name)
	defer func() {
		result.LatencyMs = time.Since(started).Milliseconds()
		span.End()
	}()
	if e.Ledger == nil {
		e.Ledger = NewEvidenceLedger()
	}
	if e.Observed == nil {
		e.Observed = map[int64]struct{}{}
	}
	maxChars := e.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxToolResultChars
	}

	var out string
	var err error
	switch name {
	case ToolSearchShops:
		e.emitStatus(StatusSearching)
		out, err = e.execSearch(ctx, argsJSON)
	case ToolGetShop:
		e.emitStatus(StatusReadingShop)
		out, err = e.execGetShop(ctx, argsJSON)
	case ToolListShopBlogs:
		e.emitStatus(StatusReadingBlogs)
		out, err = e.execListBlogs(ctx, argsJSON)
	default:
		err = fmt.Errorf("unknown tool: %s", name)
	}
	if err != nil {
		result.Status = ActionFailed
		result.ErrorCode = classifyToolError(err)
		result.ErrorDetail = err.Error()
		span.RecordError(err)
		span.SetStatus(codes.Error, result.ErrorCode)
		return result
	}
	e.syncObservedFromLedger()
	if outputCode, outputDetail := structuredOutputError(out); outputCode != "" {
		result.Status = ActionFailed
		result.ErrorCode = outputCode
		result.ErrorDetail = outputDetail
		span.SetStatus(codes.Error, outputCode)
		return result
	}
	result.Status = ActionSucceeded
	result.ResultCount = toolResultCount(name, out)
	if name == ToolSearchShops {
		result.CandidateIDs = e.CandidateIDs()
	} else if name == ToolListShopBlogs {
		result.NextCursor = toolResultNextCursor(out)
	}
	result.Output = truncateUTF8(out, maxChars)
	return result
}

// Execute keeps the V1 string/error contract while delegating all new code to
// ExecuteStructured. V1 intentionally receives an error JSON for known tools.
func (e *ToolExecutor) Execute(ctx context.Context, name, argsJSON string) (string, error) {
	result := e.ExecuteStructured(ctx, name, argsJSON)
	if result.Status == ActionSucceeded {
		return result.Output, nil
	}
	if result.ErrorCode == ErrToolUnknown {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	detail := result.ErrorDetail
	if detail == "" {
		detail = result.ErrorCode
	}
	return fmt.Sprintf(`{"error":%q}`, detail), nil
}

func toolArgsHash(argsJSON string) string {
	sum := sha256.Sum256([]byte(CanonicalArgs(argsJSON)))
	return fmt.Sprintf("%x", sum[:])
}

func classifyToolError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrToolTimeout
	}
	if errors.Is(err, context.Canceled) {
		return "tool_cancelled"
	}
	var public *PublicError
	if errors.As(err, &public) && public.Code != "" {
		return public.Code
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "unknown tool"):
		return ErrToolUnknown
	case strings.Contains(message, "invalid args"), strings.Contains(message, "required"),
		strings.Contains(message, "must be"):
		return ErrToolInvalidArgs
	default:
		return ErrToolExecution
	}
}

func structuredOutputError(raw string) (string, string) {
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil || strings.TrimSpace(payload.Error) == "" {
		return "", ""
	}
	if payload.Error == "not_found" {
		return ErrToolNotFound, payload.Error
	}
	return ErrToolExecution, payload.Error
}

func toolResultCount(name, raw string) int {
	switch name {
	case ToolSearchShops:
		var items []json.RawMessage
		if json.Unmarshal([]byte(raw), &items) == nil {
			return len(items)
		}
	case ToolGetShop:
		var item map[string]any
		if json.Unmarshal([]byte(raw), &item) == nil && len(item) > 0 {
			return 1
		}
	case ToolListShopBlogs:
		var payload struct {
			Blogs []json.RawMessage `json:"blogs"`
		}
		if json.Unmarshal([]byte(raw), &payload) == nil {
			return len(payload.Blogs)
		}
	}
	return 0
}

func toolResultNextCursor(raw string) string {
	var payload struct {
		NextCursor string `json:"next_cursor"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.NextCursor)
}

func (e *ToolExecutor) emitStatus(status ToolStatus) {
	if e == nil || e.OnStatus == nil {
		return
	}
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	e.OnStatus(status)
}

// truncateUTF8 按 rune 截断，避免切断多字节字符
func truncateUTF8(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…[truncated]"
}

func (e *ToolExecutor) syncObservedFromLedger() {
	if e.Ledger == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Observed == nil {
		e.Observed = map[int64]struct{}{}
	}
	for _, id := range e.Ledger.CiteableIDs() {
		e.Observed[id] = struct{}{}
	}
}

func (e *ToolExecutor) CandidateIDs() []int64 {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int64(nil), e.CandidateOrder...)
}

func (e *ToolExecutor) EvidenceSnapshot() EvidenceSnapshot {
	if e == nil || e.Ledger == nil {
		return EvidenceSnapshot{Shops: map[int64]ShopEvidenceSnapshot{}}
	}
	return e.Ledger.Snapshot()
}

type searchArgs struct {
	Query    string `json:"query"`
	Area     string `json:"area"`
	TypeName string `json:"type_name"`
	MaxPrice *int64 `json:"max_price"`
	MinPrice *int64 `json:"min_price"`
}

func (e *ToolExecutor) execSearch(ctx context.Context, argsJSON string) (string, error) {
	var a searchArgs
	if err := strictDecode(argsJSON, &a); err != nil {
		return "", err
	}
	if strings.TrimSpace(a.Query) == "" {
		return "", fmt.Errorf("query required")
	}
	if a.MaxPrice != nil && *a.MaxPrice < 0 {
		return "", fmt.Errorf("max_price must be >= 0")
	}
	if a.MinPrice != nil && *a.MinPrice < 0 {
		return "", fmt.Errorf("min_price must be >= 0")
	}
	if a.MinPrice != nil && a.MaxPrice != nil && *a.MinPrice > *a.MaxPrice {
		return "[]", nil
	}
	if e.Search == nil {
		return "", fmt.Errorf("search not configured")
	}
	results, err := e.Search.SearchShops(ctx, a.Query, a.Area, a.TypeName, a.MaxPrice, a.MinPrice, 5)
	if err != nil {
		return "", err
	}
	type item struct {
		ShopID                  int64   `json:"shop_id"`
		Name                    string  `json:"name"`
		Area                    string  `json:"area"`
		TypeName                string  `json:"type_name"`
		AvgPrice                int64   `json:"avg_price"`
		Score                   float64 `json:"score"`
		UntrustedReviewEvidence string  `json:"untrusted_review_evidence,omitempty"`
	}
	items := make([]item, 0, len(results))
	e.mu.Lock()
	e.CandidateOrder = e.CandidateOrder[:0]
	for _, r := range results {
		e.CandidateOrder = append(e.CandidateOrder, r.ShopID)
	}
	e.mu.Unlock()
	for _, r := range results {
		e.Ledger.DiscoverFromSearch(r.ShopID, r.Name, map[string]any{
			"avg_price": r.AvgPrice, "score": r.Score, "area": r.Area, "type_name": r.TypeName,
			"review_evidence": r.ReviewEvidence,
		})
		items = append(items, item{
			ShopID: r.ShopID, Name: r.Name, Area: r.Area,
			TypeName: r.TypeName, AvgPrice: r.AvgPrice, Score: float64(r.Score) / 10,
			UntrustedReviewEvidence: truncateUTF8(r.ReviewEvidence, 500),
		})
	}
	b, _ := json.Marshal(items)
	return string(b), nil
}

type idArgs struct {
	ShopID int64 `json:"shop_id"`
	Limit  *int  `json:"limit"`
}

type reviewArgs struct {
	ShopID        int64  `json:"shop_id"`
	Limit         *int   `json:"limit"`
	Cursor        string `json:"cursor"`
	Sort          string `json:"sort"`
	FreshnessDays int    `json:"freshness_days"`
}

func (e *ToolExecutor) execGetShop(ctx context.Context, argsJSON string) (string, error) {
	var a idArgs
	if err := strictDecode(argsJSON, &a); err != nil {
		return "", err
	}
	if a.ShopID <= 0 {
		return "", fmt.Errorf("shop_id must be > 0")
	}
	if !e.Ledger.IsDiscovered(a.ShopID) {
		return "", NewPublicError(ErrToolNotAllowed, "shop_id not in this turn candidates")
	}
	if e.ShopRepo == nil {
		return "", fmt.Errorf("shop repo not configured")
	}
	shop, err := e.ShopRepo.GetByID(ctx, a.ShopID)
	if err != nil || shop == nil {
		return `{"error":"not_found"}`, nil
	}
	_ = e.Ledger.VerifyFromGetShop(a.ShopID, shop.Name, map[string]any{
		"avg_price": shop.AvgPrice, "score": shop.Score,
		"address": shop.Address, "open_hours": shop.OpenHours, "area": shop.Area,
	})
	b, _ := json.Marshal(map[string]any{
		"shop_id": shop.Id, "name": shop.Name, "area": shop.Area,
		"avg_price": shop.AvgPrice, "score": float64(shop.Score) / 10, "address": shop.Address,
		"open_hours": shop.OpenHours,
	})
	return string(b), nil
}

func (e *ToolExecutor) execListBlogs(ctx context.Context, argsJSON string) (string, error) {
	var a reviewArgs
	if err := strictDecode(argsJSON, &a); err != nil {
		return "", err
	}
	if a.ShopID <= 0 {
		return "", fmt.Errorf("shop_id must be > 0")
	}
	limit := 5
	if a.Limit != nil {
		limit = *a.Limit
	}
	if limit <= 0 || limit > 10 {
		return "", fmt.Errorf("limit must be 1..10")
	}
	if e.BlogRepo == nil {
		return "", fmt.Errorf("blog repo not configured")
	}
	if !e.Ledger.IsDiscovered(a.ShopID) {
		// 防空 blogs 洗白：未发现店铺不得通过 list_blogs 获得 citeable
		return "", NewPublicError(ErrToolNotAllowed, "shop_id not in this turn candidates")
	}
	sortMode := strings.ToLower(strings.TrimSpace(a.Sort))
	if sortMode == "" {
		sortMode = "liked"
	}
	if sortMode != "liked" && sortMode != "recent" {
		return "", fmt.Errorf("sort must be liked or recent")
	}
	if a.FreshnessDays < 0 || a.FreshnessDays > 3650 {
		return "", fmt.Errorf("freshness_days must be 0..3650")
	}
	var blogs []model.Blog
	var nextCursor string
	if paged, ok := e.BlogRepo.(repoInterfaces.PaginatedBlogRepo); ok {
		request := repoInterfaces.BlogPageRequest{
			ShopID: a.ShopID, Cursor: strings.TrimSpace(a.Cursor), Limit: limit, Sort: sortMode,
		}
		if a.FreshnessDays > 0 {
			request.FreshAfter = time.Now().AddDate(0, 0, -a.FreshnessDays)
		}
		page, err := paged.ListByShopIDPage(ctx, request)
		if err != nil {
			return "", err
		}
		blogs, nextCursor = page.Blogs, page.NextCursor
	} else {
		if strings.TrimSpace(a.Cursor) != "" {
			return "", fmt.Errorf("review pagination not supported")
		}
		var err error
		blogs, err = e.BlogRepo.ListByShopID(ctx, a.ShopID, limit)
		if err != nil {
			return "", err
		}
	}
	ids := make([]int64, 0, len(blogs))
	texts := make([]string, 0, len(blogs))
	type bi struct {
		BlogID  int64  `json:"blog_id"`
		Title   string `json:"title"`
		Snippet string `json:"content_snippet"`
		Liked   int    `json:"liked"`
	}
	items := make([]bi, 0, len(blogs))
	for _, b := range blogs {
		ids = append(ids, b.Id)
		texts = append(texts, truncateUTF8(strings.TrimSpace(b.Title)+" "+strings.TrimSpace(b.Content), 600))
		snip := strings.TrimSpace(b.Content)
		if len([]rune(snip)) > 120 {
			snip = string([]rune(snip)[:120]) + "…"
		}
		items = append(items, bi{BlogID: b.Id, Title: b.Title, Snippet: snip, Liked: b.Liked})
	}
	_ = e.Ledger.RecordBlogEvidence(a.ShopID, ids, texts)
	// content_snippet 为用户生成内容：标注 untrusted，配合 system policy 防注入
	payload := map[string]any{
		"shop_id":        a.ShopID,
		"untrusted_data": true,
		"sort":           sortMode,
		"next_cursor":    nextCursor,
		"blogs":          items,
	}
	raw, _ := json.Marshal(payload)
	return string(raw), nil
}

func strictDecode(raw string, dst any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}
	// 拒绝尾随第二个 JSON 值
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid args: trailing JSON after first value")
		}
		// 非 EOF 的残留 token 也视为非法
		if err != io.EOF {
			return fmt.Errorf("invalid args: trailing content after JSON object")
		}
	}
	return nil
}

// CanonicalArgs 稳定 JSON（用于去重）
func CanonicalArgs(argsJSON string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return strings.TrimSpace(argsJSON)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return strings.TrimSpace(argsJSON)
	}
	return string(b)
}
