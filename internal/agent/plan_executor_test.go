package agent

import (
	"context"
	"strings"
	"testing"

	"local-review-go/internal/llm"
)

type plannerStub struct {
	initial ExecutionPlan
	revised ExecutionPlan
	replans int
}

func (p *plannerStub) Plan(context.Context, PlanInput) (ExecutionPlan, llm.TokenUsage, error) {
	return p.initial, llm.TokenUsage{TotalTokens: 20}, nil
}
func (p *plannerStub) Replan(context.Context, ReplanInput) (ExecutionPlan, llm.TokenUsage, error) {
	p.replans++
	return p.revised, llm.TokenUsage{TotalTokens: 10}, nil
}

type queryPlanSearch struct{}

func (queryPlanSearch) SearchShops(_ context.Context, query, _, _ string, _, _ *int64, _ int) ([]ShopHit, error) {
	if strings.Contains(query, "retry") {
		return []ShopHit{{ShopID: 7, Name: "重规划候选", Area: "海淀区", TypeName: "咖啡", AvgPrice: 66, Score: 46}}, nil
	}
	return nil, nil
}

func TestRunPlannedReplansAfterEmptyObservation(t *testing.T) {
	planner := &plannerStub{
		initial: ExecutionPlan{Version: 1, Goal: "找店", Steps: []PlanStep{
			{ID: "s1", Action: PlanSearchShops, Query: "none"}, {ID: "a1", Action: PlanAnswer},
		}},
		revised: ExecutionPlan{Version: 2, Goal: "换查询", Steps: []PlanStep{
			{ID: "s2", Action: PlanSearchShops, Query: "retry"}, {ID: "a2", Action: PlanAnswer},
		}},
	}
	client := &scriptedClient{turns: []llm.AssistantTurn{{
		Message: llm.ChatMessage{Role: "assistant", Content: `{"no_result":false,"summary":"找到满足条件的候选","recommendations":[{"shop_id":7,"claims":[{"text":"位于海淀区","field":"area","value":"海淀区","evidence_refs":["shop:7.area"]}]}]}`},
		Usage:   llm.TokenUsage{TotalTokens: 30},
	}}}
	exec := &ToolExecutor{Search: queryPlanSearch{}, Ledger: NewEvidenceLedger(), Observed: map[int64]struct{}{}}
	intent := FallbackIntentSpec("海淀咖啡", "agent")
	res := RunPlanned(context.Background(), client, planner, exec, DefaultRunConfig(),
		[]llm.ChatMessage{{Role: "user", Content: "海淀咖啡"}}, PlanInput{Intent: intent})
	if !res.GroundingOK || res.Replans != 1 || planner.replans != 1 {
		t.Fatalf("planned result=%+v planner=%+v", res, planner)
	}
	if res.ToolCalls != 2 || len(res.Plans) != 2 || res.Usage.TotalTokens != 60 {
		t.Fatalf("trace/counts not preserved: %+v", res)
	}
}

func TestParseExecutionPlanAddsRequiredBoundarySteps(t *testing.T) {
	intent := IntentSpec{Intent: "compare", Route: "agent", OriginalQuestion: "比较两家"}
	plan, err := ParseExecutionPlan(`{"version":1,"goal":"比较","steps":[{"id":"reviews","action":"list_shop_blogs","depends_on":[],"parallel_group":"e","target_count":9,"query":"","rationale":"核验"}],"stop_conditions":[]}`, intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Action != PlanSearchShops || plan.Steps[1].TargetCount != 3 || plan.Steps[2].Action != PlanAnswer {
		t.Fatalf("sanitized plan=%+v", plan)
	}
}
