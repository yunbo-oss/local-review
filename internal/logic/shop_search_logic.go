package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"local-review-go/internal/llm"
	"local-review-go/internal/memory"
	"local-review-go/internal/rag"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	defaultHybridCandidateK = 20
	defaultRRFK             = 60
)

// RetrieverStrategy 检索策略
type RetrieverStrategy string

const (
	RetrieverHybrid RetrieverStrategy = "hybrid" // 生产默认
	RetrieverDense  RetrieverStrategy = "dense"  // 仅诊断
)

// SearchMode 混合检索失败策略
type SearchMode string

const (
	SearchModeStrict   SearchMode = "strict"   // 任一路失败 → 整次失败（默认/评测）
	SearchModeDegraded SearchMode = "degraded" // 允许单路回退，必须显式标记 Degraded
)

// SearchOutcome 带降级元数据的检索结果（禁止静默改口径）
type SearchOutcome struct {
	Results        []repoInterfaces.ShopSearchResult
	Strategy       RetrieverStrategy
	Mode           SearchMode
	Degraded       bool
	DegradedReason string
}

// ShopSearchLogic 共享检索入口（线上 RAG / eval / 后续 Agent 共用）
type ShopSearchLogic interface {
	Search(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int) ([]repoInterfaces.ShopSearchResult, error)
	SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int, mode SearchMode) (SearchOutcome, error)
}

// FilterExtractor 从自然语言抽取过滤条件
type FilterExtractor interface {
	Extract(ctx context.Context, question string) (*repoInterfaces.VectorSearchFilter, error)
}

// ShopSearchLogicDeps 依赖
type ShopSearchLogicDeps struct {
	EmbeddingClient llm.EmbeddingClient
	VectorRepo      repoInterfaces.VectorRepo
	CandidateK      int // Hybrid 每路候选数，默认 20
	RRFK            int // RRF k，默认 60
}

type shopSearchLogic struct {
	embedding  llm.EmbeddingClient
	vector     repoInterfaces.VectorRepo
	candidateK int
	rrfK       int
}

// NewShopSearchLogic 创建共享检索 Logic
func NewShopSearchLogic(deps ShopSearchLogicDeps) ShopSearchLogic {
	ck, rk := deps.CandidateK, deps.RRFK
	if ck <= 0 {
		ck = defaultHybridCandidateK
	}
	if rk <= 0 {
		rk = defaultRRFK
	}
	return &shopSearchLogic{
		embedding:  deps.EmbeddingClient,
		vector:     deps.VectorRepo,
		candidateK: ck,
		rrfK:       rk,
	}
}

// DefaultRetrieverStrategy 从环境变量 RAG_RETRIEVER 读取，默认 hybrid
func DefaultRetrieverStrategy() RetrieverStrategy {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RAG_RETRIEVER")))
	switch v {
	case "dense":
		return RetrieverDense
	case "hybrid", "":
		return RetrieverHybrid
	default:
		logrus.Warnf("unknown RAG_RETRIEVER=%q, fallback to hybrid", v)
		return RetrieverHybrid
	}
}

// ParseRetrieverStrategy 解析 CLI/请求中的策略字符串
func ParseRetrieverStrategy(s string) (RetrieverStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hybrid", "":
		return RetrieverHybrid, nil
	case "dense":
		return RetrieverDense, nil
	default:
		return "", fmt.Errorf("unsupported retriever strategy: %q (want hybrid|dense)", s)
	}
}

// ResolveFilter：explicit 非空字段覆盖 extracted；双方皆空则 nil。
func ResolveFilter(explicit, extracted *repoInterfaces.VectorSearchFilter) *repoInterfaces.VectorSearchFilter {
	if explicit == nil && extracted == nil {
		return nil
	}
	base := &repoInterfaces.VectorSearchFilter{}
	if extracted != nil {
		*base = *extracted
	}
	if explicit != nil {
		if explicit.Area != "" {
			base.Area = explicit.Area
		}
		if explicit.TypeName != "" {
			base.TypeName = explicit.TypeName
		}
		if explicit.MaxPrice != 0 {
			base.MaxPrice = explicit.MaxPrice
		}
		if explicit.MinPrice != 0 {
			base.MinPrice = explicit.MinPrice
		}
		if explicit.MinScore != 0 {
			base.MinScore = explicit.MinScore
		}
		if explicit.MinComments != 0 {
			base.MinComments = explicit.MinComments
		}
		if explicit.MaxDistance != 0 {
			base.MaxDistance = explicit.MaxDistance
		}
	}
	if base.Area == "" && base.TypeName == "" && base.MaxPrice == 0 && base.MinPrice == 0 &&
		base.MinScore == 0 && base.MinComments == 0 && base.MaxDistance == 0 {
		return nil
	}
	return base
}

