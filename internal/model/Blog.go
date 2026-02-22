package model

import "time"

const BLOG_TABLE_NAME = "tb_blog"

type Blog struct {
	Id         int64     `gorm:"primary_key;AUTO_INCREMENT;column:id" json:"id"`
	ShopId     int64     `gorm:"column:shop_id" json:"shopId"`
	UserId     int64     `gorm:"column:user_id" json:"userId"`
	Icon       string    `gorm:"-" json:"icon"`
	Name       string    `gorm:"-" json:"name"`
	IsLike     bool      `gorm:"-" json:"isLike"`
	Title      string    `gorm:"column:title" json:"title"`
	Images     string    `gorm:"column:images" json:"images"`
	Content    string    `gorm:"column:content" json:"content"`
	Liked      int       `gorm:"column:liked" json:"liked"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

func (*Blog) TableName() string {
	return BLOG_TABLE_NAME
}
