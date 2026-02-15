package model

import "time"

type Follow struct {
	Id           int64     `gorm:"primary;AUTO_INCREMENT;column:id" json:"id"`
	UserId       int64     `gorm:"column:user_id" json:"userId"`              // 关注者
	FollowUserId int64     `gorm:"column:follow_user_id" json:"followUserId"` // 被关注者
	CreateTime   time.Time `gorm:"column:create_time" json:"createTime"`
}

func (*Follow) TableName() string {
	return "tb_follow"
}
