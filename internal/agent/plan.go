package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local-review-go/internal/llm"

	"github.com/sashabaranov/go-openai"
)

type PlanAction string

const (
	PlanSearchShops   PlanAction = "search_shops"
	PlanGetShop       PlanAction = "get_shop"
	PlanListReviews   PlanAction = "list_shop_blogs"
	PlanAnswer        PlanAction = "answer"
	PlanClarify       PlanAction = "clarify"
	DefaultMaxReplans            = 1
)

type PlanStep struct {
	ID            string     `json:"id"`
	Action        PlanAction `json:"action"`
	DependsOn     []string   `json:"depends_on"`
	ParallelGroup string     `json:"parallel_group"`
	TargetCount   int        `json:"target_count"`
	Query         string     `json:"query"`
	Rationale     string     `json:"rationale"`
}

type ExecutionPlan struct {
	Version        int        `json:"version"`
	Goal           string     `json:"goal"`
	Steps          []PlanStep `json:"steps"`
	StopConditions []string   `json:"stop_conditions"`
	Source         string     `json:"source"`
}

type PlanInput struct {
	Intent         IntentSpec
	ProfileSummary string
	HistorySummary string
}

type ReplanInput struct {
	Plan        ExecutionPlan
	Intent      IntentSpec
	Observation string
	Reason      string
}

type Planner interface {
	Plan(ctx context.Context, in PlanInput) (ExecutionPlan, llm.TokenUsage, error)
	Replan(ctx context.Context, in ReplanInput) (ExecutionPlan, llm.TokenUsage, error)
}

type llmPlanner struct {
	chat llm.ChatClient
}

func NewLLMPlanner(chat llm.ChatClient) Planner {
	if chat == nil {
		return nil
	}
	return &llmPlanner{chat: chat}
}

const plannerPrompt = `你是一个有界本地生活推荐 Agent 的 Planner。输入包含统一 IntentSpec 和已有记忆摘要。输出可审计、可执行的 JSON 计划，不要输出 Markdown。

可用 action：search_shops、get_shop、list_shop_blogs、answer、clarify。
规则：
1. 需要推荐、比较或查店时必须先 search_shops；地址、价格、营业时间等结构化事实使用 get_shop；体验、评价核验和比较使用 list_shop_blogs。
2. get_shop/list_shop_blogs 不填写具体店铺 ID，由 Executor 在 search 结果中选择；target_count 为 1~3。
3. 相互独立的 get_shop 与 list_shop_blogs 使用相同 parallel_group，允许并行执行。
4. 不得放宽 IntentSpec 中的硬条件。无结果时可以换用 rewritten_queries，仍无结果则 answer 或 clarify。
5. 最多 7 个步骤，最后一步必须是 answer 或 clarify。
6. stop_conditions 给出“何时不再继续调用工具”的明确条件。

JSON：{"version":1,"goal":"","steps":[{"id":"s1","action":"search_shops","depends_on":[],"parallel_group":"","target_count":1,"query":"","rationale":""}],"stop_conditions":[]}`

const replanPrompt = `你是有界推荐 Agent 的 Replanner。根据原计划、不可更改的 IntentSpec、执行观察和触发原因，最多生成一次修订计划。
不得放宽区域、类别、预算、评分等硬条件；不得重复已经没有结果的相同查询；只能使用 search_shops、get_shop、list_shop_blogs、answer、clarify。最后一步必须是 answer 或 clarify。只输出 JSON。`

func (p *llmPlanner) Plan(ctx context.Context, in PlanInput) (ExecutionPlan, llm.TokenUsage, error) {
	payload, _ := json.Marshal(in)
	return p.complete(ctx, plannerPrompt, string(payload), in.Intent)
}

func (p *llmPlanner) Replan(ctx context.Context, in ReplanInput) (ExecutionPlan, llm.TokenUsage, error) {
	payload, _ := json.Marshal(in)
	plan, usage, err := p.complete(ctx, replanPrompt, string(payload), in.Intent)
	if err == nil {
		plan.Version = in.Plan.Version + 1
		plan.Source = "llm_replan"
	}
	return plan, usage, err
}

func (p *llmPlanner) complete(ctx context.Context, system, user string, intent IntentSpec) (ExecutionPlan, llm.TokenUsage, error) {
	if p == nil || p.chat == nil {
		return ExecutionPlan{}, llm.TokenUsage{}, fmt.Errorf("planner not configured")
	}
	raw, usage, err := p.chat.ChatCompleteWithUsage(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: system},
		{Role: openai.ChatMessageRoleUser, Content: user},
	})
	if err != nil {
		return ExecutionPlan{}, usage, err
	}
	plan, err := ParseExecutionPlan(raw, intent)
	if err != nil {
		return ExecutionPlan{}, usage, err
	}
	plan.Source = "llm"
	return plan, usage, nil
}

