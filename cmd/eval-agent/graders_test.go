package main

import (
	"strings"
	"testing"
)

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

func TestApplyExperimentTrajectoryContractUsesMeasuredRuntime(t *testing.T) {
	got := applyExperimentTrajectoryContract(Expected{MaxSteps: 3, MaxToolCalls: 5}, ExperimentMeta{
		AgentMaxSteps: 4, AgentMaxTools: 10, AgentRuntimeVersion: "v2_react",
		AgentMaxSearchRounds: 2, AgentMaxReviewPages: 2,
	})
	if got.MaxSteps != 4 || got.MaxToolCalls != 10 || got.RuntimeVersion != "v2_react" ||
		got.MaxSearchRounds != 2 || got.MaxReviewPagesPerShop != 2 || !got.RequireAnswerVerified {
		t.Fatalf("runtime contract was not applied: %+v", got)
	}
}

func TestGradeTrajectorySkipsAgentOnlyContractForRoutedRAG(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{
		Route: logicRouteRAGForTest, Steps: 0, ToolTraceAvailable: true,
		RuntimeVersion: "", AnswerVerified: true,
	}
	expected := Expected{
		RequiredTools:  []string{"search_shops", "list_shop_blogs"},
		RuntimeVersion: "v2_react", RequireAnswerVerified: true,
	}
	got := GradeTrajectory(actual, expected)
	if !got.Pass || len(got.Deferred) != 1 {
		t.Fatalf("RAG route must be judged by task outcome, not Agent tool/runtime conformance: %+v", got)
	}
}

const logicRouteRAGForTest = "rag_oneshot"

func TestV2RuntimeAndEvidenceBudgetsAreGraded(t *testing.T) {
	actual := OutcomeActual{
		Steps: 4, ToolCalls: 8, MaxToolCallsInTurn: 8,
		RuntimeVersion: "v2_react", SearchRounds: 2, MaxReviewPages: 2,
		AnswerVerified: true,
	}
	expected := Expected{
		MaxSteps: 4, MaxToolCalls: 10, RuntimeVersion: "v2_react",
		MaxSearchRounds: 2, MaxReviewPagesPerShop: 2, RequireAnswerVerified: true,
	}
	if got := GradeOutcome(actual, expected); !got.Pass {
		t.Fatalf("outcome=%+v", got)
	}
	if got := GradeTrajectory(actual, expected); !got.Pass {
		t.Fatalf("trajectory=%+v", got)
	}
	actual.MaxReviewPages = 3
	actual.AnswerVerified = false
	if got := GradeOutcome(actual, expected); !got.Pass {
		t.Fatalf("runtime conformance must not contaminate task outcome: %+v", got)
	}
	if got := GradeTrajectory(actual, expected); got.Pass {
		t.Fatal("review-page overflow and unverified answer must fail conformance")
	}
}

func TestGradeOutcomeIgnoresAgentOnlyConformance(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{
		RecommendationHeaderFound: true,
		RuntimeVersion:            "",
		AnswerVerified:            false,
		Steps:                     99,
		ToolCalls:                 99,
	}
	expected := Expected{
		RequireRecommendationHeader: true,
		RuntimeVersion:              "v2_react",
		RequireAnswerVerified:       true,
		MaxSteps:                    4,
		MaxToolCalls:                10,
	}
	if got := GradeOutcome(actual, expected); !got.Pass {
		t.Fatalf("task outcome must contain only user-visible assertions: %+v", got)
	}
	if got := GradeTrajectory(actual, expected); got.Pass {
		t.Fatal("agent-only conformance must still be graded separately")
	}
}

func TestGradeTrajectory_MultiTurnUsesPerTurnToolBound(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{Steps: 3, ToolCalls: 7, MaxToolCallsInTurn: 3}
	if got := GradeTrajectory(actual, Expected{MaxSteps: 3, MaxToolCalls: 5}); !got.Pass {
		t.Fatalf("scenario total is a cost metric, per-turn maximum enforces the loop bound: %v", got.Reasons)
	}
	actual.MaxToolCallsInTurn = 6
	if got := GradeTrajectory(actual, Expected{MaxSteps: 3, MaxToolCalls: 5}); got.Pass {
		t.Fatal("a single turn above the tool bound must fail")
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

func TestGradeOutcome_AllowedOnlyRejectsUnlistedCitation(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{CitedShopIDs: []int64{26, 18}}
	got := GradeOutcome(actual, Expected{AllowedShopIDs: []int64{26}, AllowedOnly: true})
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, " "), "outside exhaustive allowed set") {
		t.Fatalf("allowed_only should reject an unlisted citation: %+v", got)
	}
}

func TestGradeOutcome_AllowedOnlyAcceptsExplicitlyPermittedComparison(t *testing.T) {
	t.Parallel()
	actual := OutcomeActual{CitedShopIDs: []int64{26, 27}}
	got := GradeOutcome(actual, Expected{
		AllowedShopIDs: []int64{26}, PermittedShopIDs: []int64{27}, AllowedOnly: true,
	})
	if !got.Pass {
		t.Fatalf("explicit comparison shop should be permitted: %v", got.Reasons)
	}
}

func TestGradeOutcome_RecommendationAndEvidenceCitationsAreDistinct(t *testing.T) {
	t.Parallel()
	expected := Expected{
		AllowedShopIDs: []int64{26}, AllowedOnly: true,
		ForbiddenShopIDs: []int64{16}, RequireRecommendationHeader: true,
	}
	actual := OutcomeActual{
		CitedShopIDs: []int64{26, 16}, RecommendedShopIDs: []int64{26},
		RecommendationHeaderFound: true,
	}
	if got := GradeOutcome(actual, expected); !got.Pass {
		t.Fatalf("negative evidence citation must not count as recommendation: %v", got.Reasons)
	}

	expected.ForbiddenCitedShopIDs = []int64{16}
	if got := GradeOutcome(actual, expected); got.Pass {
		t.Fatal("security-sensitive forbidden citation should fail")
	}
}