// MergeFilterWithProfile 在已 Resolve 的 filter 上，用 profile 仅补空字段。
// 优先级：显式/抽取（已在 filter 中）> profile 默认 > 无。
func MergeFilterWithProfile(filter *repoInterfaces.VectorSearchFilter, profile memory.Profile) *repoInterfaces.VectorSearchFilter {
	base := &repoInterfaces.VectorSearchFilter{}
	if filter != nil {
		*base = *filter
	}
	if base.Area == "" && len(profile.PreferredAreas) > 0 {
		base.Area = profile.PreferredAreas[0]
	}
	if base.TypeName == "" && len(profile.PreferredTypes) > 0 {
		base.TypeName = profile.PreferredTypes[0]
	}
	if base.MaxPrice == 0 && profile.BudgetMax != nil && *profile.BudgetMax > 0 {
		base.MaxPrice = *profile.BudgetMax
	}
	if base.Area == "" && base.TypeName == "" && base.MaxPrice == 0 && base.MinPrice == 0 &&
		base.MinScore == 0 && base.MinComments == 0 && base.MaxDistance == 0 {
		return nil
	}
	return base
}

// DefaultSearchMode 从 RAG_SEARCH_MODE 读取；默认 strict
func DefaultSearchMode() SearchMode {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("RAG_SEARCH_MODE"))) {
	case "degraded":
		return SearchModeDegraded
	default:
		return SearchModeStrict
	}
}

// Search 在已解析 filter 上检索（strict：与评测口径一致）
func (l *shopSearchLogic) Search(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int) ([]repoInterfaces.ShopSearchResult, error) {
	out, err := l.SearchWithMeta(ctx, query, filter, strategy, topK, SearchModeStrict)
	if err != nil {
		return nil, err
	}
	return out.Results, nil
}

// SearchWithMeta 带 strict/degraded 元数据
func (l *shopSearchLogic) SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy RetrieverStrategy, topK int, mode SearchMode) (SearchOutcome, error) {
	if l.embedding == nil || l.vector == nil {
		return SearchOutcome{}, fmt.Errorf("ShopSearchLogic 未配置 EmbeddingClient/VectorRepo")
	}
	if topK <= 0 {
		topK = 5
	}
	if strategy == "" {
		strategy = DefaultRetrieverStrategy()
	}
	if mode == "" {
		mode = DefaultSearchMode()
	}
	switch strategy {
	case RetrieverDense:
		res, err := l.searchDense(ctx, query, filter, topK)
		return SearchOutcome{Results: res, Strategy: RetrieverDense, Mode: mode}, err
	case RetrieverHybrid:
		return l.searchHybridMeta(ctx, query, filter, topK, mode)
	default:
		return SearchOutcome{}, fmt.Errorf("unsupported strategy: %s", strategy)
	}
}

func (l *shopSearchLogic) searchDense(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, topK int) ([]repoInterfaces.ShopSearchResult, error) {
	queryVec, err := l.embedding.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}
	return l.vector.SearchShops(ctx, queryVec, filter, topK)
}

func (l *shopSearchLogic) searchHybridMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, topK int, mode SearchMode) (SearchOutcome, error) {
	queryVec, err := l.embedding.Embed(ctx, query)
	if err != nil {
		return SearchOutcome{}, fmt.Errorf("embedding: %w", err)
	}

	type arm struct {
		res []repoInterfaces.ShopSearchResult
		err error
	}
	var denseArm, textArm arm
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		denseArm.res, denseArm.err = l.vector.SearchShops(gctx, queryVec, filter, l.candidateK)
		return nil // 错误在 Wait 后按 mode 处理，避免一端失败取消另一端
	})
	g.Go(func() error {
		textArm.res, textArm.err = l.vector.SearchText(gctx, query, filter, l.candidateK)
		return nil
	})
	_ = g.Wait()

	out := SearchOutcome{Strategy: RetrieverHybrid, Mode: mode}
	if denseArm.err != nil && textArm.err != nil {
		return out, fmt.Errorf("hybrid both failed: dense=%v text=%v", denseArm.err, textArm.err)
	}
	if denseArm.err != nil || textArm.err != nil {
		if mode == SearchModeStrict {
			if denseArm.err != nil {
				return out, fmt.Errorf("dense KNN: %w", denseArm.err)
			}
			return out, fmt.Errorf("text search: %w", textArm.err)
		}
		// degraded：单路回退 + 显式标记
		out.Degraded = true
		if denseArm.err != nil {
			out.DegradedReason = "dense_failed:" + denseArm.err.Error()
			out.Results = truncateShopResults(textArm.res, topK)
			return out, nil
		}
		out.DegradedReason = "text_failed:" + textArm.err.Error()
		out.Results = truncateShopResults(denseArm.res, topK)
		return out, nil
	}

	denseIDs := shopIDs(denseArm.res)
	textIDs := shopIDs(textArm.res)
	fused := rag.FuseRRF([][]int64{denseIDs, textIDs}, l.rrfK, topK)
	byID := mergeShopMeta(denseArm.res, textArm.res)
	res := make([]repoInterfaces.ShopSearchResult, 0, len(fused))
	for _, id := range fused {
		if s, ok := byID[id]; ok {
			res = append(res, s)
		}
	}
	out.Results = res
	return out, nil
}

