package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local-review-go/internal/config"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/llm"
	"local-review-go/internal/logic"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
)

func main() {
	testSetPath := flag.String("test-set", "rag-evals/golden/retrieval.v1.json", "测试集 JSON 路径")
	split := flag.String("split", "test", "test|dev|all（smoke 忽略）")
	filterMode := flag.String("filter-mode", "llm", "none|oracle|llm")
	retriever := flag.String("retriever", "hybrid", "hybrid|dense")
	topK := flag.Int("top-k", 5, "TopK")
	outPath := flag.String("out", "rag-evals/reports/retrieval_latest.json", "报告输出路径")
	writeBaseline := flag.Bool("write-baseline", false, "写入正式 baseline（禁止 smoke）")
	baselinePath := flag.String("baseline", "rag-evals/baseline/hybrid_prod_v1.json", "baseline 路径")
	flag.Parse()

	config.Init()
	ctx := context.Background()

	if os.Getenv("LLM_API_KEY") == "" {
		log.Fatal("请设置 LLM_API_KEY 环境变量")
	}

	raw, err := os.ReadFile(*testSetPath)
	if err != nil {
		log.Fatalf("读取测试集失败: %v", err)
	}
	datasetSHA := sha256Hex(raw)
	cases, version, isSmoke, err := loadCases(raw, *testSetPath)
	if err != nil {
		log.Fatalf("加载测试集失败: %v", err)
	}
	if isSmoke && *filterMode != "none" {
		log.Fatalf("smoke 集仅允许 --filter-mode=none，got %q", *filterMode)
	}
	if isSmoke && *writeBaseline {
		log.Fatal("smoke 集禁止 --write-baseline")
	}
	if !isSmoke {
		cases = filterSplit(cases, *split)
	}
	if len(cases) == 0 {
		log.Fatal("过滤后测试集为空")
	}

	strategy, err := logic.ParseRetrieverStrategy(*retriever)
	if err != nil {
		log.Fatal(err)
	}

	cfg := llm.LoadConfig()
	embClient, chatClient, _ := llm.NewOpenAIClient(cfg)
	if embClient == nil {
		log.Fatal("Embedding 客户端初始化失败")
	}
	vecRepo := repository.NewVectorRepo(redis.GetRedisClient())
	shopSearch := logic.NewShopSearchLogic(logic.ShopSearchLogicDeps{
		EmbeddingClient: embClient,
		VectorRepo:      vecRepo,
	})
	var extractor logic.FilterExtractor
	if *filterMode == "llm" {
		extractor = logic.NewLLMFilterExtractor(chatClient)
	}

	report := EvalReport{
		DatasetVersion:     version,
		DatasetSHA256:      datasetSHA,
		SeedVersion:        "seed.sql",
		RedisImage:         "redis/redis-stack-server:7.4.0-v8",
		IndexSchemaVersion: "idx:shop:vector",
		Retriever:          string(strategy),
		FilterMode:         *filterMode,
		EmbeddingModel:     cfg.EmbeddingModel,
		EmbeddingDim:       cfg.EmbeddingDim,
		FilterModel:        cfg.ChatModel,
		TopK:               *topK,
		RRFK:               60,
		CandidateK:         20,
		IsSmoke:            isSmoke,
		NTotal:             len(cases),
	}

	var filterAccSum, filterCompSum float64
	var filterAccN, filterCompN int

	for i, c := range cases {
		if i > 0 {
			time.Sleep(300 * time.Millisecond)
		}
		cr := CaseResult{ID: c.ID, Question: c.Question}
		relevant := c.RelevantShopIDs
		if len(relevant) == 0 {
			relevant = c.ExpectedShopIDs
		}

		filter, ferr := resolveEvalFilter(ctx, *filterMode, c, extractor)
		if ferr != nil {
			cr.InfraError = ferr.Error()
			report.NInfraError++
			report.PerCase = append(report.PerCase, cr)
			log.Printf("[%s] infra filter: %v", c.ID, ferr)
			continue
		}

		shops, serr := shopSearch.Search(ctx, c.Question, filter, strategy, *topK)
		if serr != nil {
			cr.InfraError = serr.Error()
			report.NInfraError++
			report.PerCase = append(report.PerCase, cr)
			log.Printf("[%s] infra search: %v", c.ID, serr)
			continue
		}

		ids := make([]int64, len(shops))
		hits := make([]ShopHit, len(shops))
		for j, s := range shops {
			ids[j] = s.ShopID
			hits[j] = ShopHit{
				ShopID: s.ShopID, Area: s.Area, TypeName: s.TypeName,
				AvgPrice: s.AvgPrice, ShopScore: s.ShopScore, Comments: s.Comments,
			}
		}
		cr.RetrievedIDs = ids
		cr.HitRate = HitRateAtK(ids, relevant, *topK)
		cr.Recall = RecallAtK(ids, relevant, *topK)
		cr.Precision = PrecisionAtK(ids, relevant, *topK)
		cr.MRR = MRR(ids, relevant)
		cr.NDCG = NDCGAtK(ids, relevant, *topK)

		if *filterMode == "llm" && c.OracleFilter != nil {
			pred := filterToJSON(filter)
			cr.FilterFieldAccuracy = FilterFieldAccuracy(pred, c.OracleFilter)
			filterAccSum += cr.FilterFieldAccuracy
			filterAccN++
		}
		if c.OracleFilter != nil {
			cr.FilterCompliance = FilterComplianceAtK(hits, c.OracleFilter, *topK)
			filterCompSum += cr.FilterCompliance
			filterCompN++
		}

		report.NEvaluated++
		report.PerCase = append(report.PerCase, cr)
		mark := "✗"
		if cr.HitRate > 0 {
			mark = "✓"
		}
		log.Printf("[%s] %s %s hit=%.0f recall=%.2f", c.ID, mark, truncate(c.Question, 40), cr.HitRate, cr.Recall)
	}

	hr, rc, pr, mrr, ndcg := AggregateQuality(report.PerCase, *topK)
	report.HitRateAtK = hr
	report.RecallAtK = rc
	report.PrecisionAtK = pr
	report.MRR = mrr
	report.NDCGAtK = ndcg
	if filterAccN > 0 {
		report.FilterFieldAccuracy = filterAccSum / float64(filterAccN)
	}
	if filterCompN > 0 {
		report.FilterComplianceAtK = filterCompSum / float64(filterCompN)
	}
	if report.NTotal > 0 {
		report.InfraErrorRate = float64(report.NInfraError) / float64(report.NTotal)
	}

	log.Println("----------")
	log.Printf("HitRate@%d:    %.1f%%", *topK, 100*report.HitRateAtK)
	log.Printf("Recall@%d:     %.1f%%", *topK, 100*report.RecallAtK)
	log.Printf("Precision@%d:  %.1f%%", *topK, 100*report.PrecisionAtK)
	log.Printf("MRR:           %.4f", report.MRR)
	log.Printf("nDCG@%d:       %.4f", *topK, report.NDCGAtK)
	log.Printf("n_total=%d n_evaluated=%d n_infra_error=%d is_smoke=%v",
		report.NTotal, report.NEvaluated, report.NInfraError, report.IsSmoke)
	log.Printf("filter_mode=%s retriever=%s", report.FilterMode, report.Retriever)

	if err := writeJSON(*outPath, report); err != nil {
		log.Fatalf("写报告失败: %v", err)
	}
	log.Printf("report → %s", *outPath)

	if *writeBaseline {
		if report.IsSmoke || strategy != logic.RetrieverHybrid {
			log.Fatal("baseline 仅允许正式集 + hybrid")
		}
		if err := writeJSON(*baselinePath, report); err != nil {
			log.Fatalf("写 baseline 失败: %v", err)
		}
		log.Printf("baseline → %s", *baselinePath)
	}

	if !isSmoke && report.NInfraError > 0 {
		os.Exit(1)
	}
}

