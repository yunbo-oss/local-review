package logic

import (
	"context"
	"fmt"
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils/redisx"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type VoucherLogic interface {
	AddVoucher(ctx context.Context, voucher *model.Voucher) error
	AddSeckillVoucher(ctx context.Context, voucher *model.Voucher) error
	QueryVoucherOfShop(ctx context.Context, shopID int64) ([]model.Voucher, error)
}

type voucherLogic struct{}

func NewVoucherLogic() VoucherLogic {
	return &voucherLogic{}
}

func (l *voucherLogic) AddVoucher(ctx context.Context, voucher *model.Voucher) error {
	if err := voucher.AddVoucher(mysql.GetMysqlDB().WithContext(ctx)); err != nil {
		return fmt.Errorf("db add voucher: %w", err)
	}
	return nil
}

func (l *voucherLogic) AddSeckillVoucher(ctx context.Context, voucher *model.Voucher) error {
	return mysql.GetMysqlDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := voucher.AddVoucher(tx); err != nil {
			return fmt.Errorf("写入主表失败: %w", err)
		}

		seckillVoucher := model.SecKillVoucher{
			VoucherId:  voucher.Id,
			Stock:      voucher.Stock,
			BeginTime:  voucher.BeginTime,
			EndTime:    voucher.EndTime,
			CreateTime: voucher.CreateTime,
			UpdateTime: voucher.UpdateTime,
		}
		if err := seckillVoucher.AddSeckillVoucher(tx); err != nil {
			return fmt.Errorf("写入秒杀表失败: %w", err)
		}
		return nil
	})

	// 事务成功后，异步更新Redis
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		redisKey := redisx.SECKILL_STOCK_KEY + strconv.FormatInt(voucher.Id, 10)
		if err := redis.GetRedisClient().Set(ctx, redisKey, voucher.Stock, 24*time.Hour).Err(); err != nil {
			logrus.Errorf("Redis缓存更新失败: key=%s, error=%v", redisKey, err)
			retryUpdateRedis(redisKey, voucher.Stock)
		}
	}()

	return nil
}

func (l *voucherLogic) QueryVoucherOfShop(ctx context.Context, shopID int64) ([]model.Voucher, error) {
	var vocherUtils model.Voucher
	vouchers, err := vocherUtils.QueryVoucherByShop(ctx, shopID)
	if err != nil {
		return nil, fmt.Errorf("db query vouchers by shop %d: %w", shopID, err)
	}
	return vouchers, nil
}

// 辅助函数：Redis更新重试
func retryUpdateRedis(key string, stock int) {
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i+1) * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := redis.GetRedisClient().Set(ctx, key, stock, 24*time.Hour).Err()
		cancel()
		if err == nil {
			return
		}
		logrus.Warnf("Redis重试%d次失败: %v", i+1, err)
	}
}
