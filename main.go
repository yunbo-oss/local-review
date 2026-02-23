package main

import (
	"local-review-go/src/config"
	"local-review-go/src/handler"
	"local-review-go/src/service"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	config.Init()
	handler.ConfigRouter(r)
	service.InitOrderHandler()

	r.Run(":8088")

}
