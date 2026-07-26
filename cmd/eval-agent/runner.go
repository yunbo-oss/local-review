package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local-review-go/internal/agent"
	"local-review-go/internal/logic"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/google/uuid"
)

// TrialRunner 单次 trial 执行（便于 fake / inprocess）
type TrialRunner interface {
	RunTrial(ctx context.Context, c AgentCase, trialIdx int, forceRoute string) (TrialDetail, error)
}

// FakeRunner 确定性假运行：不调 LLM；用于 harness 形状与隔离测试
type FakeRunner struct {
	DefaultShop int64
}

func (f *FakeRunner) RunTrial(ctx context.Context, c AgentCase, trialIdx int, forceRoute string) (TrialDetail, error) {
	sid := fmt.Sprintf("eval-%s-t%d-%s", c.ID, trialIdx, uuid.NewString()[:8])
	shop := f.DefaultShop
	if shop == 0 {
		shop = 8
	}
	if len(c.Expected.AllowedShopIDs) > 0 {
		shop = c.Expected.AllowedShopIDs[0]
	}
	filter := map[string]any{}
	for k, v := range c.Expected.FilterContains {
		filter[k] = v
	}
	prof := map[string]any{}
	for k, v := range c.Expected.ProfileAfter {
		prof[k] = v
	}
	if len(prof) == 0 {
		prof = cloneSetup(c.SetupProfile)
	}
	ans := fmt.Sprintf("推荐 [shop:%d]", shop)
	if c.Expected.ExpectNoResults {
		ans = "没有合适的店铺"
		shop = 0
	}
	cited := []int64{}
	obs := []int64{}
	if shop > 0 {
		cited = []int64{shop}
		obs = []int64{shop}
	}
	actual := OutcomeActual{
		Filter: filter, CitedShopIDs: cited, ObservedShopIDs: obs,
		ProfileAfter: prof, Steps: 1, ToolCalls: 1,
		Answer: ans, LatencyMs: 10, Tokens: 100,
	}
	return TrialDetail{
		TrialIndex: trialIdx, SessionID: sid,
		Route: forceRoute, TraceID: "fake-" + sid,
		Actual: actual,
	}, nil
}

func cloneSetup(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	b, _ := json.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}

// capturingSearch 记录最后一次合并后的 filter（eval 用）
type capturingSearch struct {
	inner      logic.ShopSearchLogic
	lastFilter *repoInterfaces.VectorSearchFilter
	degraded   bool
	reason     string
}

func (c *capturingSearch) Search(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy logic.RetrieverStrategy, topK int) ([]repoInterfaces.ShopSearchResult, error) {
	c.lastFilter = filter
	return c.inner.Search(ctx, query, filter, strategy, topK)
}

func (c *capturingSearch) SearchWithMeta(ctx context.Context, query string, filter *repoInterfaces.VectorSearchFilter, strategy logic.RetrieverStrategy, topK int, mode logic.SearchMode) (logic.SearchOutcome, error) {
	c.lastFilter = filter
	out, err := c.inner.SearchWithMeta(ctx, query, filter, strategy, topK, mode)
	if out.Degraded {
		c.degraded = true
		c.reason = out.DegradedReason
	}
	return out, err
}

// InProcessRunner 进程内 RecommendAgentLogic
type InProcessRunner struct {
	Logic  logic.RecommendAgentLogic
	Memory repoInterfaces.MemoryRepo
	Search *capturingSearch
	UserID int64
}

