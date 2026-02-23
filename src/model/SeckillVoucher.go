package model

import (
	"errors"
	"github.com/jinzhu/gorm"
	"local-review-go/src/config/mysql"
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

func (sec *SecKillVoucher) AddSeckillVoucher(tx *gorm.DB) error {
	return tx.Table(sec.TableName()).Create(sec).Error
}

func (sec *SecKillVoucher) QuerySeckillVoucherById(id int64) error {
	return mysql.GetMysqlDB().Table(sec.TableName()).Where("voucher_id = ?", id).First(sec).Error
}

// 扣减库存
func (sv *SecKillVoucher) DecrVoucherStock(voucherId int64, tx *gorm.DB) error {
	// 判断秒杀库存是否足够
	result := tx.Exec(`
		UPDATE tb_seckill_voucher 
		SET stock = stock - 1 
		WHERE voucher_id = ? AND stock > 0
	`, voucherId)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return ErrStockNotEnough
	}
	return nil
}
