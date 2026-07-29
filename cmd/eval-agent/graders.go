package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// GradeOutcome 检查任务结果：filter、店铺约束、profile_after、预算上限
func GradeOutcome(actual OutcomeActual, expected Expected) GradeResult {
	r := GradeResult{Name: "outcome", Pass: true}
	var fails []string

	for k, want := range expected.FilterContains {
		got, ok := actual.Filter[k]
		if !ok || fmt.Sprint(got) != fmt.Sprint(want) {
			fails = append(fails, fmt.Sprintf("filter %s: want %v got %v", k, want, got))
		}
	}
	if len(expected.AllowedShopIDs) > 0 {
		if len(actual.CitedShopIDs) == 0 && !expected.ExpectNoResults {
			fails = append(fails, "expected at least one cited shop")
		}
		allowed := toSet(expected.AllowedShopIDs)
		hit := false
		for _, id := range actual.CitedShopIDs {
			if allowed[id] {
				hit = true
			}
		}
		// allowed_shop_ids is a positive relevance set, not an exhaustive
		// deny-list. A grounded comparison may cite a neutral alternative;
		// cases that must reject a specific hard negative use
		// forbidden_shop_ids below.
		if len(actual.CitedShopIDs) > 0 && !hit {
			fails = append(fails, "no cited shop matched allowed set")
		}
	}
	for _, id := range expected.ForbiddenShopIDs {
		for _, c := range actual.CitedShopIDs {
			if c == id {
				fails = append(fails, fmt.Sprintf("forbidden shop %d cited", id))
			}
		}
	}
	if expected.ExpectNoResults && len(actual.CitedShopIDs) > 0 {
		fails = append(fails, "expected no results but cited shops")
	}
	if expected.ProfileAfter != nil {
		fails = append(fails, diffProfile(actual.ProfileAfter, expected.ProfileAfter)...)
	}
	if expected.MaxSteps > 0 && actual.Steps > expected.MaxSteps {
		fails = append(fails, fmt.Sprintf("steps %d > max %d", actual.Steps, expected.MaxSteps))
	}
	if expected.MaxToolCalls > 0 && actual.ToolCalls > expected.MaxToolCalls {
		fails = append(fails, fmt.Sprintf("tool_calls %d > max %d", actual.ToolCalls, expected.MaxToolCalls))
	}

	if len(fails) > 0 {
		r.Pass = false
		r.Reasons = fails
	}
	return r
}

// GradeGroundedness 引用必须 ⊆ observed
func GradeGroundedness(actual OutcomeActual, expected Expected) GradeResult {
	r := GradeResult{Name: "groundedness", Pass: true}
	if expected.ExpectGroundedness != nil && !*expected.ExpectGroundedness {
		return r
	}
	obs := toSet(actual.ObservedShopIDs)
	var fails []string
	if len(expected.AllowedShopIDs) > 0 && !expected.ExpectNoResults && len(actual.CitedShopIDs) == 0 {
		fails = append(fails, "successful shop answer requires at least one [shop:id] citation")
	}
	for _, id := range actual.CitedShopIDs {
		if !obs[id] {
			fails = append(fails, fmt.Sprintf("cited shop %d not in observed", id))
		}
	}
	if len(fails) > 0 {
		r.Pass = false
		r.Reasons = fails
	}
	return r
}

// GradeTrajectory 步数/工具数/重复调用
func GradeTrajectory(actual OutcomeActual, expected Expected) GradeResult {
	r := GradeResult{Name: "trajectory", Pass: true}
	var fails []string
	maxSteps := expected.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 3
	}
	maxTools := expected.MaxToolCalls
	if maxTools <= 0 {
		maxTools = 5
	}
	if actual.Steps > maxSteps {
		fails = append(fails, fmt.Sprintf("steps %d > %d", actual.Steps, maxSteps))
	}
	if actual.ToolCalls > maxTools {
		fails = append(fails, fmt.Sprintf("tool_calls %d > %d", actual.ToolCalls, maxTools))
	}
	if actual.DuplicateToolCalls > 0 {
		fails = append(fails, fmt.Sprintf("duplicate tool calls: %d", actual.DuplicateToolCalls))
	}
	if len(fails) > 0 {
		r.Pass = false
		r.Reasons = fails
	}
	return r
}

// AggregateTrials 汇总多 trial 成功率
func AggregateTrials(trialPasses []bool) TrialAggregate {
	n := len(trialPasses)
	ok := 0
	for _, p := range trialPasses {
		if p {
			ok++
		}
	}
	rate := 0.0
	if n > 0 {
		rate = float64(ok) / float64(n)
	}
	return TrialAggregate{Trials: n, Successes: ok, SuccessRate: rate}
}

func toSet(ids []int64) map[int64]bool {
	m := map[int64]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func diffProfile(actual map[string]any, want map[string]any) []string {
	var fails []string
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		wb, _ := json.Marshal(want[k])
		ab, _ := json.Marshal(actual[k])
		if string(wb) != string(ab) {
			// special: budget_max null vs missing
			if k == "budget_max" && (want[k] == nil || fmt.Sprint(want[k]) == "<nil>") {
				if actual[k] == nil || fmt.Sprint(actual[k]) == "<nil>" || fmt.Sprint(actual[k]) == "0" {
					continue
				}
			}
			fails = append(fails, fmt.Sprintf("profile.%s want %s got %s", k, string(wb), string(ab)))
		}
	}
	return fails
}
