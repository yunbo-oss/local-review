package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

const (
	ragTopK         = 5
	ragBlogLimit    = 3 // 每个店铺最多取几条探店笔记加入上下文
	ragSystemPrompt = `你是一个大众点评的智能助手。
根据以下检索到的店铺信息及用户点评，回答用户的问题。请简洁、友好地给出推荐建议。
若检索到的店铺信息不足以回答，可说明并建议用户补充需求。`

	// filter 提取的 system prompt
	ragFilterExtractPrompt = `你是一个意图解析助手。从用户的店铺检索问题中提取结构化过滤条件，输出 JSON。
可选区域：朝阳区、海淀区、西城区、东城区、丰台区（用户提到其他区域时用最接近的或留空）。
可选类型：美食、咖啡、酒店（用户说火锅、川菜、咖啡厅等时映射到对应类型）。
人均价格：用户说「人均100」「100以内」「不超过200」等时提取为 maxPrice；「人均50以上」为 minPrice。
评分：用户要求「评分高的」「4星以上」等可设为 minScore（满分50，45 约等于 4.5 星）。
仅输出 JSON，不要其他文字。未提及的字段填 0 或空字符串。
格式：{"area":"","typeName":"","maxPrice":0,"minPrice":0,"minScore":0,"minComments":0}`
)

// RAGLogic RAG 智能点评逻辑
type RAGLogic interface {
	Chat(ctx context.Context, question string, onChunk func(string)) error
	ChatWithFilter(ctx context.Context, question string, filter *repoInterfaces.VectorSearchFilter, onChunk func(string)) error
	IngestShop(ctx context.Context, shopID int64, name, typeName, area, textContent string, embedding []float32) error
}

// RAGLogicDeps 依赖
type RAGLogicDeps struct {
	EmbeddingClient llm.EmbeddingClient
	ChatClient      llm.ChatClient
	VectorRepo      repoInterfaces.VectorRepo
	BlogRepo        repoInterfaces.BlogRepo // 可选：用于获取店铺探店笔记
}

type ragLogic struct {
	embedding llm.EmbeddingClient
	chat      llm.ChatClient
	vector    repoInterfaces.VectorRepo
	blog      repoInterfaces.BlogRepo
}

// NewRAGLogic 创建 RAG Logic
func NewRAGLogic(deps RAGLogicDeps) RAGLogic {
	return &ragLogic{
		embedding: deps.EmbeddingClient,
		chat:      deps.ChatClient,
		vector:    deps.VectorRepo,
		blog:      deps.BlogRepo,
	}
}

// Chat 用户提问 → 向量检索 → LLM 生成 → 流式输出（无过滤）
func (l *ragLogic) Chat(ctx context.Context, question string, onChunk func(string)) error {
	return l.ChatWithFilter(ctx, question, nil, onChunk)
}

// ChatWithFilter 带预过滤的 RAG 对话（Filtered Vector Search）
// 若 filter 为 nil，则通过 LLM 从用户提问中自动提取过滤条件
func (l *ragLogic) ChatWithFilter(ctx context.Context, question string, filter *repoInterfaces.VectorSearchFilter, onChunk func(string)) error {
	if l.embedding == nil || l.chat == nil || l.vector == nil {
		return fmt.Errorf("RAG 服务未配置（请设置 LLM_API_KEY）")
	}

	// 0. 若未传入 filter，用 LLM 从提问中提取
	if filter == nil {
		filter = l.extractFilterFromQuestion(ctx, question)
	}

	// 1. 问题转向量
	queryVec, err := l.embedding.Embed(ctx, question)
	if err != nil {
		return fmt.Errorf("embedding 问题: %w", err)
	}

	// 2. 带预过滤的 KNN 检索 TopK 店铺
	shops, err := l.vector.SearchShops(ctx, queryVec, filter, ragTopK)
	if err != nil {
		return fmt.Errorf("向量检索: %w", err)
	}
	if len(shops) == 0 {
		onChunk("暂无相关店铺数据，请先执行向量导入（make seed-vector）。")
		return nil
	}

	// 3. 组装上下文（含店铺基本信息 + 用户探店笔记）
	contextText := l.buildShopContext(ctx, shops)

	// 4. 组装 Prompt
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: ragSystemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: contextText + "\n\n用户问题：" + question + "\n\n请根据以上店铺信息回答："},
	}

	// 5. 流式调用 LLM
	if err := l.chat.ChatStream(ctx, messages, onChunk); err != nil {
		logrus.Errorf("RAG ChatStream 失败: %v", err)
		return fmt.Errorf("生成回答: %w", err)
	}
	return nil
}