func TestGradeOutcome_RequiresRecommendationHeader(t *testing.T) {
	t.Parallel()
	expected := Expected{AllowedShopIDs: []int64{26}, RequireRecommendationHeader: true}
	actual := OutcomeActual{CitedShopIDs: []int64{26}, RecommendedShopIDs: []int64{26}}
	if got := GradeOutcome(actual, expected); got.Pass {
		t.Fatal("missing recommendation header should fail")
	}
}

func TestGradeOutcome_FactualLookupRequiresCitationNotRecommendation(t *testing.T) {
	t.Parallel()
	expected := Expected{
		RequiredCitedShopIDs: []int64{30}, PermittedShopIDs: []int64{30},
		AllowedOnly: true, RequireRecommendationHeader: true,
	}
	actual := OutcomeActual{
		Answer:       "推荐结果：无\n预约政策无法确认 [shop:30]。",
		CitedShopIDs: []int64{30}, RecommendationHeaderFound: true,
	}
	if got := GradeOutcome(actual, expected); !got.Pass {
		t.Fatalf("factual lookup should not force a recommendation: %v", got.Reasons)
	}
}

func TestGradeOutcome_AnswerAssertions(t *testing.T) {
	t.Parallel()
	expected := Expected{
		RequiredAnswerSubstrings:  []string{"没有合适"},
		ForbiddenAnswerSubstrings: []string{"EVAL_CANARY"},
		RequiredAnswerRegex:       []string{"(没有|未找到).*(日料|店铺)"},
		ForbiddenAnswerRegex:      []string{"(?i)password\\s*="},
	}
	if got := GradeOutcome(OutcomeActual{Answer: "没有合适的日料。"}, expected); !got.Pass {
		t.Fatalf("valid answer unexpectedly failed: %v", got.Reasons)
	}
	for name, answer := range map[string]string{
		"missing":   "建议提高预算",
		"substring": "没有合适的日料，但 EVAL_CANARY",
		"regex":     "没有合适的日料，password = secret",
	} {
		t.Run(name, func(t *testing.T) {
			if got := GradeOutcome(OutcomeActual{Answer: answer}, expected); got.Pass {
				t.Fatalf("answer %q should fail", answer)
			}
		})
	}
}

func TestGradeOutcome_InvalidRegexFailsClosed(t *testing.T) {
	t.Parallel()
	got := GradeOutcome(OutcomeActual{Answer: "anything"}, Expected{RequiredAnswerRegex: []string{"["}})
	if got.Pass || !strings.Contains(strings.Join(got.Reasons, " "), "invalid regex") {
		t.Fatalf("invalid golden regex must fail closed: %+v", got)
	}
}

func TestGradeGroundedness_SeparatesCitationAndClaimCoverage(t *testing.T) {
	t.Parallel()
	expected := Expected{RequiredClaimRegex: []string{"安静|适合办公", "嘈杂|很吵|不适合办公"}}
	got := GradeGroundedness(OutcomeActual{
		Answer: "评价说这里安静 [shop:26]。", CitedShopIDs: []int64{26}, ObservedShopIDs: []int64{26},
	}, expected)
	if got.Pass || got.Components["citation_legality"] != "pass" || got.Components["claim_coverage"] != "fail" {
		t.Fatalf("expected legal citation but incomplete claim coverage: %+v", got)
	}
	got = GradeGroundedness(OutcomeActual{
		Answer:       "评价既有安静反馈，也有人说高峰期嘈杂 [shop:26]。",
		CitedShopIDs: []int64{26}, ObservedShopIDs: []int64{26},
	}, expected)
	if !got.Pass || got.Components["citation_legality"] != "pass" || got.Components["claim_coverage"] != "pass" {
		t.Fatalf("complete grounded answer should pass: %+v", got)
	}
}

func TestGradeGroundedness_MarksUnassertedClaimCoverageNotEvaluated(t *testing.T) {
	t.Parallel()
	got := GradeGroundedness(OutcomeActual{}, Expected{})
	if !got.Pass || got.Components["claim_coverage"] != "not_evaluated" {
		t.Fatalf("unasserted claim coverage must not be overstated as passing: %+v", got)
	}
}

func TestGradeTrajectory_RequiredTools(t *testing.T) {
	t.Parallel()
	expected := Expected{RequiredTools: []string{"search_shops", "list_shop_reviews"}}
	deferred := GradeTrajectory(OutcomeActual{Steps: 1, ToolCalls: 2}, expected)
	if !deferred.Pass || len(deferred.Deferred) != 1 {
		t.Fatalf("missing tool-name telemetry should defer, not invent a failure: %+v", deferred)
	}
	missing := GradeTrajectory(OutcomeActual{
		Steps: 1, ToolCalls: 1, ToolTraceAvailable: true, ToolNames: []string{"search_shops"},
	}, expected)
	if missing.Pass {
		t.Fatal("available tool trace must enforce required tools")
	}
	passed := GradeTrajectory(OutcomeActual{
		Steps: 1, ToolCalls: 2, ToolTraceAvailable: true,
		ToolNames: []string{"search_shops", "list_shop_reviews"},
	}, expected)
	if !passed.Pass || len(passed.Deferred) != 0 {
		t.Fatalf("required tools were observed: %+v", passed)
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
