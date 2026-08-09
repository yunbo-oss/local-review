package evalmeta

import (
	"strings"
	"testing"
)

func TestCaptureHonorsExplicitGitMetadata(t *testing.T) {
	t.Setenv("EVAL_GIT_SHA", "deadbeef")
	t.Setenv("EVAL_GIT_DIRTY", "false")
	m := Capture()
	if m.GitCommit != "deadbeef" || m.GitDirty {
		t.Fatalf("metadata=%+v", m)
	}
	if m.StartedAtUTC == "" || !strings.HasPrefix(m.GoVersion, "go") {
		t.Fatalf("runtime metadata incomplete: %+v", m)
	}
}
