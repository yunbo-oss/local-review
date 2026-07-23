package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Phase stub：完整 harness 在 US5；当前可跑 graders 单测与 golden 校验。
func main() {
	testSet := flag.String("test-set", "rag-evals/golden/agent.v1.json", "agent golden set")
	out := flag.String("out", "", "report output path (optional)")
	flag.Parse()

	raw, err := os.ReadFile(*testSet)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read test-set: %v\n", err)
		os.Exit(1)
	}
	var file AgentCaseFile
	if err := json.Unmarshal(raw, &file); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("loaded agent golden version=%s cases=%d\n", file.Version, len(file.Cases))
	fmt.Println("NOTE: full Agent eval harness (API trials + graders) lands in US5; use go test ./cmd/eval-agent for graders.")
	if *out != "" {
		rep := map[string]any{
			"version": "agent-eval.v1-stub",
			"n_total": len(file.Cases),
			"note":    "stub report — run full harness after RecommendAgent wired",
		}
		b, _ := json.MarshalIndent(rep, "", "  ")
		_ = os.WriteFile(*out, b, 0o644)
	}
}
