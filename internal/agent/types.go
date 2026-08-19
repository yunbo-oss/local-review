package agent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxSteps           = 3
	DefaultMaxToolCalls       = 5
	DefaultMaxToolAttempts    = 8
	DefaultMaxToolsPerTurn    = 3
	DefaultRunTimeout         = 45 * time.Second
	DefaultToolTimeout        = 10 * time.Second
	DefaultMaxToolResultChars = 6000
)

// RunConfig 单次推荐运行的硬预算（可由环境变量覆盖）
type RunConfig struct {
	MaxSteps           int
	MaxToolCalls       int // 成功执行的工具次数上限（兼容旧字段）
	MaxToolAttempts    int // 含失败/重复/未知的尝试次数上限
	MaxToolsPerTurn    int // 单轮模型 turn 最多执行的 tool call 数
	RunTimeout         time.Duration
	ToolTimeout        time.Duration
	MaxToolResultChars int
}

// DefaultRunConfig 从环境变量加载，非法/空值回退默认
func DefaultRunConfig() RunConfig {
	cfg := RunConfig{
		MaxSteps:           DefaultMaxSteps,
		MaxToolCalls:       DefaultMaxToolCalls,
		MaxToolAttempts:    DefaultMaxToolAttempts,
		MaxToolsPerTurn:    DefaultMaxToolsPerTurn,
		RunTimeout:         DefaultRunTimeout,
		ToolTimeout:        DefaultToolTimeout,
		MaxToolResultChars: DefaultMaxToolResultChars,
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_MAX_STEPS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxSteps = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_MAX_TOOL_CALLS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxToolCalls = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_MAX_TOOL_ATTEMPTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxToolAttempts = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_MAX_TOOLS_PER_TURN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxToolsPerTurn = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_RUN_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.RunTimeout = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_TOOL_TIMEOUT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ToolTimeout = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_MAX_TOOL_RESULT_CHARS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxToolResultChars = n
		}
	}
	return cfg
}

// RuntimeVersionFromEnv keeps V1 available as a replay/baseline path while
// making the observation-driven V2 runtime the production default.
func RuntimeVersionFromEnv() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_RUNTIME_VERSION"))) {
	case "v1", "v1_plan", "plan":
		return RuntimeVersionV1Plan
	default:
		return RuntimeVersionV2React
	}
}
