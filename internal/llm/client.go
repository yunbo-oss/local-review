package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

const (
	// DeepSeek 官方提供 Chat/Tool Calls，不提供本项目所需的 Embeddings。
	// 默认用确定性的本地 feature-hash 向量，只把 API 用于 Chat/Agent。
	defaultEmbeddingProvider = "local"
	defaultEmbeddingModel    = "local-feature-hash-zh-v2"
	defaultEmbeddingDim      = 384
	defaultChatModel         = "deepseek-v4-flash"
	defaultBaseURL           = "https://api.deepseek.com"
)

// EmbeddingClient 文本向量化接口
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// ChatClient 对话接口（支持流式 + 非流式）
type ChatClient interface {
	ChatStream(ctx context.Context, messages []openai.ChatCompletionMessage, onChunk func(string)) error
	// ChatComplete 非流式完成，返回完整回复（用于 filter 提取等需要结构化输出的场景）
	ChatComplete(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error)
	ChatCompleteWithUsage(ctx context.Context, messages []openai.ChatCompletionMessage) (string, TokenUsage, error)
}

// Config 从环境变量读取 LLM 配置
type Config struct {
	BaseURL               string
	APIKey                string
	EmbeddingProvider     string
	EmbeddingModel        string
	ChatModel             string
	ThinkingMode          string
	EmbeddingDim          int
	TLSInsecureSkipVerify bool // 跳过 TLS 证书校验（仅开发/调试，生产慎用）
}

func LoadConfig() Config {
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	apiKey := os.Getenv("LLM_API_KEY")
	embProvider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_EMBEDDING_PROVIDER")))
	if embProvider == "" {
		embProvider = defaultEmbeddingProvider
	}
	embModel := os.Getenv("LLM_EMBEDDING_MODEL")
	if embModel == "" {
		if embProvider == "local" {
			embModel = defaultEmbeddingModel
		} else {
			embModel = "text-embedding-3-small"
		}
	}
	chatModel := os.Getenv("LLM_CHAT_MODEL")
	if chatModel == "" {
		chatModel = defaultChatModel
	}
	thinkingMode := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_THINKING_MODE")))
	if thinkingMode == "" {
		thinkingMode = "disabled"
	}
	dim := defaultEmbeddingDim
	if d := os.Getenv("LLM_EMBEDDING_DIM"); d != "" {
		if n, err := fmt.Sscanf(d, "%d", &dim); err == nil && n == 1 {
			// ok
		}
	}
	tlsSkip := false
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_TLS_INSECURE_SKIP_VERIFY"))); v == "1" || v == "true" || v == "yes" {
		tlsSkip = true
		logrus.Warn("LLM_TLS_INSECURE_SKIP_VERIFY=true，已跳过 TLS 证书校验（仅限开发/调试）")
	}
	return Config{
		BaseURL:               baseURL,
		APIKey:                apiKey,
		EmbeddingProvider:     embProvider,
		EmbeddingModel:        embModel,
		ChatModel:             chatModel,
		ThinkingMode:          thinkingMode,
		EmbeddingDim:          dim,
		TLSInsecureSkipVerify: tlsSkip,
	}
}

// openAIClient 基于 go-openai 的实现（支持 DeepSeek、智谱、通义等 OpenAI 兼容 API）
type openAIClient struct {
	client *openai.Client
	config Config
}

// NewOpenAIClient 按配置创建独立的 embedding 与 chat 客户端。
// local embedding 不需要 API key；Chat/Tool Calls 仍要求 API key。
func NewOpenAIClient(config Config) (EmbeddingClient, ChatClient, ToolChatClient) {
	var embedding EmbeddingClient
	switch strings.ToLower(strings.TrimSpace(config.EmbeddingProvider)) {
	case "", "local":
		embedding = NewLocalEmbeddingClient(config.EmbeddingDim)
	case "api", "openai":
		// Assigned after the OpenAI-compatible client is created below.
	default:
		logrus.Errorf("unsupported LLM_EMBEDDING_PROVIDER=%q", config.EmbeddingProvider)
		return nil, nil, nil
	}
	if config.APIKey == "" {
		logrus.Warn("LLM_API_KEY not set, Chat/Agent disabled; local retrieval remains available")
		return embedding, nil, nil
	}
	cfg := openai.DefaultConfig(config.APIKey)
	cfg.BaseURL = config.BaseURL
	httpClient := &http.Client{}
	if config.TLSInsecureSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	if config.ThinkingMode == "disabled" && strings.Contains(strings.ToLower(config.BaseURL), "deepseek.com") {
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		httpClient.Transport = &deepSeekThinkingTransport{base: base}
	}
	cfg.HTTPClient = httpClient
	c := &openAIClient{
		client: openai.NewClientWithConfig(cfg),
		config: config,
	}
	if embedding == nil {
		embedding = c
	}
	return embedding, c, c
}

// Embed 单条文本向量化
func (c *openAIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("embedding returned empty")
	}
	return vecs[0], nil
}

// EmbedBatch 批量向量化
func (c *openAIClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(c.config.EmbeddingModel),
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("create embeddings: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding response empty")
	}
	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = d.Embedding
	}
	if err := c.checkEmbeddingBatch(result); err != nil {
		return nil, err
	}
	return result, nil
}

