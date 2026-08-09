package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"
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
	SearchShops(ctx context.Context, query, area, typeName string, maxPrice *int64, topK int) ([]ShopHit, error)
}

// ToolExecutor 执行领域工具
type ToolExecutor struct {
	Search   ShopSearcher
	ShopRepo repoInterfaces.ShopRepo
	BlogRepo repoInterfaces.BlogRepo
	MaxChars int
	OnStatus func(ToolStatus)
	Ledger   *EvidenceLedger
	// RequiredSemantics are derived deterministically from the original user
	// question and must be backed by fetched review text before recommendation.
	RequiredSemantics []string
	// Observed 与 Ledger 可引用集同步（兼容旧 groundedness / graders）
	Observed map[int64]struct{}
}

// ToolDefinitions OpenAI-compatible schemas
func ToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        ToolSearchShops,
			Description: "按条件搜索候选店铺（Hybrid 检索）",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"area":{"type":"string"},"type_name":{"type":"string"},"max_price":{"type":"integer"}},"required":["query"],"additionalProperties":false}`),
		},
		{
			Name:        ToolGetShop,
			Description: "读取单店权威结构化详情（地址、价格、营业时间）；不能证明安静、无障碍、宠物友好等体验",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"shop_id":{"type":"integer"}},"required":["shop_id"],"additionalProperties":false}`),
		},
		{
			Name:        ToolListShopBlogs,
			Description: "列出店铺真实评价；判断安静办公、约会、亲子、宠物友好、无障碍、商务宴请、深夜体验等语义适配性的必需工具",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"shop_id":{"type":"integer"},"limit":{"type":"integer"}},"required":["shop_id"],"additionalProperties":false}`),
		},
	}
}

// Execute 校验并执行工具；结果截断；更新 Ledger/Observed
func (e *ToolExecutor) Execute(ctx context.Context, name, argsJSON string) (string, error) {
	ctx, span := StartToolSpan(ctx, name)
	defer span.End()
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
		if e.OnStatus != nil {
			e.OnStatus(StatusSearching)
		}
		out, err = e.execSearch(ctx, argsJSON)
	case ToolGetShop:
		if e.OnStatus != nil {
			e.OnStatus(StatusReadingShop)
		}
		out, err = e.execGetShop(ctx, argsJSON)
	case ToolListShopBlogs:
		if e.OnStatus != nil {
			e.OnStatus(StatusReadingBlogs)
		}
		out, err = e.execListBlogs(ctx, argsJSON)
	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil
	}
	e.syncObservedFromLedger()
	out = truncateUTF8(out, maxChars)
	return out, nil
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
	if e.Observed == nil {
		e.Observed = map[int64]struct{}{}
	}
	for _, id := range e.Ledger.CiteableIDs() {
		e.Observed[id] = struct{}{}
	}
}

type searchArgs struct {
	Query    string `json:"query"`
	Area     string `json:"area"`
	TypeName string `json:"type_name"`
	MaxPrice *int64 `json:"max_price"`
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
	if e.Search == nil {
		return "", fmt.Errorf("search not configured")
	}
	results, err := e.Search.SearchShops(ctx, a.Query, a.Area, a.TypeName, a.MaxPrice, 5)
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
	var a idArgs
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
	blogs, err := e.BlogRepo.ListByShopID(ctx, a.ShopID, limit)
	if err != nil {
		return "", err
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
		texts = append(texts, strings.TrimSpace(b.Title)+" "+strings.TrimSpace(b.Content))
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
