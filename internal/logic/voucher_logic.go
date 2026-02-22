package logic

import (
	"context"
	"fmt"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/model"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils/redisx"
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

type voucherLogic struct {
	voucherRepo        repoInterfaces.VoucherRepo
	seckillVoucherRepo repoInterfaces.SeckillVoucherRepo
}

// VoucherLogicDeps 用于实例化 voucherLogic 的依赖
type VoucherLogicDeps struct {
	VoucherRepo        repoInterfaces.VoucherRepo
	SeckillVoucherRepo repoInterfaces.SeckillVoucherRepo
}

func NewVoucherLogic(deps VoucherLogicDeps) VoucherLogic {
	voucherRepo := deps.VoucherRepo
	if voucherRepo == nil {
		voucherRepo = repository.NewVoucherRepo(mysql.GetMysqlDB())
	}
	seckillVoucherRepo := deps.SeckillVoucherRepo
	if seckillVoucherRepo == nil {
		seckillVoucherRepo = repository.NewSeckillVoucherRepo(mysql.GetMysqlDB())
	}
	return &voucherLogic{
		voucherRepo:        voucherRepo,
		seckillVoucherRepo: seckillVoucherRepo,
	}
}

func (l *voucherLogic) AddVoucher(ctx context.Context, voucher *model.Voucher) error {
	if err := l.voucherRepo.Create(ctx, voucher, nil); err != nil {
		return fmt.Errorf("db add voucher: %w", err)
	}
	return nil
}

func (l *voucherLogic) AddSeckillVoucher(ctx context.Context, voucher *model.Voucher) error {
	err := mysql.GetMysqlDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := l.voucherRepo.Create(ctx, voucher, tx); err != nil {
			return fmt.Errorf("写入主表失败: %w", err)
		}

		seckillVoucher := &model.SecKillVoucher{
			VoucherId:  voucher.Id,
			Stock:      voucher.Stock,
			BeginTime:  voucher.BeginTime,
			EndTime:    voucher.EndTime,
			CreateTime: voucher.CreateTime,
			UpdateTime: voucher.UpdateTime,
		}
		if err := l.seckillVoucherRepo.Create(ctx, seckillVoucher, tx); err != nil {
			return fmt.Errorf("写入秒杀表失败: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

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
	vouchers, err := l.voucherRepo.ListByShopID(ctx, shopID)
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
