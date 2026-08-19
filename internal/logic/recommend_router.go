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
	Question       string
	ForceRoute     string
	HasHistory     bool // 同 session 已有多轮时可提示记忆路径
	ProfileSummary string
	HistorySummary string
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

var preferenceFields = []string{
	"偏好", "预算", "价格", "人均", "区域", "地区", "类别", "类型", "口味",
}

var preferenceDeleteHints = []string{
	"忘掉", "忘记", "去掉", "删掉", "删除", "清除", "清空", "取消", "作废",
	"不再限制", "不用考虑", "不设限制", "不限制", "不要",
}

var preferenceUpdateHints = []string{
	"改成", "改为", "改到", "调整到", "换成", "换为", "改去", "纠正",
	"以后优先", "以后只", "以后不要", "记住以后", "设为偏好",
}

var compareHints = []string{
	"对比", "比较", "横向", "优缺点", "长处和短板", "长处与短板",
	"二选一", "选谁", "哪家更好", "哪个更好", "哪一个更", "分别有什么",
	"逐一说说", "逐项比", "更值得选",
}

var detailHints = []string{
	"营业时间", "打烊时间", "几点关门", "开到几点", "几点打烊",
	"具体地址", "详细地址", "怎么走", "怎么到", "具体在哪", "店在哪里",
	"店的地址", "门店地址", "联系电话", "电话号码", "门店详情",
}

var evidenceHints = []string{
	"核实", "核验", "核查", "查证", "查清", "验证", "证据", "依据",
	"是否真的", "真假", "互相矛盾", "相互矛盾", "评论判断", "评价判断",
	"看看评价", "查评价", "看看评论", "查评论", "用户反馈", "评价再", "口碑再",
	"评价怎么样", "评论怎么样", "口碑怎么样", "哪些评价",
}

var explanationHints = []string{
	"为什么推荐", "说明理由", "给出理由", "推荐理由", "推荐依据",
	"值不值得", "别只给结论", "分析这家店", "分析这家门店",
}

var sessionFollowupHints = []string{
	"上次", "上一轮", "刚才", "刚刚", "之前", "前面提到", "原来的",
	"那种", "那个", "这家", "那家", "第一个", "第二个", "第三个",
	"第一家", "第二家", "第三家", "前两家", "继续", "接着", "沿用",
	"再来一个", "换个相似", "换个更", "按原条件", "照之前", "按之前",
}

var bareAmbiguousQuestions = map[string]struct{}{
	"那个": {}, "这个": {}, "那家": {}, "这家": {}, "还有吗": {},
	"帮我看看": {}, "帮我找那个": {}, "推荐一下": {}, "随便": {},
	"附近那家怎么样": {}, "附近的怎么样": {}, "这家呢": {}, "它呢": {},
	"再来一个": {}, "你看着办": {}, "前面那个": {}, "刚才那家呢": {},
	"还是原来的": {}, "帮我选": {}, "哪一个": {}, "随意推荐": {}, "继续": {},
}

// NeedsSessionHistory 表示这类问法的路由结果依赖当前 session 是否有上下文。
// 调用方可据此避免为每个一次性 RAG 请求都额外访问记忆存储。
func NeedsSessionHistory(question string) bool {
	question = routingIntent(question)
	if strings.HasPrefix(question, "还是") {
		return true
	}
	for _, hint := range sessionFollowupHints {
		if strings.Contains(question, hint) {
			return true
		}
	}
	for _, hint := range []string{"还是上次", "还是那种", "还是原来", "上一家", "下一家"} {
		if strings.Contains(question, hint) {
			return true
		}
	}
	return false
}

func routingIntent(question string) string {
	q := strings.TrimSpace(question)
	// 当用户明确区分“引用的脏文本”和“实际需求”时，只使用后半段做路由，
	// 避免评论中的“忘掉预算/比较所有店”劫持规则分类器。
	for _, marker := range []string{
		"但我的实际需求只是", "我的实际需求只是", "但我的实际需求是", "我的实际需求是",
		"实际需求只是", "实际需求就是", "实际需求是",
		"真正想找的是", "真正想找的只是", "真实想找的是",
		"我真正要的只是", "我真正要的是", "真实需求只是", "真实需求是",
		"但我只是想", "我只是想", "但我只想", "我只想",
	} {
		if idx := strings.LastIndex(q, marker); idx >= 0 {
			if suffix := strings.TrimSpace(q[idx+len(marker):]); suffix != "" {
				return suffix
			}
		}
	}
	return q
}

func containsAny(question string, hints []string) bool {
	for _, hint := range hints {
		if strings.Contains(question, hint) {
			return true
		}
	}
	return false
}

func hasPreferenceMutation(question string) bool {
	if containsAny(question, preferenceUpdateHints) {
		return true
	}
	if !containsAny(question, preferenceFields) {
		return false
	}
	return containsAny(question, preferenceDeleteHints)
}

func hasMultistepIntent(question string) bool {
	return containsAny(question, compareHints) ||
		containsAny(question, detailHints) ||
		containsAny(question, evidenceHints) ||
		containsAny(question, explanationHints)
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
	if hasPreferenceMutation(q) {
		return RouteDecision{Route: RouteAgentMemory, Reason: "preference_mutation", Confidence: 1}
	}
	if hasMultistepIntent(q) {
		return RouteDecision{Route: RouteAgentMultistep, Reason: "needs_tools", Confidence: 0.95}
	}
	if NeedsSessionHistory(q) {
		if in.HasHistory {
			return RouteDecision{Route: RouteAgentMemory, Reason: "session_followup", Confidence: 0.9}
		}
		return RouteDecision{Route: RouteClarify, Reason: "missing_session_context", Confidence: 0.9}
	}
	// 过短且无明确条件 → clarify
	if utf8.RuneCountInString(q) < 4 {
		return RouteDecision{Route: RouteClarify, Reason: "too_short", Confidence: 0.8}
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