func resolveEvalFilter(ctx context.Context, mode string, c RetrievalCase, extractor logic.FilterExtractor) (*repoInterfaces.VectorSearchFilter, error) {
	switch mode {
	case "none":
		return nil, nil
	case "oracle":
		return filterJSONToRepo(c.OracleFilter), nil
	case "llm":
		if extractor == nil {
			return nil, fmt.Errorf("llm filter-mode requires ChatClient")
		}
		extracted, err := extractor.Extract(ctx, c.Question)
		if err != nil {
			return nil, err
		}
		return logic.ResolveFilter(nil, extracted), nil
	default:
		return nil, fmt.Errorf("unsupported filter-mode: %s", mode)
	}
}

func filterJSONToRepo(f *FilterJSON) *repoInterfaces.VectorSearchFilter {
	if f == nil || !f.HasHardConstraints() {
		return nil
	}
	return &repoInterfaces.VectorSearchFilter{
		Area: f.Area, TypeName: f.TypeName,
		MaxPrice: f.MaxPrice, MinPrice: f.MinPrice,
		MinScore: f.MinScore, MinComments: f.MinComments,
	}
}

func filterToJSON(f *repoInterfaces.VectorSearchFilter) *FilterJSON {
	if f == nil {
		return &FilterJSON{}
	}
	return &FilterJSON{
		Area: f.Area, TypeName: f.TypeName,
		MaxPrice: f.MaxPrice, MinPrice: f.MinPrice,
		MinScore: f.MinScore, MinComments: f.MinComments,
	}
}

func loadCases(raw []byte, path string) ([]RetrievalCase, string, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		// smoke 旧格式
		var smoke []struct {
			Question        string  `json:"question"`
			ExpectedShopIDs []int64 `json:"expected_shop_ids"`
		}
		if err := json.Unmarshal(raw, &smoke); err != nil {
			return nil, "", false, err
		}
		out := make([]RetrievalCase, 0, len(smoke))
		for i, s := range smoke {
			if s.Question == "" {
				continue // skip comment-only objects
			}
			if err := ValidateRelevantNonEmpty(fmt.Sprintf("smoke-%d", i+1), s.ExpectedShopIDs); err != nil {
				return nil, "", false, err
			}
			out = append(out, RetrievalCase{
				ID:              fmt.Sprintf("smoke-%d", i+1),
				Question:        s.Question,
				RelevantShopIDs: s.ExpectedShopIDs,
				ExpectedShopIDs: s.ExpectedShopIDs,
				IsSmoke:         true,
			})
		}
		return out, "smoke", true, nil
	}

	var gf GoldenFile
	if err := json.Unmarshal(raw, &gf); err != nil {
		return nil, "", false, err
	}
	if gf.Version == "" {
		gf.Version = filepath.Base(path)
	}
	for i := range gf.Cases {
		c := &gf.Cases[i]
		if err := ValidateRelevantNonEmpty(c.ID, c.RelevantShopIDs); err != nil {
			return nil, "", false, err
		}
		if strings.TrimSpace(c.Evidence) == "" {
			return nil, "", false, fmt.Errorf("case %q: evidence must be non-empty", c.ID)
		}
	}
	return gf.Cases, gf.Version, false, nil
}

func filterSplit(cases []RetrievalCase, split string) []RetrievalCase {
	split = strings.ToLower(strings.TrimSpace(split))
	if split == "" || split == "all" {
		return cases
	}
	out := make([]RetrievalCase, 0, len(cases))
	for _, c := range cases {
		if strings.EqualFold(c.Split, split) {
			out = append(out, c)
		}
	}
	return out
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