// validateEmbeddingDimension 校验单条向量长度与配置维度一致；禁止截断/填充。
func validateEmbeddingDimension(dim int, vec []float32) error {
	if dim <= 0 {
		return fmt.Errorf("invalid embedding dimension config: %d (must be > 0)", dim)
	}
	if len(vec) != dim {
		return fmt.Errorf("embedding dimension mismatch: got %d, want LLM_EMBEDDING_DIM=%d", len(vec), dim)
	}
	return nil
}

func (c *openAIClient) checkEmbeddingBatch(vecs [][]float32) error {
	for i, v := range vecs {
		if err := validateEmbeddingDimension(c.config.EmbeddingDim, v); err != nil {
			return fmt.Errorf("embedding[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *openAIClient) Dimension() int {
	return c.config.EmbeddingDim
}

// deepSeekThinkingTransport injects the current DeepSeek V4 non-thinking
// switch without forking the OpenAI-compatible SDK request types.
type deepSeekThinkingTransport struct {
	base http.RoundTripper
}

func (t *deepSeekThinkingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil || !strings.HasSuffix(req.URL.Path, "/chat/completions") {
		return t.base.RoundTrip(req)
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	patched, err := injectDisabledThinking(raw)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(patched))
	req.ContentLength = int64(len(patched))
	req.Header.Set("Content-Length", fmt.Sprint(len(patched)))
	return t.base.RoundTrip(req)
}

func injectDisabledThinking(raw []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("decode chat request for DeepSeek thinking mode: %w", err)
	}
	body["thinking"] = map[string]any{"type": "disabled"}
	return json.Marshal(body)
}

// ChatComplete 非流式对话，返回完整内容
func (c *openAIClient) ChatComplete(ctx context.Context, messages []openai.ChatCompletionMessage) (string, error) {
	content, _, err := c.ChatCompleteWithUsage(ctx, messages)
	return content, err
}

func (c *openAIClient) ChatCompleteWithUsage(ctx context.Context, messages []openai.ChatCompletionMessage) (string, TokenUsage, error) {
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     c.config.ChatModel,
		Messages:  messages,
		Stream:    false,
		MaxTokens: 512,
	})
	if err != nil {
		return "", TokenUsage{}, fmt.Errorf("create chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", TokenUsage{}, fmt.Errorf("chat completion empty")
	}
	return resp.Choices[0].Message.Content, TokenUsage{
		PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens: resp.Usage.TotalTokens,
	}, nil
}

// ChatWithTools 带 tools 的非流式一轮（function calling）
func (c *openAIClient) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (AssistantTurn, error) {
	req := openai.ChatCompletionRequest{
		Model:     c.config.ChatModel,
		Messages:  toOpenAIMessages(messages),
		Stream:    false,
		MaxTokens: 800,
	}
	if len(tools) > 0 {
		req.Tools = toOpenAITools(tools)
	}
	resp, err := c.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return AssistantTurn{}, fmt.Errorf("chat with tools: %w", err)
	}
	return assistantTurnFromResponse(resp)
}

// ChatCompleteTurn 无工具非流式完成
func (c *openAIClient) ChatCompleteTurn(ctx context.Context, messages []ChatMessage) (AssistantTurn, error) {
	return c.ChatWithTools(ctx, messages, nil)
}

func toOpenAIMessages(msgs []ChatMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		om := openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			Name:       m.Name,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				om.ToolCalls[i] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: tc.Args,
					},
				}
			}
		}
		out = append(out, om)
	}
	return out
}

func toOpenAITools(tools []ToolDefinition) []openai.Tool {
	out := make([]openai.Tool, 0, len(tools))
	for _, t := range tools {
		params := map[string]any{}
		if len(t.Parameters) > 0 {
			_ = json.Unmarshal(t.Parameters, &params)
		}
		out = append(out, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func assistantTurnFromResponse(resp openai.ChatCompletionResponse) (AssistantTurn, error) {
	if len(resp.Choices) == 0 {
		return AssistantTurn{}, fmt.Errorf("chat completion empty")
	}
	msg := resp.Choices[0].Message
	turn := AssistantTurn{
		Message: ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		},
		Usage: TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
	if len(msg.ToolCalls) > 0 {
		turn.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		turn.Message.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			call := ToolCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: tc.Function.Arguments,
			}
			turn.ToolCalls[i] = call
			turn.Message.ToolCalls[i] = call
		}
	}
	return turn, nil
}

// ChatStream 流式对话
func (c *openAIClient) ChatStream(ctx context.Context, messages []openai.ChatCompletionMessage, onChunk func(string)) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:     c.config.ChatModel,
		Messages:  messages,
		Stream:    true,
		MaxTokens: 800,
	})
	if err != nil {
		return fmt.Errorf("create chat stream: %w", err)
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			onChunk(chunk.Choices[0].Delta.Content)
		}
	}
}

// Float32ToBytes 将 []float32 转为 little-endian []byte，用于 Redis 向量存储
func Float32ToBytes(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// BytesToFloat32 将 []byte 转为 []float32
func BytesToFloat32(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid byte length %d", len(b))
	}
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = float32fromBytes(b[i*4 : (i+1)*4])
	}
	return v, nil
}

func float32fromBytes(b []byte) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}
