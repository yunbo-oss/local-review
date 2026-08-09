package logic

import (
	"strings"
	"unicode/utf8"
)

// RecommendRoute 生产路径
type RecommendRoute string

const (
	RouteRAGOneshot     RecommendRoute = "rag_oneshot"
	RouteAgentMultistep RecommendRoute = "agent_multistep"
	RouteAgentMemory    RecommendRoute = "agent_memory"
	RouteClarify        RecommendRoute = "clarify"
)

// RouteDecision 路由结果
type RouteDecision struct {
	Route      RecommendRoute `json:"route"`
	Reason     string         `json:"reason"`
	Confidence float64        `json:"confidence"`
	Forced     bool           `json:"forced"`
}

// RouteInput 路由输入
type RouteInput struct {
	Question   string
	ForceRoute string
	HasHistory bool // 同 session 已有多轮时可提示记忆路径
}

// RecommendRouter 规则路由（Phase A）；embedding 级联留后
type RecommendRouter interface {
	Route(in RouteInput) RouteDecision
}

type ruleRecommendRouter struct{}

// NewRecommendRouter 创建规则路由器
func NewRecommendRouter() RecommendRouter {
	return &ruleRecommendRouter{}
}

var memoryCorrectHints = []string{
	"忘掉", "忘记", "清空预算", "取消预算", "不要预算", "预算作废",
	"改成", "换成", "纠正", "不要海淀", "不要朝阳",
}

var multistepHints = []string{
	"评价", "点评", "对比", "比较", "营业时间", "地址", "详情",
	"为什么推荐", "哪家更好", "看看评价", "口碑",
}

var sessionFollowupHints = []string{"还是", "上次", "那种", "那个", "这家", "那家", "继续", "沿用"}

var bareAmbiguousQuestions = map[string]struct{}{
	"那个": {}, "这个": {}, "那家": {}, "这家": {}, "还有吗": {},
	"帮我看看": {}, "帮我找那个": {}, "推荐一下": {}, "随便": {},
	"附近那家怎么样": {}, "这家呢": {},
}

// NeedsSessionHistory 表示这类问法的路由结果依赖当前 session 是否有上下文。
// 调用方可据此避免为每个一次性 RAG 请求都额外访问记忆存储。
func NeedsSessionHistory(question string) bool {
	for _, hint := range sessionFollowupHints {
		if strings.Contains(strings.TrimSpace(question), hint) {
			return true
		}
	}
	return false
}

func routingIntent(question string) string {
	q := strings.TrimSpace(question)
	// 当用户明确区分“引用的脏文本”和“实际需求”时，只使用后半段做路由，
	// 避免评论中的“忘掉预算/比较所有店”劫持规则分类器。
	for _, marker := range []string{"实际需求是", "真实需求是", "我真正要的是"} {
		if idx := strings.LastIndex(q, marker); idx >= 0 {
			if suffix := strings.TrimSpace(q[idx+len(marker):]); suffix != "" {
				return suffix
			}
		}
	}
	return q
}

func isBareAmbiguousQuestion(question string) bool {
	q := strings.Trim(strings.TrimSpace(question), "，。！？!?；;：: ")
	_, ok := bareAmbiguousQuestions[q]
	return ok
}

// Route 规则分流；force_route 合法则覆盖
func (r *ruleRecommendRouter) Route(in RouteInput) RouteDecision {
	if fr := normalizeForce(in.ForceRoute); fr != "" {
		return RouteDecision{Route: fr, Reason: "force", Confidence: 1, Forced: true}
	}
	q := routingIntent(in.Question)
	if q == "" {
		return RouteDecision{Route: RouteClarify, Reason: "empty_question", Confidence: 1}
	}
	// 过短且无明确条件 → clarify（A 期可被调用方映射为 rag）
	if utf8.RuneCountInString(q) < 4 {
		return RouteDecision{Route: RouteClarify, Reason: "too_short", Confidence: 0.6}
	}
	lower := q
	for _, h := range memoryCorrectHints {
		if strings.Contains(lower, h) {
			return RouteDecision{Route: RouteAgentMemory, Reason: "pref_correct", Confidence: 1}
		}
	}
	for _, h := range multistepHints {
		if strings.Contains(lower, h) {
			return RouteDecision{Route: RouteAgentMultistep, Reason: "needs_detail", Confidence: 1}
		}
	}
	if NeedsSessionHistory(q) {
		if in.HasHistory {
			return RouteDecision{Route: RouteAgentMemory, Reason: "session_followup", Confidence: 0.8}
		}
		if isBareAmbiguousQuestion(q) {
			return RouteDecision{Route: RouteClarify, Reason: "missing_session_context", Confidence: 0.9}
		}
	}
	if isBareAmbiguousQuestion(q) {
		return RouteDecision{Route: RouteClarify, Reason: "underspecified", Confidence: 0.8}
	}
	return RouteDecision{Route: RouteRAGOneshot, Reason: "clear_oneshot", Confidence: 0.9}
}

func normalizeForce(s string) RecommendRoute {
	switch strings.TrimSpace(s) {
	case string(RouteRAGOneshot):
		return RouteRAGOneshot
	case string(RouteAgentMultistep):
		return RouteAgentMultistep
	case string(RouteAgentMemory):
		return RouteAgentMemory
	case string(RouteClarify):
		return RouteClarify
	default:
		return ""
	}
}

// IsAgentRoute 是否走完整助手
func IsAgentRoute(r RecommendRoute) bool {
	return r == RouteAgentMultistep || r == RouteAgentMemory
}
