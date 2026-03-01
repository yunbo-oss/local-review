package logic

import (
	"context"
	"fmt"
	"strings"

	"local-review-go/internal/llm"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

const (
	ragTopK       = 5
	ragSystemPrompt = `你是一个大众点评的智能助手。
根据以下检索到的店铺信息，回答用户的问题。请简洁、友好地给出推荐建议。
若检索到的店铺信息不足以回答，可说明并建议用户补充需求。`
)

// RAGLogic RAG 智能点评逻辑
type RAGLogic interface {
	Chat(ctx context.Context, question string, onChunk func(string)) error
	IngestShop(ctx context.Context, shopID int64, name, typeName, area, textContent string, embedding []float32) error
}

// RAGLogicDeps 依赖
type RAGLogicDeps struct {
	EmbeddingClient llm.EmbeddingClient
	ChatClient      llm.ChatClient
	VectorRepo      repoInterfaces.VectorRepo
}

type ragLogic struct {
	embedding llm.EmbeddingClient
	chat      llm.ChatClient
	vector    repoInterfaces.VectorRepo
}

// NewRAGLogic 创建 RAG Logic
func NewRAGLogic(deps RAGLogicDeps) RAGLogic {
	return &ragLogic{
		embedding: deps.EmbeddingClient,
		chat:      deps.ChatClient,
		vector:    deps.VectorRepo,
	}
}

// Chat 用户提问 → 向量检索 → LLM 生成 → 流式输出
func (l *ragLogic) Chat(ctx context.Context, question string, onChunk func(string)) error {
	if l.embedding == nil || l.chat == nil || l.vector == nil {
		return fmt.Errorf("RAG 服务未配置（请设置 LLM_API_KEY）")
	}

	// 1. 问题转向量
	queryVec, err := l.embedding.Embed(ctx, question)
	if err != nil {
		return fmt.Errorf("embedding 问题: %w", err)
	}

	// 2. KNN 检索 TopK 店铺
	shops, err := l.vector.SearchShops(ctx, queryVec, "", ragTopK)
	if err != nil {
		return fmt.Errorf("向量检索: %w", err)
	}
	if len(shops) == 0 {
		onChunk("暂无相关店铺数据，请先执行向量导入（make seed-vector）。")
		return nil
	}

	// 3. 组装上下文
	contextText := buildShopContext(shops)

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

func buildShopContext(shops []repoInterfaces.ShopSearchResult) string {
	var sb strings.Builder
	sb.WriteString("检索到的店铺信息：\n")
	for i, s := range shops {
		if s.TextContent != "" {
			sb.WriteString(fmt.Sprintf("店铺%d：%s\n", i+1, s.TextContent))
		} else {
			sb.WriteString(fmt.Sprintf("店铺%d：%s（%s，%s）\n", i+1, s.Name, s.TypeName, s.Area))
		}
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
