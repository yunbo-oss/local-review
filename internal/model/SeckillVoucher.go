package model

import (
	"errors"
	"time"
)

const SECKILL_VOUCHER_NAME = "tb_seckill_voucher"

// 定义明确的错误类型
var (
	ErrStockNotEnough = errors.New("库存不足")
	ErrDuplicateOrder = errors.New("请勿重复购买")
)

type SecKillVoucher struct {
	VoucherId  int64     `gorm:"primary;column:voucher_id" json:"voucherId"`
	Stock      int       `gorm:"column:stock" json:"stock"`
	CreateTime time.Time `gorm:"column:create_time" json:"createTime"`
	BeginTime  time.Time `gorm:"column:begin_time" json:"beginTime"`
	EndTime    time.Time `gorm:"column:end_time" json:"endTime"`
	UpdateTime time.Time `gorm:"column:update_time" json:"updateTime"`
}

func (*SecKillVoucher) TableName() string {
	return SECKILL_VOUCHER_NAME
}
