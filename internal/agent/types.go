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
	DefaultRunTimeout         = 45 * time.Second
	DefaultToolTimeout        = 10 * time.Second
	DefaultMaxToolResultChars = 6000
)

// RunConfig 单次推荐运行的硬预算（可由环境变量覆盖）
type RunConfig struct {
	MaxSteps           int
	MaxToolCalls       int
	RunTimeout         time.Duration
	ToolTimeout        time.Duration
	MaxToolResultChars int
}

// DefaultRunConfig 从环境变量加载，非法/空值回退默认
func DefaultRunConfig() RunConfig {
	cfg := RunConfig{
		MaxSteps:           DefaultMaxSteps,
		MaxToolCalls:       DefaultMaxToolCalls,
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
