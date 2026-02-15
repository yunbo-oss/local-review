package model

import "time"

type User struct {
	Id         int64     `gorm:"primary;AUTO_INCREMENT;column:id" json:"id"`
	Phone      string    `gorm:"column:phone" json:"phone"`
	Password   string    `gorm:"column:password" json:"password"`
	NickName   string    `gorm:"column:nick_name" json:"nickName"`
	Icon       string    `gorm:"column:icon" json:"icon"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

func (*User) TableName() string {
	return "tb_user"
}
