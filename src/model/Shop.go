package model

import (
	"fmt"
	"local-review-go/src/config/mysql"
	"local-review-go/src/utils/redisx"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const SHOP_TABLE_NAME = "tb_shop"

type Shop struct {
	Id         int64     `gorm:"primary;AUTO_INCREMENT;column:id" json:"id"`
	Name       string    `gorm:"column:name" json:"name"`
	TypeId     int64     `gorm:"column:type_id" json:"typeId"`
	Images     string    `gorm:"column:images" json:"images"`
	Area       string    `gorm:"column:area" json:"area"`
	Address    string    `gorm:"column:address" json:"address"`
	X          float64   `gorm:"column:x" json:"x"`
	Y          float64   `gorm:"column:y" json:"y"`
	AvgPrice   int64     `gorm:"column:avg_price" json:"avgPrice"`
	Sold       int       `gorm:"column:sold" json:"sold"`
	Comments   int       `gorm:"column:comments" json:"comments"`
	Score      int       `gorm:"column:score" json:"score"`
	OpenHours  string    `gorm:"column:open_hours" json:"openHours"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
	Distance   float64   `gorm:"-" json:"distance"`
}

func (*Shop) TableName() string {
	return SHOP_TABLE_NAME
}

func (shop *Shop) QueryShopById(id int64) error {
	err := mysql.GetMysqlDB().Model(shop).Where("id = ?", id).First(shop).Error
	return err
}

func (*Shop) QueryShopByIds(ids []int64) ([]Shop, error) {
	if len(ids) == 0 {
		return []Shop{}, nil
	}

	// 1. 构造 FIELD 排序子句
	idStrs := make([]string, len(ids))
	for i, id := range ids {
		idStrs[i] = strconv.FormatInt(id, 10)
	}
	order := fmt.Sprintf("FIELD(id,%s)", strings.Join(idStrs, ","))

	// 2. 一次查询
	var shops []Shop
	err := mysql.GetMysqlDB().
		Where("id IN ?", ids).
		Order(order).
		Find(&shops).Error
	return shops, err
}

func (shop *Shop) SaveShop() error {
	// 设置创建时间和更新时间
	now := time.Now()
	if shop.CreateTime.IsZero() {
		shop.CreateTime = now
	}
	if shop.UpdateTime.IsZero() {
		shop.UpdateTime = now
	}
	err := mysql.GetMysqlDB().Table(shop.TableName()).Create(shop).Error
	return err
}

func (shop *Shop) UpdateShop(tx *gorm.DB) error {
	err := tx.Model(shop).Save(shop).Error
	return err
}

func (shop *Shop) QueryShopByType(typeId int, current int) ([]Shop, error) {
	var shops []Shop
	err := mysql.GetMysqlDB().Table(shop.TableName()).Where("type_id = ?", typeId).Offset((current - 1) * redisx.DEFAULTPAGESIZE).Limit(redisx.DEFAULTPAGESIZE).Find(&shops).Error
	return shops, err
}

func (shop *Shop) QueryShopByName(name string, current int) ([]Shop, error) {
	var shops []Shop
	err := mysql.GetMysqlDB().Table(shop.TableName()).Where("name LIKE ?", name).Offset((current - 1) * redisx.MAXPAGESIZE).Limit(redisx.MAXPAGESIZE).Find(&shops).Error
	return shops, err
}
