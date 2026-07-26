package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"local-review-go/internal/agent"
	"local-review-go/internal/config"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/repository"
)

func main() {
	testSet := flag.String("test-set", "rag-evals/golden/agent.v1.json", "agent golden set")
	out := flag.String("out", "rag-evals/reports/agent_latest.json", "report output path")
	split := flag.String("split", "test", "test|dev|all")
	trialsFlag := flag.Int("trials", 0, "override case trials (0=use case.trials, min 1)")
	compareBaseline := flag.String("compare-baseline", "", "hybrid_prod baseline path for Agent vs Hybrid RAG")
	forceRoute := flag.String("force-route", "", "force route for all cases")
	mode := flag.String("mode", "inprocess", "inprocess|fake")
	limit := flag.Int("limit", 0, "max cases (0=all); smoke helper")
	userID := flag.Int64("user-id", 900001, "eval user id for memory isolation")
	flag.Parse()

	raw, err := os.ReadFile(*testSet)
	if err != nil {
		log.Fatalf("read test-set: %v", err)
	}
	datasetHash := sha256Hex(raw)
	var file AgentCaseFile
	if err := json.Unmarshal(raw, &file); err != nil {
		log.Fatalf("parse: %v", err)
	}
	cases := filterCases(file.Cases, *split)
	if *limit > 0 && len(cases) > *limit {
		cases = cases[:*limit]
	}
	if len(cases) == 0 {
		log.Fatal("no cases after split/limit")
	}

	var runner TrialRunner
	exp := ExperimentMeta{
		AgentMaxSteps: agent.DefaultMaxSteps,
		AgentMaxTools: agent.DefaultMaxToolCalls,
		ForceRoute:    *forceRoute,
		Mode:          *mode,
		PolicyVersion: "agent-policy-v1",
	}

	switch strings.ToLower(*mode) {
	case "fake":
		runner = &FakeRunner{}
		exp.ChatModel = "fake"
	case "inprocess":
		config.Init()
		if os.Getenv("LLM_API_KEY") == "" {
			log.Fatal("请设置 LLM_API_KEY（或使用 --mode=fake 做 harness 冒烟）")
		}
		cfg := llm.LoadConfig()
		emb, chat, toolChat := llm.NewOpenAIClient(cfg)
		if toolChat == nil {
			log.Fatal("ToolChatClient 不可用：当前模型/网关可能不支持 function calling")
		}
		exp.ChatModel = cfg.ChatModel
		capSearch := &capturingSearch{
			inner: logic.NewShopSearchLogic(logic.ShopSearchLogicDeps{
				EmbeddingClient: emb,
				VectorRepo:      repository.NewVectorRepo(redis.GetRedisClient()),
			}),
		}
		mem := repository.NewHybridMemoryRepo(redis.GetRedisClient(),
			repository.NewAgentProfileRepo(mysql.GetMysqlDB(), redis.GetRedisClient()))
		shopRepo := repository.NewShopRepo(mysql.GetMysqlDB())
		blogRepo := repository.NewBlogRepo(mysql.GetMysqlDB())
		agentLogic := logic.NewRecommendAgentLogic(logic.RecommendAgentLogicDeps{
			ToolChat: toolChat, ChatClient: chat, Memory: mem,
			Search: capSearch, ShopRepo: shopRepo, BlogRepo: blogRepo,
			RunRepo: repository.NewAgentRunRepo(mysql.GetMysqlDB()),
			Router:  logic.NewRecommendRouter(),
			Config:  agent.DefaultRunConfig(),
		})
		runner = &InProcessRunner{Logic: agentLogic, Memory: mem, Search: capSearch, UserID: *userID}
	default:
		log.Fatalf("unknown mode %q", *mode)
	}

	ctx := context.Background()
	caseReports := make([]CaseReport, 0, len(cases))
	nInfra := 0

	for i, c := range cases {
		nTrials := c.Trials
		if nTrials <= 0 {
			nTrials = 1
		}
		if *trialsFlag > 0 {
			nTrials = *trialsFlag
		}
		cr := CaseReport{ID: c.ID, Tags: c.Tags, TrialDetails: make([]TrialDetail, 0, nTrials)}
		passes := make([]bool, 0, nTrials)
		var outN, gN, tN, outOK, gOK, tOK int

		for t := 0; t < nTrials; t++ {
			if i > 0 || t > 0 {
				time.Sleep(200 * time.Millisecond)
			}
			td, err := runner.RunTrial(ctx, c, t, *forceRoute)
			if err != nil && td.InfraError == "" && td.Actual.Answer == "" {
				if td.InfraError == "" {
					td.InfraError = err.Error()
				}
			}
			if td.InfraError != "" {
				nInfra++
				cr.InfraErrors++
				passes = append(passes, false)
				cr.TrialDetails = append(cr.TrialDetails, td)
				log.Printf("[%s trial=%d] infra: %s", c.ID, t, td.InfraError)
				continue
			}
			gradeTrial(&td, c.Expected)
			outN++
			gN++
			tN++
			if td.Outcome.Pass {
				outOK++
			}
			if td.Ground.Pass {
				gOK++
			}
			if td.Traj.Pass {
				tOK++
			}
			passes = append(passes, td.Pass)
			cr.TrialDetails = append(cr.TrialDetails, td)
			log.Printf("[%s trial=%d] pass=%v outcome=%v ground=%v traj=%v session=%s",
				c.ID, t, td.Pass, td.Outcome.Pass, td.Ground.Pass, td.Traj.Pass, td.SessionID)
		}
		cr.Trials = AggregateTrials(passes)
		if outN > 0 {
			cr.OutcomePass = float64(outOK) / float64(outN)
			cr.GroundPass = float64(gOK) / float64(gN)
			cr.TrajPass = float64(tOK) / float64(tN)
		}
		caseReports = append(caseReports, cr)
	}

	rep, err := buildReport(file.Version, datasetHash, exp, caseReports, nInfra)
	if err != nil {
		log.Fatal(err)
	}
	if *compareBaseline != "" {
		if err := compareHybridBaseline(&rep, *compareBaseline); err != nil {
			log.Fatalf("compare-baseline: %v", err)
		}
	}
	if err := writeReport(*out, rep); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s version=%s cases=%d evaluated=%d infra=%d outcome_rate=%.3f pass^k=%.3f\n",
		*out, rep.Version, rep.NTotal, rep.NEvaluated, rep.NInfraError,
		rep.Summary.OutcomeRate, rep.Summary.PassAtKRate)
	if nInfra > 0 {
		os.Exit(2)
	}
}