func ParseExecutionPlan(raw string, intent IntentSpec) (ExecutionPlan, error) {
	if start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var plan ExecutionPlan
	if err := dec.Decode(&plan); err != nil {
		return ExecutionPlan{}, fmt.Errorf("parse execution plan: %w", err)
	}
	if err := sanitizeExecutionPlan(&plan, intent); err != nil {
		return ExecutionPlan{}, err
	}
	return plan, nil
}

func sanitizeExecutionPlan(plan *ExecutionPlan, intent IntentSpec) error {
	if plan.Version <= 0 {
		plan.Version = 1
	}
	plan.Goal = truncateIntentText(plan.Goal, 160)
	if plan.Goal == "" {
		plan.Goal = truncateIntentText(intent.OriginalQuestion, 160)
	}
	allowed := map[PlanAction]bool{
		PlanSearchShops: true, PlanGetShop: true, PlanListReviews: true,
		PlanAnswer: true, PlanClarify: true,
	}
	seen := map[string]struct{}{}
	steps := make([]PlanStep, 0, len(plan.Steps)+2)
	for _, step := range plan.Steps {
		step.ID = strings.TrimSpace(step.ID)
		if step.ID == "" || !allowed[step.Action] {
			continue
		}
		if _, duplicate := seen[step.ID]; duplicate {
			continue
		}
		seen[step.ID] = struct{}{}
		if step.TargetCount <= 0 {
			step.TargetCount = 1
		}
		if step.TargetCount > 3 {
			step.TargetCount = 3
		}
		step.Query = truncateIntentText(step.Query, 180)
		step.Rationale = truncateIntentText(step.Rationale, 100)
		steps = append(steps, step)
		if len(steps) >= 7 {
			break
		}
	}
	needsSearch := intent.Route != "clarify" && intent.Intent != "preference_update"
	hasSearch := false
	for _, step := range steps {
		if step.Action == PlanSearchShops {
			hasSearch = true
			break
		}
	}
	if needsSearch && !hasSearch {
		steps = append([]PlanStep{{
			ID: "s0", Action: PlanSearchShops, TargetCount: 3,
			Query: firstRetrievalQuery(intent), Rationale: "获取候选店铺",
		}}, steps...)
	}
	if len(steps) == 0 || (steps[len(steps)-1].Action != PlanAnswer && steps[len(steps)-1].Action != PlanClarify) {
		steps = append(steps, PlanStep{ID: uniquePlanID(steps, "answer"), Action: PlanAnswer, Rationale: "基于已获取证据回答"})
	}
	if len(steps) > 7 {
		steps = steps[:7]
		steps[len(steps)-1] = PlanStep{ID: uniquePlanID(steps[:len(steps)-1], "answer"), Action: PlanAnswer}
	}
	plan.Steps = steps
	plan.StopConditions = compactIntentStrings(plan.StopConditions, 6, 100)
	return nil
}

func firstRetrievalQuery(intent IntentSpec) string {
	queries := intent.RetrievalQueries()
	if len(queries) > 1 {
		return queries[1]
	}
	if len(queries) == 1 {
		return queries[0]
	}
	return intent.OriginalQuestion
}

func uniquePlanID(steps []PlanStep, base string) string {
	seen := map[string]struct{}{}
	for _, step := range steps {
		seen[step.ID] = struct{}{}
	}
	if _, ok := seen[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, ok := seen[candidate]; !ok {
			return candidate
		}
	}
}

func FallbackExecutionPlan(intent IntentSpec) ExecutionPlan {
	steps := []PlanStep{{
		ID: "search", Action: PlanSearchShops, TargetCount: 3,
		Query: firstRetrievalQuery(intent), Rationale: "检索满足硬条件的候选",
	}}
	needsDetail, needsReviews := false, len(intent.SoftPreferences) > 0
	for _, requirement := range intent.EvidenceRequirements {
		needsDetail = needsDetail || requirement == "shop_detail"
		needsReviews = needsReviews || requirement == "reviews"
	}
	if needsDetail {
		steps = append(steps, PlanStep{ID: "details", Action: PlanGetShop, DependsOn: []string{"search"}, ParallelGroup: "evidence", TargetCount: 2})
	}
	if needsReviews {
		steps = append(steps, PlanStep{ID: "reviews", Action: PlanListReviews, DependsOn: []string{"search"}, ParallelGroup: "evidence", TargetCount: 2})
	}
	steps = append(steps, PlanStep{ID: "answer", Action: PlanAnswer, DependsOn: []string{"search"}})
	return ExecutionPlan{
		Version: 1, Goal: intent.OriginalQuestion, Steps: steps,
		StopConditions: []string{"工具预算耗尽", "候选为空且改写查询仍无结果", "证据足以支持或拒绝推荐"},
		Source:         "fallback",
	}
}
