package main

import (
	"context"
	"local-review-go/internal/config"
	"local-review-go/internal/config/mysql"
	"local-review-go/internal/config/redis"
	"local-review-go/internal/handler"
	"local-review-go/internal/logic"
	"local-review-go/internal/model"
	"local-review-go/internal/mq"
	"local-review-go/internal/repository"
	repoInterfaces "local-review-go/internal/repository/interface"
	"local-review-go/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	r := gin.Default()
	config.Init()

	shopRepo := repository.NewShopRepo(mysql.GetMysqlDB())
	shopLogic := logic.NewShopLogic(logic.ShopLogicDeps{ShopRepo: shopRepo})
	shopHandler := handler.NewShopHandler(shopLogic)

	userRepo := repository.NewUserRepo(mysql.GetMysqlDB())
	userInfoRepo := repository.NewUserInfoRepo(mysql.GetMysqlDB())
	shopTypeRepo := repository.NewShopTypeRepo(mysql.GetMysqlDB())
	blogRepo := repository.NewBlogRepo(mysql.GetMysqlDB())
	followRepo := repository.NewFollowRepo(mysql.GetMysqlDB())

	userLogic := logic.NewUserLogic(logic.UserLogicDeps{UserRepo: userRepo, UserInfoRepo: userInfoRepo})
	userHandler := handler.NewUserHandler(userLogic)
	shopTypeLogic := logic.NewShopTypeLogic(logic.ShopTypeLogicDeps{ShopTypeRepo: shopTypeRepo})
	shopTypeHandler := handler.NewShopTypeHandler(shopTypeLogic)

	voucherRepo := repository.NewVoucherRepo(mysql.GetMysqlDB())
	seckillVoucherRepo := repository.NewSeckillVoucherRepo(mysql.GetMysqlDB())
	voucherOrderRepo := repository.NewVoucherOrderRepo(mysql.GetMysqlDB())

	voucherLogic := logic.NewVoucherLogic(logic.VoucherLogicDeps{VoucherRepo: voucherRepo, SeckillVoucherRepo: seckillVoucherRepo})
	voucherHandler := handler.NewVoucherHandler(voucherLogic)

	seckillProducer, err := mq.NewSeckillProducer(redis.GetRedisClient(), "script/voucher_script.lua")
	if err != nil {
		logrus.Fatalf("RocketMQ 秒杀事务生产者初始化失败（请确保 RocketMQ 已启动）: %v", err)
	}
	orderTimeoutProducer, err := mq.NewOrderTimeoutProducer()
	if err != nil {
		logrus.Fatalf("RocketMQ 订单超时生产者初始化失败: %v", err)
	}
	voucherOrderLogic := logic.NewVoucherOrderLogic(logic.VoucherOrderLogicDeps{
		VoucherOrderRepo:     voucherOrderRepo,
		SeckillVoucherRepo:   seckillVoucherRepo,
		Producer:             seckillProducer,
		OrderTimeoutProducer: orderTimeoutProducer,
	})
	voucherOrderHandler := handler.NewVoucherOrderHandler(voucherOrderLogic)
	blogLogic := logic.NewBlogLogic(logic.BlogLogicDeps{BlogRepo: blogRepo, UserRepo: userRepo, FollowRepo: followRepo})
	blogHandler := handler.NewBlogHandler(blogLogic)
	followLogic := logic.NewFollowLogic(logic.FollowLogicDeps{UserRepo: userRepo, FollowRepo: followRepo})
	followHandler := handler.NewFollowHandler(followLogic)
	uploadLogic := logic.NewUploadLogic()
	uploadHandler := handler.NewUploadHandler(uploadLogic)
	statisticsLogic := logic.NewStatisticsLogic()
	statisticsHandler := handler.NewStatisticsHandler(statisticsLogic)

	// Auto Migrate
	mysql.GetMysqlDB().AutoMigrate(
		&model.User{},
		&model.UserInfo{},
		&model.Shop{},
		&model.ShopType{},
		&model.Blog{},
		&model.BlogComments{},
		&model.Voucher{},
		&model.SecKillVoucher{},
		&model.VoucherOrder{},
		&model.Follow{},
	)

	handler.ConfigRouter(r, handler.Handlers{
		Shop:         shopHandler,
		User:         userHandler,
		ShopType:     shopTypeHandler,
		Voucher:      voucherHandler,
		VoucherOrder: voucherOrderHandler,
		Blog:         blogHandler,
		Follow:       followHandler,
		Upload:       uploadHandler,
		Statistics:   statisticsHandler,
	})
	voucherOrderLogic.StartConsumers()

	// Init BloomFilter (异步预热)
	initBloomFilter(shopLogic, shopRepo)

	r.Run(":8088")

}

// initBloomFilter 异步预热布隆过滤器
func initBloomFilter(shopLogic logic.ShopLogic, shopRepo repoInterfaces.ShopRepo) {
	logrus.Info("Starting Bloom Filter pre-heating (async)...")

	// 先设置一个空的布隆过滤器实例，避免nil指针
	client := redis.GetRedisClient()
	bf := utils.NewBloomFilter(client, "bf:shop", 100000, 0.01)
	shopLogic.SetBloomFilter(bf)

	// 异步预热，不阻塞服务启动
	go func() {
		logrus.Info("Bloom Filter pre-heating started in background...")

		ids, err := shopRepo.ListAllIDs(context.Background())
		if err != nil {
			logrus.Errorf("Failed to query shop IDs for Bloom Filter pre-heating: %v", err)
			return
		}

		if len(ids) == 0 {
			logrus.Info("No shops found for Bloom Filter pre-heating")
			return
		}

		// 批量添加到布隆过滤器
		batchSize := 500
		totalCount := 0
		successCount := 0

		for i := 0; i < len(ids); i += batchSize {
			end := i + batchSize
			if end > len(ids) {
				end = len(ids)
			}
			batchIds := ids[i:end]

			err := bf.AddBatch(batchIds)
			if err != nil {
				logrus.Warnf("Failed to add batch [%d-%d] to Bloom Filter: %v", i, end-1, err)
				for _, id := range batchIds {
					if err := bf.Add(id); err != nil {
						logrus.Warnf("Failed to add shop %d to Bloom Filter: %v", id, err)
					} else {
						successCount++
					}
				}
			} else {
				successCount += len(batchIds)
			}

			totalCount = end

			if totalCount%1000 == 0 || end == len(ids) {
				logrus.Infof("Bloom Filter pre-heating progress: %d/%d shops (success: %d)", totalCount, len(ids), successCount)
			}
		}

		logrus.Infof("Bloom Filter pre-heating completed: %d/%d shops loaded successfully", successCount, len(ids))
	}()
}
