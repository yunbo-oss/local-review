package model

import "time"

const (
	NORMAL     = 0 // 正常
	REPORTED   = 1 // 被举报
	PROHIBITED = 2 // 被禁止
)

type BlogComments struct {
	Id         int64     `gorm:"primary;AUTO_INCREMENT;column:id" json:"id"`
	UserId     int64     `gorm:"column:user_id" json:"userId"`
	BlogId     int64     `gorm:"column:blog_id" json:"blogId"`
	ParentId   int64     `gorm:"column:parent_id" json:"parentId"`
	AnswerId   int64     `gorm:"column:answer_id" json:"answerId"`
	Content    string    `gorm:"column:content" json:"content"`
	Liked      int       `gorm:"column:liked" json:"liked"`
	Status     int       `gorm:"column:status" json:"status"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}
