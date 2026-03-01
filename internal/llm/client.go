package llm

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

const (
	// 默认模型（DeepSeek / OpenAI 兼容）
	defaultEmbeddingModel = "text-embedding-3-small"
	defaultChatModel      = "deepseek-chat"
	defaultBaseURL        = "https://api.deepseek.com/v1"
)

// EmbeddingClient 文本向量化接口
type EmbeddingClient interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// ChatClient 对话接口（支持流式）
type ChatClient interface {
	ChatStream(ctx context.Context, messages []openai.ChatCompletionMessage, onChunk func(string)) error
}

// Config 从环境变量读取 LLM 配置
type Config struct {
	BaseURL        string
	APIKey         string
	EmbeddingModel string
	ChatModel      string
	EmbeddingDim   int
}

func LoadConfig() Config {
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	apiKey := os.Getenv("LLM_API_KEY")
	embModel := os.Getenv("LLM_EMBEDDING_MODEL")
	if embModel == "" {
		embModel = defaultEmbeddingModel
	}
	chatModel := os.Getenv("LLM_CHAT_MODEL")
	if chatModel == "" {
		chatModel = defaultChatModel
	}
	dim := 1024
	if d := os.Getenv("LLM_EMBEDDING_DIM"); d != "" {
		if n, err := fmt.Sscanf(d, "%d", &dim); err == nil && n == 1 {
			// ok
		}
	}
	return Config{
		BaseURL:        baseURL,
		APIKey:         apiKey,
		EmbeddingModel: embModel,
		ChatModel:      chatModel,
		EmbeddingDim:   dim,
	}
}

// openAIClient 基于 go-openai 的实现（支持 DeepSeek、智谱、通义等 OpenAI 兼容 API）
type openAIClient struct {
	client *openai.Client
	config Config
}

// NewOpenAIClient 创建 LLM 客户端。若 APIKey 为空则返回 nil，RAG 功能不可用。
func NewOpenAIClient(config Config) (EmbeddingClient, ChatClient) {
	if config.APIKey == "" {
		logrus.Warn("LLM_API_KEY not set, RAG chat disabled")
		return nil, nil
	}
	cfg := openai.DefaultConfig(config.APIKey)
	cfg.BaseURL = config.BaseURL
	c := &openAIClient{
		client: openai.NewClientWithConfig(cfg),
		config: config,
	}
	return c, c
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
	return result, nil
}

func (c *openAIClient) Dimension() int {
	return c.config.EmbeddingDim
}

// ChatStream 流式对话
func (c *openAIClient) ChatStream(ctx context.Context, messages []openai.ChatCompletionMessage, onChunk func(string)) error {
	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    c.config.ChatModel,
		Messages: messages,
		Stream:   true,
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
