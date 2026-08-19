package model

import "time"

const USER_AGENT_PROFILE_TABLE = "user_agent_profiles"

// UserAgentProfile 长期偏好（PostgreSQL 事实源）
type UserAgentProfile struct {
	UserID      int64     `gorm:"primaryKey;column:user_id" json:"user_id"`
	ProfileJSON string    `gorm:"column:profile_json;type:jsonb;not null" json:"profile_json"`
	Version     int64     `gorm:"column:version;not null;default:0" json:"version"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (*UserAgentProfile) TableName() string { return USER_AGENT_PROFILE_TABLE }

const USER_AGENT_PROFILE_EVENT_TABLE = "user_agent_profile_events"

// UserAgentProfileEvent 偏好变更审计
type UserAgentProfileEvent struct {
	ID         int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID     int64     `gorm:"column:user_id;index;not null" json:"user_id"`
	RunID      int64     `gorm:"column:run_id;index" json:"run_id"`
	PatchJSON  string    `gorm:"column:patch_json;type:jsonb;not null" json:"patch_json"`
	OldVersion int64     `gorm:"column:old_version;not null" json:"old_version"`
	NewVersion int64     `gorm:"column:new_version;not null" json:"new_version"`
	CreatedAt  time.Time `gorm:"column:created_at" json:"created_at"`
}

func (*UserAgentProfileEvent) TableName() string { return USER_AGENT_PROFILE_EVENT_TABLE }
