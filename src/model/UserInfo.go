package model

import (
	_ "gorm.io/gorm"
	"local-review-go/src/config/mysql"
	"time"
)

const USERINFO_TABLE_NAME = "tb_user_info"

type UserInfo struct {
	UserId     int64     `gorm:"column:user_id" json:"userId"`
	City       string    `gorm:"column:city" json:"city"`
	Introduce  string    `gorm:"column:introduce" json:"introduce"`
	Fans       int       `gorm:"column:fans" json:"fans"`
	Followee   int       `gorm:"column:followee" json:"followee"`
	Gender     bool      `gorm:"column:gender" json:"gender"`
	Birthday   time.Time `gorm:"column:birthday" json:"birthday"`
	Credits    int       `gorm:"column:credits" json:"credits"`
	Level      bool      `gorm:"column:level" json:"level"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

func (*UserInfo) TableName() string {
	return USERINFO_TABLE_NAME
}

func (u *UserInfo) GetUserInfoById(id int64) (UserInfo, error) {
	var userInfo UserInfo
	err := mysql.GetMysqlDB().Table(u.TableName()).Where("user_id = ?", id).First(&userInfo).Error
	return userInfo, err
}