// extractFilterFromQuestion 用 LLM 从用户提问中提取过滤条件
func (l *ragLogic) extractFilterFromQuestion(ctx context.Context, question string) *repoInterfaces.VectorSearchFilter {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: ragFilterExtractPrompt},
		{Role: openai.ChatMessageRoleUser, Content: "用户问题：" + question},
	}
	resp, err := l.chat.ChatComplete(ctx, messages)
	if err != nil {
		logrus.Warnf("LLM 提取 filter 失败，将不过滤: %v", err)
		return nil
	}
	return parseFilterFromJSON(resp)
}

// parseFilterFromJSON 解析 LLM 返回的 JSON 为 VectorSearchFilter
func parseFilterFromJSON(s string) *repoInterfaces.VectorSearchFilter {
	s = strings.TrimSpace(s)
	// 去除 markdown 代码块
	if m := regexp.MustCompile("(?s)```(?:json)?\\s*([^`]+)```").FindStringSubmatch(s); len(m) > 1 {
		s = strings.TrimSpace(m[1])
	}
	var v struct {
		Area        string  `json:"area"`
		TypeName    string  `json:"typeName"`
		MaxPrice    int64   `json:"maxPrice"`
		MinPrice    int64   `json:"minPrice"`
		MinScore    int     `json:"minScore"`
		MinComments int     `json:"minComments"`
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
	// 全空则返回 nil
	if f.Area == "" && f.TypeName == "" && f.MaxPrice == 0 && f.MinPrice == 0 && f.MinScore == 0 && f.MinComments == 0 {
		return nil
	}
	return f
}

// buildShopContext 组装 RAG 上下文：店铺基本信息 + 该店铺的用户探店笔记（Blog）
func (l *ragLogic) buildShopContext(ctx context.Context, shops []repoInterfaces.ShopSearchResult) string {
	var sb strings.Builder
	sb.WriteString("检索到的店铺信息：\n")
	for i, s := range shops {
		if s.TextContent != "" {
			sb.WriteString(fmt.Sprintf("店铺%d：%s", i+1, s.TextContent))
		} else {
			sb.WriteString(fmt.Sprintf("店铺%d：%s（%s，%s）", i+1, s.Name, s.TypeName, s.Area))
		}
		// 附加该店铺的用户探店笔记（点评）
		if l.blog != nil {
			blogs, err := l.blog.ListByShopID(ctx, s.ShopID, ragBlogLimit)
			if err == nil && len(blogs) > 0 {
				sb.WriteString("；用户点评：")
				for j, b := range blogs {
					if j > 0 {
						sb.WriteString(" | ")
					}
					content := strings.TrimSpace(b.Content)
					if len(content) > 100 {
						content = content[:100] + "..."
					}
					if b.Title != "" {
						sb.WriteString(fmt.Sprintf("[%s] %s", b.Title, content))
					} else {
						sb.WriteString(content)
					}
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// IngestShop 存储单个店铺向量（供离线任务或 MQ 消费者调用）
func (l *ragLogic) IngestShop(ctx context.Context, shopID int64, name, typeName, area, textContent string, embedding []float32) error {
	if l.vector == nil {
		return fmt.Errorf("VectorRepo 未配置")
	}
	return l.vector.StoreShop(ctx, &repoInterfaces.ShopVectorDoc{
		ShopID:      shopID,
		Name:        name,
		TypeName:    typeName,
		Area:        area,
		TextContent: textContent,
		Embedding:   embedding,
	})
}
