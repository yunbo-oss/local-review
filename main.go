package main

import (
	"local-review-go/src/config"
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
	"local-review-go/src/handler"
	"local-review-go/src/logic"
	"local-review-go/src/model"
	"local-review-go/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	r := gin.Default()
	config.Init()

	shopLogic := logic.NewShopLogic(logic.ShopLogicDeps{})
	shopHandler := handler.NewShopHandler(shopLogic)
	userLogic := logic.NewUserLogic()
	userHandler := handler.NewUserHandler(userLogic)
	shopTypeLogic := logic.NewShopTypeLogic()
	shopTypeHandler := handler.NewShopTypeHandler(shopTypeLogic)
	voucherLogic := logic.NewVoucherLogic()
	voucherHandler := handler.NewVoucherHandler(voucherLogic)
	voucherOrderLogic := logic.NewVoucherOrderLogic()
	voucherOrderHandler := handler.NewVoucherOrderHandler(voucherOrderLogic)
	blogLogic := logic.NewBlogLogic()
	blogHandler := handler.NewBlogHandler(blogLogic)
	followLogic := logic.NewFollowLogic()
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

	// Init BloomFilter (同步预热)
	initBloomFilter(shopLogic)

	r.Run(":8088")

}

// initBloomFilter 异步预热布隆过滤器
func initBloomFilter(shopLogic logic.ShopLogic) {
	logrus.Info("Starting Bloom Filter pre-heating (async)...")

	// 先设置一个空的布隆过滤器实例，避免nil指针
	client := redis.GetRedisClient()
	bf := utils.NewBloomFilter(client, "bf:shop", 100000, 0.01)
	shopLogic.SetBloomFilter(bf)

	// 异步预热，不阻塞服务启动
	go func() {
		logrus.Info("Bloom Filter pre-heating started in background...")

		// Query all shops from MySQL
		var shops []model.Shop
		err := mysql.GetMysqlDB().Select("id").Find(&shops).Error
		if err != nil {
			logrus.Errorf("Failed to query shops for Bloom Filter pre-heating: %v", err)
			return
		}

		if len(shops) == 0 {
			logrus.Info("No shops found for Bloom Filter pre-heating")
			return
		}

		// 真正的批量添加：收集ID，一次性执行Pipeline
		batchSize := 500 // 增大批次大小，减少Redis往返次数
		totalCount := 0
		successCount := 0

		for i := 0; i < len(shops); i += batchSize {
			end := i + batchSize
			if end > len(shops) {
				end = len(shops)
			}

			// 收集当前批次的ID
			batchIds := make([]int64, 0, end-i)
			for j := i; j < end; j++ {
				batchIds = append(batchIds, shops[j].Id)
			}

			// 批量添加到布隆过滤器（一次Redis往返）
			err := bf.AddBatch(batchIds)
			if err != nil {
				logrus.Warnf("Failed to add batch [%d-%d] to Bloom Filter: %v", i, end-1, err)
				// 批量失败时，回退到单个添加（容错处理）
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

			// 每处理一批后记录进度
			if totalCount%1000 == 0 || end == len(shops) {
				logrus.Infof("Bloom Filter pre-heating progress: %d/%d shops (success: %d)", totalCount, len(shops), successCount)
			}
		}

		logrus.Infof("Bloom Filter pre-heating completed: %d/%d shops loaded successfully", successCount, len(shops))
	}()
}
