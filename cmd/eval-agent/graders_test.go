package main

import "testing"

func TestGradeGroundedness_UnknownShop(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{
		CitedShopIDs:    []int64{99},
		ObservedShopIDs: []int64{1, 2},
	}
	r := GradeGroundedness(actual, Expected{})
	if r.Pass {
		t.Fatal("expected fail for unknown shop")
	}
}

func TestGradeGroundedness_RequiresCitationForShopTask(t *testing.T) {
	t.Parallel()
	r := GradeGroundedness(OutcomeActual{Answer: "推荐一家店"}, Expected{AllowedShopIDs: []int64{26}})
	if r.Pass {
		t.Fatal("shop task without citation must fail groundedness")
	}
}

func TestGradeTrajectory_OverMaxSteps(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{Steps: 4, ToolCalls: 2}
	r := GradeTrajectory(actual, Expected{MaxSteps: 3, MaxToolCalls: 5})
	if r.Pass {
		t.Fatal("expected fail over max steps")
	}
}

func TestGradeOutcome_ProfileMismatch(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{
		ProfileAfter: map[string]any{"preferred_areas": []any{"朝阳区"}},
	}
	r := GradeOutcome(actual, Expected{
		ProfileAfter: map[string]any{"preferred_areas": []any{"海淀区"}},
	})
	if r.Pass {
		t.Fatal("expected profile mismatch fail")
	}
}

func TestGradeOutcome_AllowedIsPositiveSetAndForbiddenIsDenyList(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{CitedShopIDs: []int64{26, 18}}
	if got := GradeOutcome(actual, Expected{AllowedShopIDs: []int64{26}}); !got.Pass {
		t.Fatalf("grounded neutral comparison should not fail positive-set grading: %v", got.Reasons)
	}
	if got := GradeOutcome(actual, Expected{
		AllowedShopIDs: []int64{26}, ForbiddenShopIDs: []int64{18},
	}); got.Pass {
		t.Fatal("explicit forbidden citation must fail")
	}
}

func TestGradeTrajectory_AlternateToolOrderOK(t *testing.T) {
	t.Parallel()
	// 不同工具顺序只要步数/次数合规即通过（grader 不强制顺序）
	actual := OutcomeActual{Steps: 2, ToolCalls: 2, ObservedShopIDs: []int64{1}, CitedShopIDs: []int64{1}}
	r := GradeTrajectory(actual, Expected{MaxSteps: 3, MaxToolCalls: 5})
	if !r.Pass {
		t.Fatalf("unexpected fail: %v", r.Reasons)
	}
	g := GradeGroundedness(actual, Expected{})
	if !g.Pass {
		t.Fatalf("groundedness: %v", g.Reasons)
	}
}

func TestAggregateTrials(t *testing.T) {
	t.Parallel()
	agg := AggregateTrials([]bool{true, false, true})
	if agg.Trials != 3 || agg.Successes != 2 || agg.SuccessRate < 0.66 || agg.SuccessRate > 0.67 {
		t.Fatalf("%+v", agg)
	}
}
