package service

import (
	"context"
	"fmt"
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type VoucherService struct {
}

var VoucherManager *VoucherService

func (*VoucherService) AddVoucher(ctx context.Context, voucher *model.Voucher) error {
	err := voucher.AddVoucher(mysql.GetMysqlDB().WithContext(ctx))
	return err
}

// QueryVoucherOfShop 查询优惠卷
func (*VoucherService) QueryVoucherOfShop(ctx context.Context, shopId int64) ([]model.Voucher, error) {
	var vocherUtils model.Voucher
	return vocherUtils.QueryVoucherByShop(ctx, shopId)
}

func (vs *VoucherService) AddSeckillVoucher(ctx context.Context, voucher *model.Voucher) error {
	// 使用 GORM v2 的事务方式
	return mysql.GetMysqlDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 操作1：写入主表
		if err := voucher.AddVoucher(tx); err != nil {
			return fmt.Errorf("写入主表失败: %w", err)
		}

		// 2. 操作2：写入秒杀表
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

		// 事务成功会自动提交，失败会自动回滚
		return nil
	})

	// 5. 事务成功后，异步更新Redis
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		redisKey := utils.SECKILL_STOCK_KEY + strconv.FormatInt(voucher.Id, 10)
		if err := redis.GetRedisClient().Set(ctx, redisKey, voucher.Stock, 24*time.Hour).Err(); err != nil {
			// Redis更新失败时，可通过以下方式补偿：
			// a. 记录日志并报警
			logrus.Errorf("Redis缓存更新失败: key=%s, error=%v", redisKey, err)
			// b. 启动重试机制
			retryUpdateRedis(redisKey, voucher.Stock)
		}
	}()

	return nil
}

// 辅助函数：Redis更新重试
func retryUpdateRedis(key string, stock int) {
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i+1) * time.Second) // 退避策略
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := redis.GetRedisClient().Set(ctx, key, stock, 24*time.Hour).Err()
		cancel()
		if err == nil {
			return
		}
		logrus.Warnf("Redis重试%d次失败: %v", i+1, err)
	}
}
