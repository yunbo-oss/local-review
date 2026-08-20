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
	"local-review-go/internal/config/postgres"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/evalmeta"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/repository"
)

func main() {
	testSet := flag.String("test-set", "rag-evals/golden/agent.v2.json", "agent golden set")
	out := flag.String("out", "rag-evals/reports/agent_latest.json", "report output path")
	system := flag.String("system", "agent", "agent|hybrid_rag")
	split := flag.String("split", "test", "test|dev|all")
	trialsFlag := flag.Int("trials", 0, "override case trials (0=use case.trials, min 1)")
	compareBaseline := flag.String("compare-baseline", "", "same-task Hybrid RAG task report path")
	forceRoute := flag.String("force-route", "", "force route for all cases")
	mode := flag.String("mode", "inprocess", "inprocess|fake")
	entrypoint := flag.String("entrypoint", "agent_direct", "agent_direct|router_e2e (agent system only)")
	limit := flag.Int("limit", 0, "max cases (0=all); smoke helper")
	caseID := flag.String("case-id", "", "run one exact case id; empty runs the selected split")
	userID := flag.Int64("user-id", 900001, "eval user id for memory isolation")
	inputPrice := flag.Float64("input-price-usd-per-million", 0.14, "cache-miss input token price used for cost estimate")
	outputPrice := flag.Float64("output-price-usd-per-million", 0.28, "output token price used for cost estimate")
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
	if *caseID != "" {
		filtered := cases[:0]
		for _, c := range cases {
			if c.ID == *caseID {
				filtered = append(filtered, c)
			}
		}
		cases = filtered
	}
	if *limit > 0 && len(cases) > *limit {
		cases = cases[:*limit]
	}
	if len(cases) == 0 {
		log.Fatal("no cases after split/limit")
	}

	var runner TrialRunner
	exp := ExperimentMeta{
		System:                *system,
		EntryPoint:            *entrypoint,
		Split:                 *split,
		AgentMaxSteps:         agent.DefaultMaxSteps,
		AgentMaxTools:         agent.DefaultMaxToolCalls,
		ForceRoute:            *forceRoute,
		Mode:                  *mode,
		PolicyVersion:         "agent-policy-v2-plan-claim",
		Retriever:             "hybrid",
		TopK:                  5,
		InputPriceUSDPerMTok:  *inputPrice,
		OutputPriceUSDPerMTok: *outputPrice,
		Runtime:               evalmeta.Capture(),
	}

	switch strings.ToLower(*mode) {
	case "fake":
		if *system != "agent" {
			log.Fatal("fake mode only supports --system=agent")
		}
		runner = &FakeRunner{}
		exp.ChatModel = "fake"
		if runtimeVersion := cases[0].Expected.RuntimeVersion; runtimeVersion != "" {
			exp.AgentRuntimeVersion = runtimeVersion
			exp.AgentMaxSteps = cases[0].Expected.MaxSteps
			exp.AgentMaxTools = cases[0].Expected.MaxToolCalls
			exp.AgentMaxSearchRounds = cases[0].Expected.MaxSearchRounds
			exp.AgentMaxReviewPages = cases[0].Expected.MaxReviewPagesPerShop
			exp.PolicyVersion = "agent-policy-v3-parallel-react"
		}
	case "inprocess":
		config.Init()
		if os.Getenv("LLM_API_KEY") == "" {
			log.Fatal("请设置 LLM_API_KEY（或使用 --mode=fake 做 harness 冒烟）")
		}
		cfg := llm.LoadConfig()
		emb, chat, toolChat := llm.NewOpenAIClient(cfg)
		exp.ChatModel = cfg.ChatModel
		exp.ChatTemperature = cfg.Temperature
		exp.ThinkingMode = cfg.ThinkingMode
		exp.EmbeddingProvider = cfg.EmbeddingProvider
		exp.EmbeddingModel = cfg.EmbeddingModel
		exp.EmbeddingDim = cfg.EmbeddingDim
		baseSearch := logic.NewShopSearchLogic(logic.ShopSearchLogicDeps{
			EmbeddingClient: emb,
			VectorRepo:      repository.NewVectorRepo(postgres.GetPostgresDB()),
		})
		switch *system {
		case "hybrid_rag":
			if chat == nil {
				log.Fatal("Hybrid RAG ChatClient 不可用")
			}
			runner = &HybridRAGRunner{Search: baseSearch, Chat: chat}
			exp.AgentMaxSteps, exp.AgentMaxTools = 0, 0
			exp.PolicyVersion = "hybrid-rag-v2"
		case "agent":
			if toolChat == nil {
				log.Fatal("ToolChatClient 不可用：当前模型/网关可能不支持 function calling")
			}
			capSearch := &capturingSearch{inner: baseSearch}
			mem := repository.NewHybridMemoryRepo(redis.GetRedisClient(),
				repository.NewAgentProfileRepo(postgres.GetPostgresDB(), redis.GetRedisClient()))
			shopRepo := repository.NewShopRepo(postgres.GetPostgresDB())
			blogRepo := repository.NewBlogRepo(postgres.GetPostgresDB())
			runCfg := agent.DefaultRunConfig()
			runtimeVersion := agent.RuntimeVersionFromEnv()
			baseRouter := logic.NewRecommendRouter()
			adaptiveRouter := logic.NewAdaptiveRecommendRouter(
				baseRouter, agent.NewLLMQueryUnderstander(chat),
			)
			exp.AgentRuntimeVersion = runtimeVersion
			exp.AgentMaxSteps = runCfg.MaxSteps
			exp.AgentMaxTools = runCfg.MaxToolCalls
			exp.AgentMaxToolAttempts = runCfg.MaxToolAttempts
			exp.AgentMaxToolsPerTurn = runCfg.MaxToolsPerTurn
			if runtimeVersion == agent.RuntimeVersionV2React {
				budget := agent.RuntimeBudgetFromEnv()
				exp.AgentMaxSteps = budget.MaxTurns
				exp.AgentMaxTools = budget.MaxToolCalls
				exp.AgentMaxToolAttempts = budget.MaxToolAttempts
				exp.AgentMaxToolsPerTurn = budget.MaxParallelTools
				exp.AgentMaxSearchRounds = budget.MaxSearchRounds
				exp.AgentMaxReviewPages = budget.MaxReviewPagesPerShop
				exp.PolicyVersion = "agent-policy-v3-parallel-react"
			}
			exp.AgentRunTimeout = runCfg.RunTimeout.String()
			exp.AgentToolTimeout = runCfg.ToolTimeout.String()
			agentLogic := logic.NewRecommendAgentLogic(logic.RecommendAgentLogicDeps{
				ToolChat: toolChat, ChatClient: chat, Memory: mem,
				Search: capSearch, ShopRepo: shopRepo, BlogRepo: blogRepo,
				RunRepo:        repository.NewAgentRunRepo(postgres.GetPostgresDB()),
				Router:         baseRouter,
				AdaptiveRouter: adaptiveRouter,
				Reranker:       logic.NewLLMCandidateReranker(chat),
				Planner:        agent.NewLLMPlanner(chat),
				Controller:     agent.NewLLMDecisionController(toolChat),
				Checkpointer:   repository.NewRedisAgentCheckpointer(redis.GetRedisClient()),
				RuntimeVersion: runtimeVersion,
				Summarizer:     agent.NewLLMSessionSummarizer(chat),
				Config:         runCfg,
			})
			inProcess := &InProcessRunner{Logic: agentLogic, Memory: mem, Search: capSearch, UserID: *userID}
			switch strings.TrimSpace(*entrypoint) {
			case "agent_direct":
			case "router_e2e":
				inProcess.Router = adaptiveRouter
				inProcess.RAG = &HybridRAGRunner{Search: capSearch, Chat: chat}
				exp.ForceRoute = ""
			default:
				log.Fatalf("unknown entrypoint %q", *entrypoint)
			}
			runner = inProcess
		default:
			log.Fatalf("unknown system %q", *system)
		}
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
			gradeTrial(&td, c.Expected, exp)
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
	fmt.Printf("wrote %s version=%s cases=%d trials=%d evaluated=%d infra=%d outcome_rate=%.3f all_trials_pass_rate=%.3f\n",
		*out, rep.Version, rep.NCases, rep.NTrials, rep.NEvaluated, rep.NInfraError,
		rep.Summary.OutcomeRate, rep.Summary.AllTrialsPassRate)
	if nInfra > 0 {
		os.Exit(2)
	}
}
