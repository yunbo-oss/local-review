package handler

import (
	"local-review-go/src/middleware"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handlers struct {
	Shop         *ShopHandler
	User         *UserHandler
	ShopType     *ShopTypeHandler
	Voucher      *VoucherHandler
	VoucherOrder *VoucherOrderHandler
	Blog         *BlogHandler
	Follow       *FollowHandler
	Upload       *UploadHandler
	Statistics   *StatisticsHandler
}

func ConfigRouter(r *gin.Engine, handlers Handlers) {
	if handlers.Shop == nil || handlers.User == nil || handlers.ShopType == nil || handlers.Voucher == nil || handlers.VoucherOrder == nil || handlers.Blog == nil || handlers.Follow == nil || handlers.Upload == nil || handlers.Statistics == nil {
		panic("handlers not fully wired: please initialize all handlers before configuring routes")
	}

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
			userController.POST("/logout", handlers.User.Logout)
			userController.GET("/me", handlers.User.Me)
			userController.GET("/info/:id", handlers.User.Info)
			userController.GET("/sign", handlers.User.sign)
			userController.GET("/sign/count", handlers.User.SignCount)
		}

		shopController := authGroup.Group("/shop")

		{
			shopController.GET("/:id", handlers.Shop.QueryShopById)
			shopController.POST("", handlers.Shop.SaveShop)
			shopController.PUT("", handlers.Shop.UpdateShop)
			shopController.GET("/of/type", handlers.Shop.QueryShopByType)
			shopController.GET("/of/name", handlers.Shop.QueryShopByName)
		}

		voucherController := authGroup.Group("/voucher")

		{
			voucherController.POST("", handlers.Voucher.AddVoucher)
			voucherController.POST("/seckill", handlers.Voucher.AddSecKillVoucher)
			voucherController.GET("/list/:shopId", handlers.Voucher.QueryVoucherOfShop)
		}

		voucherOrderController := authGroup.Group("/voucher-order")

		{
			voucherOrderController.POST("/seckill/:id", handlers.VoucherOrder.SeckillVoucher)
		}

		blogController := authGroup.Group("/blog")

		{
			blogController.POST("", handlers.Blog.SaveBlog)
			blogController.PUT("/like/:id", handlers.Blog.LikeBlog)
			blogController.GET("/of/me", handlers.Blog.QueryMyBlog)
			blogController.GET("/:id", handlers.Blog.GetBlogById)
			blogController.GET("/likes/:id", handlers.Blog.QueryUserLiked)
			blogController.GET("/of/follow", handlers.Blog.QueryBlogOfFollow)
		}

		followContoller := authGroup.Group("/follow")

		{
			followContoller.PUT("/:id/:isFollow", handlers.Follow.Follow)
			followContoller.GET("/common/:id", handlers.Follow.FollowCommons)
			followContoller.GET("/or/not/:id", handlers.Follow.IsFollow)
		}

		uploadController := authGroup.Group("/upload")

		{
			uploadController.POST("/blog", handlers.Upload.UploadImage)
			uploadController.GET("/blog/delete", handlers.Upload.DeleteBlogImg)
		}
	}

	// 不需要认证的路由组
	publicGroup := r.Group("/")
	{
		userControllerWithOutMid := publicGroup.Group("/user")

		{
			userControllerWithOutMid.POST("/code", handlers.User.SendCode)
			userControllerWithOutMid.POST("/login", handlers.User.Login)
		}

		shopTypeController := publicGroup.Group("/shop-type")

		{
			shopTypeController.GET("/list", handlers.ShopType.QueryShopTypeList)
		}

		blogControllerWithOutMid := publicGroup.Group("/blog")
		{
			blogControllerWithOutMid.GET("/hot", handlers.Blog.QueryHotBlog)
		}
	}

	// 添加统计路由
	statisticsGroup := r.Group("/statistics")
	{
		statisticsGroup.GET("/uv", handlers.Statistics.QueryUV)
		statisticsGroup.GET("/uv/current", handlers.Statistics.QueryCurrentUV)
	}

}
