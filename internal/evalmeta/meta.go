// Package evalmeta records execution conditions shared by formal evaluators.
package evalmeta

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Runtime struct {
	StartedAtUTC  string `json:"started_at_utc"`
	GitCommit     string `json:"git_commit"`
	GitDirty      bool   `json:"git_dirty"`
	GitStateKnown bool   `json:"git_state_known"`
	GoVersion     string `json:"go_version"`
}

func Capture() Runtime {
	commit := strings.TrimSpace(os.Getenv("EVAL_GIT_SHA"))
	if commit == "" {
		commit = commandOutput("git", "rev-parse", "HEAD")
	}
	dirty := false
	if forced := strings.TrimSpace(os.Getenv("EVAL_GIT_DIRTY")); forced != "" {
		dirty = forced == "1" || strings.EqualFold(forced, "true")
	} else {
		dirty = commandOutput("git", "status", "--porcelain") != ""
	}
	return Runtime{
		StartedAtUTC: time.Now().UTC().Format(time.RFC3339),
		GitCommit:    commit, GitDirty: dirty, GitStateKnown: commit != "", GoVersion: runtime.Version(),
	}
}

func commandOutput(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
