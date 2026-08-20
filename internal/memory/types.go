package memory

import "time"

// Profile 长期结构化偏好
type Profile struct {
	PreferredAreas []string                      `json:"preferred_areas"`
	PreferredTypes []string                      `json:"preferred_types"`
	BudgetMax      *int64                        `json:"budget_max,omitempty"` // nil=未设；0=已清空；>0=上限
	Dislikes       []string                      `json:"dislikes"`
	Summary        string                        `json:"summary"`
	Version        int64                         `json:"version"`
	UpdatedAt      int64                         `json:"updated_at"` // Unix 秒
	PreferenceMeta map[string]PreferenceMetadata `json:"preference_meta,omitempty"`
}

type PreferenceMetadata struct {
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	UpdatedAt  int64   `json:"updated_at"`
}

// Message 会话消息
type Message struct {
	Role    string `json:"role"` // user|assistant
	Content string `json:"content"`
	Ts      int64  `json:"ts"`
}

// ProfilePatch 增量偏好补丁（非持久化）
type ProfilePatch struct {
	PreferredAreasAdd    []string `json:"preferred_areas_add,omitempty"`
	PreferredAreasRemove []string `json:"preferred_areas_remove,omitempty"`
	PreferredTypesAdd    []string `json:"preferred_types_add,omitempty"`
	PreferredTypesRemove []string `json:"preferred_types_remove,omitempty"`
	DislikesAdd          []string `json:"dislikes_add,omitempty"`
	DislikesRemove       []string `json:"dislikes_remove,omitempty"`
	BudgetMax            *int64   `json:"budget_max,omitempty"` // nil=不变；0=清空；>0=覆盖
	Summary              *string  `json:"summary,omitempty"`
	Source               string   `json:"-"`
	Confidence           float64  `json:"-"`
}

// SessionSummary is the episodic memory layer. ThroughTs marks the newest
// message included so recent working memory can be kept separately.
type SessionSummary struct {
	Content   string `json:"content"`
	ThroughTs int64  `json:"through_ts"`
	Version   int64  `json:"version"`
	UpdatedAt int64  `json:"updated_at"`
}

type LayeredContext struct {
	SemanticProfile Profile
	EpisodicSummary SessionSummary
	Relevant        []Message
	Working         []Message
}

// NowUnix 便于测试注入
var NowUnix = func() int64 { return time.Now().Unix() }

const MaxSummaryRunes = 80
