package handler

import (
	"local-review-go/src/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ConfigRouter(r *gin.Engine) {
	// 全局中间件：处理所有请求的Token
	r.Use(middleware.GlobalTokenMiddleware())

	// 添加UV统计中间件（应用到所有路由）
	r.Use(middleware.UVStatisticsMiddleware())
	r.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, "pong")
	})

	// 需要认证的路由组
	authGroup := r.Group("/")
	authGroup.Use(middleware.AuthRequired())
	{
		userController := authGroup.Group("/user")

		{
			userController.POST("/logout", userHandler.Logout)
			userController.GET("/me", userHandler.Me)
			userController.GET("/info/:id", userHandler.Info)
			userController.GET("/sign", userHandler.sign)
			userController.GET("/sign/count", userHandler.SignCount)
		}

		shopController := authGroup.Group("/shop")

		{
			shopController.GET("/:id", shopHandler.QueryShopById)
			shopController.POST("", shopHandler.SaveShop)
			shopController.PUT("", shopHandler.UpdateShop)
			shopController.GET("/of/type", shopHandler.QueryShopByType)
			shopController.GET("/of/name", shopHandler.QueryShopByName)
		}

		voucherController := authGroup.Group("/voucher")

		{
			voucherController.POST("", voucherHandler.AddVoucher)
			voucherController.POST("/seckill", voucherHandler.AddSecKillVoucher)
			voucherController.GET("/list/:shopId", voucherHandler.QueryVoucherOfShop)
		}

		voucherOrderController := authGroup.Group("/voucher-order")

		{
			voucherOrderController.POST("/seckill/:id", voucherOrderHandler.SeckillVoucher)
		}

		blogController := authGroup.Group("/blog")

		{
			blogController.POST("", blogHandler.SaveBlog)
			blogController.PUT("/like/:id", blogHandler.LikeBlog)
			blogController.GET("/of/me", blogHandler.QueryMyBlog)
			blogController.GET("/:id", blogHandler.GetBlogById)
			blogController.GET("/likes/:id", blogHandler.QueryUserLiked)
			blogController.GET("/of/follow", blogHandler.QueryBlogOfFollow)
		}

		followContoller := authGroup.Group("/follow")

		{
			followContoller.PUT("/:id/:isFollow", followHanlder.Follow)
			followContoller.GET("/common/:id", followHanlder.FollowCommons)
			followContoller.GET("/or/not/:id", followHanlder.IsFollow)
		}

		uploadController := authGroup.Group("/upload")

		{
			uploadController.POST("/blog", uploadHandler.UploadImage)
			uploadController.GET("/blog/delete", uploadHandler.DeleteBlogImg)
		}
	}

	// 不需要认证的路由组
	publicGroup := r.Group("/")
	{
		userControllerWithOutMid := publicGroup.Group("/user")

		{
			userControllerWithOutMid.POST("/code", userHandler.SendCode)
			userControllerWithOutMid.POST("/login", userHandler.Login)
		}

		shopTypeController := publicGroup.Group("/shop-type")

		{
			shopTypeController.GET("/list", shopTypeHandler.QueryShopTypeList)
		}

		blogControllerWithOutMid := publicGroup.Group("/blog")
		{
			blogControllerWithOutMid.GET("/hot", blogHandler.QueryHotBlog)
		}
	}

	// 添加统计路由
	statisticsGroup := r.Group("/statistics")
	{
		statisticsGroup.GET("/uv", statisticsHandler.QueryUV)
		statisticsGroup.GET("/uv/current", statisticsHandler.QueryCurrentUV)
	}

}
