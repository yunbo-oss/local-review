package llm

import (
	"context"
	"encoding/json"
)

// ToolDefinition OpenAI-compatible function tool（Agent 核心依赖本包类型，不直接绑 go-openai）
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // JSON Schema object
}

// ToolCall 模型发起的一次工具调用
type ToolCall struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Args string `json:"arguments"` // JSON object string
}

// ChatMessage 通用对话消息（含 tool 角色）
type ChatMessage struct {
	Role       string     `json:"role"` // system|user|assistant|tool
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"` // tool 名（role=tool 时）
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// TokenUsage token 用量摘要
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AssistantTurn 一轮模型输出
type AssistantTurn struct {
	Message   ChatMessage
	ToolCalls []ToolCall
	Usage     TokenUsage
}

// ToolChatClient 带工具调用的对话接口
type ToolChatClient interface {
	ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolDefinition) (AssistantTurn, error)
	// ChatCompleteTurn 无工具的非流式完成（返回 AssistantTurn，便于统一 usage）
	ChatCompleteTurn(ctx context.Context, messages []ChatMessage) (AssistantTurn, error)
}
