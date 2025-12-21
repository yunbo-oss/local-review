package main

import (
	"local-review-go/src/config"
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
	"local-review-go/src/handler"
	"local-review-go/src/model"
	"local-review-go/src/service"
	"local-review-go/src/utils"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func main() {
	r := gin.Default()
	config.Init()

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

	handler.ConfigRouter(r)
	service.InitOrderHandler()

	// Init BloomFilter
	initBloomFilter()

	r.Run(":8088")

}

func initBloomFilter() {
	client := redis.GetRedisClient()
	// Pre-heat Bloom Filter
	// Estimate 100,000 shops, 0.01 false positive rate
	bf := utils.NewBloomFilter(client, "bf:shop", 100000, 0.01)

	// Query all shops from MySQL
	var shops []model.Shop
	err := mysql.GetMysqlDB().Select("id").Find(&shops).Error
	if err != nil {
		logrus.Errorf("Failed to query shops for Bloom Filter pre-heating: %v", err)
		return
	}

	// Set global BloomFilter in Service
	service.SetShopBloomFilter(bf)

	if len(shops) == 0 {
		logrus.Info("No shops found for Bloom Filter pre-heating")
		return
	}

	count := 0
	for _, shop := range shops {
		err := bf.Add(shop.Id)
		if err != nil {
			logrus.Errorf("Failed to add shop %d to Bloom Filter: %v", shop.Id, err)
			continue
		}
		count++
	}
	logrus.Infof("Bloom Filter pre-heated with %d shops", count)

	// Set global BloomFilter in Service (Need to expose a Set/Get method or public variable in service)
	// For now, we assume we need to pass this to ShopService.
	// Since ShopService uses a singleton or similar, let's look at how to inject it.
	// We'll update ShopService to have a public SetBloomFilter method.
	service.SetShopBloomFilter(bf)
}