func truncateShopResults(in []repoInterfaces.ShopSearchResult, topK int) []repoInterfaces.ShopSearchResult {
	if topK <= 0 || len(in) <= topK {
		return in
	}
	return in[:topK]
}

func shopIDs(res []repoInterfaces.ShopSearchResult) []int64 {
	ids := make([]int64, len(res))
	for i, r := range res {
		ids[i] = r.ShopID
	}
	return ids
}

func mergeShopMeta(lists ...[]repoInterfaces.ShopSearchResult) map[int64]repoInterfaces.ShopSearchResult {
	m := make(map[int64]repoInterfaces.ShopSearchResult)
	for _, list := range lists {
		for _, s := range list {
			if prev, ok := m[s.ShopID]; ok {
				// 优先保留更完整的元数据
				if s.Name != "" {
					prev.Name = s.Name
				}
				if s.TypeName != "" {
					prev.TypeName = s.TypeName
				}
				if s.Area != "" {
					prev.Area = s.Area
				}
				if s.TextContent != "" {
					prev.TextContent = s.TextContent
				}
				if s.AvgPrice != 0 {
					prev.AvgPrice = s.AvgPrice
				}
				if s.ShopScore != 0 {
					prev.ShopScore = s.ShopScore
				}
				if s.Comments != 0 {
					prev.Comments = s.Comments
				}
				if s.Sold != 0 {
					prev.Sold = s.Sold
				}
				m[s.ShopID] = prev
			} else {
				m[s.ShopID] = s
			}
		}
	}
	return m
}

// --- FilterExtractor 生产实现（LLM） ---

type llmFilterExtractor struct {
	chat llm.ChatClient
}

// NewLLMFilterExtractor 创建与线上一致的 LLM filter 抽取器
func NewLLMFilterExtractor(chat llm.ChatClient) FilterExtractor {
	return &llmFilterExtractor{chat: chat}
}

func (e *llmFilterExtractor) Extract(ctx context.Context, question string) (*repoInterfaces.VectorSearchFilter, error) {
	if e.chat == nil {
		return nil, fmt.Errorf("ChatClient 未配置")
	}
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: ragFilterExtractPrompt},
		{Role: openai.ChatMessageRoleUser, Content: "用户问题：" + question},
	}
	resp, err := e.chat.ChatComplete(ctx, messages)
	if err != nil {
		return nil, err
	}
	return parseFilterFromJSON(resp), nil
}

func parseFilterFromJSON(s string) *repoInterfaces.VectorSearchFilter {
	s = strings.TrimSpace(s)
	if m := regexp.MustCompile("(?s)```(?:json)?\\s*([^`]+)```").FindStringSubmatch(s); len(m) > 1 {
		s = strings.TrimSpace(m[1])
	}
	var v struct {
		Area        string `json:"area"`
		TypeName    string `json:"typeName"`
		MaxPrice    int64  `json:"maxPrice"`
		MinPrice    int64  `json:"minPrice"`
		MinScore    int    `json:"minScore"`
		MinComments int    `json:"minComments"`
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		logrus.Warnf("解析 filter JSON 失败: %v, raw: %s", err, s)
		return nil
	}
	f := &repoInterfaces.VectorSearchFilter{
		Area:        strings.TrimSpace(v.Area),
		TypeName:    strings.TrimSpace(v.TypeName),
		MaxPrice:    v.MaxPrice,
		MinPrice:    v.MinPrice,
		MinScore:    v.MinScore,
		MinComments: v.MinComments,
	}
	if f.Area == "" && f.TypeName == "" && f.MaxPrice == 0 && f.MinPrice == 0 && f.MinScore == 0 && f.MinComments == 0 {
		return nil
	}
	return f
}