func (r *InProcessRunner) RunTrial(ctx context.Context, c AgentCase, trialIdx int, forceRoute string) (TrialDetail, error) {
	sid := fmt.Sprintf("eval-%s-t%d-%s", c.ID, trialIdx, uuid.NewString()[:8])
	td := TrialDetail{TrialIndex: trialIdx, SessionID: sid}
	if r.Logic == nil || r.Memory == nil {
		td.InfraError = "runner not configured"
		return td, fmt.Errorf("%s", td.InfraError)
	}
	prof := setupToProfile(c.SetupProfile)
	if err := r.Memory.ReplaceProfile(ctx, r.UserID, prof); err != nil {
		td.InfraError = "setup_profile: " + err.Error()
		return td, err
	}
	var lastAns string
	var lastRes logic.RecommendResult
	var lastErr error
	start := time.Now()
	for _, turn := range c.Turns {
		q := strings.TrimSpace(turn.User)
		if q == "" {
			continue
		}
		res, err := r.Logic.Recommend(ctx, r.UserID, sid, q, forceRoute, nil)
		lastRes, lastErr = res, err
		lastAns = res.Answer
		if err != nil && res.Answer == "" {
			break
		}
	}
	latency := time.Since(start).Milliseconds()
	if lastErr != nil && lastAns == "" {
		msg := lastErr.Error()
		if isInfraErr(msg) {
			td.InfraError = msg
			return td, lastErr
		}
	}
	after, _ := r.Memory.LoadProfile(ctx, r.UserID)
	filter := filterToMap(nil)
	if r.Search != nil {
		filter = filterToMap(r.Search.lastFilter)
	}
	cited := agent.ParseCitedShopIDs(lastAns)
	if len(cited) == 0 {
		cited = agent.ParseCitedShopIDs(lastRes.Answer)
	}
	td.Actual = OutcomeActual{
		Filter:          filter,
		CitedShopIDs:    cited,
		ObservedShopIDs: lastRes.ObservedShopIDs,
		ProfileAfter:    profileToMap(after),
		Steps:           lastRes.Steps,
		ToolCalls:       lastRes.ToolCalls,
		Answer:          lastAns,
		LatencyMs:       latency,
		Tokens:          lastRes.Usage.TotalTokens,
	}
	td.Route = lastRes.Route
	td.TraceID = lastRes.TraceID
	return td, nil
}

func isInfraErr(msg string) bool {
	low := strings.ToLower(msg)
	for _, s := range []string{"未完整配置", "timeout", "connection refused", "api key", "429", "redis", "mysql"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

func setupToProfile(m map[string]any) memory.Profile {
	p := memory.Profile{}
	if m == nil {
		return p
	}
	b, _ := json.Marshal(m)
	_ = json.Unmarshal(b, &p)
	if v, ok := m["budget_max"]; ok && v != nil {
		switch t := v.(type) {
		case float64:
			x := int64(t)
			p.BudgetMax = &x
		case int64:
			p.BudgetMax = &t
		case json.Number:
			if i, err := t.Int64(); err == nil {
				p.BudgetMax = &i
			}
		}
	}
	if p.Version <= 0 {
		p.Version = 1
	}
	p.UpdatedAt = memory.NowUnix()
	return p
}

func profileToMap(p memory.Profile) map[string]any {
	m := map[string]any{
		"preferred_areas": p.PreferredAreas,
		"preferred_types": p.PreferredTypes,
		"dislikes":        p.Dislikes,
		"summary":         p.Summary,
	}
	if p.BudgetMax == nil {
		m["budget_max"] = nil
	} else {
		m["budget_max"] = *p.BudgetMax
	}
	return m
}

func filterToMap(f *repoInterfaces.VectorSearchFilter) map[string]any {
	if f == nil {
		return map[string]any{}
	}
	m := map[string]any{}
	if f.Area != "" {
		m["area"] = f.Area
	}
	if f.TypeName != "" {
		m["typeName"] = f.TypeName
	}
	if f.MaxPrice > 0 {
		m["maxPrice"] = f.MaxPrice
	}
	return m
}

func gradeTrial(td *TrialDetail, expected Expected) {
	td.Outcome = GradeOutcome(td.Actual, expected)
	td.Ground = GradeGroundedness(td.Actual, expected)
	td.Traj = GradeTrajectory(td.Actual, expected)
	td.Pass = td.InfraError == "" && td.Outcome.Pass && td.Ground.Pass && td.Traj.Pass
}
