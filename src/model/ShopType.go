package model

import "time"

const SHOP_TYPE_TABLE_NAME = "tb_shop_type"

type ShopType struct {
	Id         int64     `grom:"primary;AUTO_INCREMENT;column:id" json:"id"`
	Name       string    `gorm:"column:name" json:"name"`
	Icon       string    `gorm:"column:icon" json:"icon"`
	Sort       int       `gorm:"column:sort" json:"sort"`
	CreateTime time.Time `gorm:"column:create_time" json:"-"`
	UpdateTime time.Time `gorm:"column:update_time" json:"-"`
}

func (*ShopType) TableName() string {
	return SHOP_TYPE_TABLE_NAME
}
