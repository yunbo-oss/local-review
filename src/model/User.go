package model

import (
	"gorm.io/gorm/clause"
	"local-review-go/src/config/mysql"
	"time"
)

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

func (user *User) GetUserById(id int64) (User, error) {
	var u User
	err := mysql.GetMysqlDB().Table(user.TableName()).Where("id = ?", id).First(&u).Error
	return u, err
}

func (user *User) GetUserByPhone(phone string) error {
	err := mysql.GetMysqlDB().Table(user.TableName()).Where("phone = ?", phone).First(user).Error
	return err
}

func (user *User) SaveUser() error {
	err := mysql.GetMysqlDB().Table(user.TableName()).Create(user).Error
	return err
}

func (user *User) GetUsersByIds(ids []int64) ([]User, error) {
	var users []User

	err := mysql.GetMysqlDB().
		Table(user.TableName()).
		Where("id IN ?", ids).
		Order(clause.Expr{SQL: "FIELD(id, ?)", Vars: []interface{}{ids}, WithoutParentheses: false}).
		Find(&users).Error

	return users, err
}
