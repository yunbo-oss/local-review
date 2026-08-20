package agent

import "testing"

func TestDefaultRunConfig_Defaults(t *testing.T) {
	t.Setenv("AGENT_MAX_STEPS", "")
	t.Setenv("AGENT_MAX_TOOL_CALLS", "")
	t.Setenv("AGENT_RUN_TIMEOUT", "")
	t.Setenv("AGENT_TOOL_TIMEOUT", "")
	t.Setenv("AGENT_MAX_TOOL_RESULT_CHARS", "")
	cfg := DefaultRunConfig()
	if cfg.MaxSteps != DefaultMaxSteps || cfg.MaxToolCalls != DefaultMaxToolCalls {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.RunTimeout != DefaultRunTimeout || cfg.ToolTimeout != DefaultToolTimeout {
		t.Fatalf("unexpected timeouts: %+v", cfg)
	}
}

func TestDefaultRunConfig_FromEnv(t *testing.T) {
	t.Setenv("AGENT_MAX_STEPS", "4")
	t.Setenv("AGENT_MAX_TOOL_CALLS", "7")
	t.Setenv("AGENT_RUN_TIMEOUT", "30s")
	t.Setenv("AGENT_TOOL_TIMEOUT", "5s")
	t.Setenv("AGENT_MAX_TOOL_RESULT_CHARS", "1000")
	cfg := DefaultRunConfig()
	if cfg.MaxSteps != 4 || cfg.MaxToolCalls != 7 {
		t.Fatalf("got %+v", cfg)
	}
	if cfg.RunTimeout.Seconds() != 30 || cfg.ToolTimeout.Seconds() != 5 {
		t.Fatalf("timeouts %+v", cfg)
	}
	if cfg.MaxToolResultChars != 1000 {
		t.Fatalf("chars %d", cfg.MaxToolResultChars)
	}
}

func TestRuntimeVersionFromEnv(t *testing.T) {
	t.Setenv("AGENT_RUNTIME_VERSION", "v1")
	if got := RuntimeVersionFromEnv(); got != RuntimeVersionV1Plan {
		t.Fatalf("v1=%q", got)
	}
	t.Setenv("AGENT_RUNTIME_VERSION", "")
	if got := RuntimeVersionFromEnv(); got != RuntimeVersionV2React {
		t.Fatalf("default=%q", got)
	}
}
