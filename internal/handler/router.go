package handler

import (
	"local-review-go/internal/middleware"
	"net/http"
	"path/filepath"

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

	// 静态文件：前端页面和资源（适配黑马点评前端）
	r.Static("/imgs", "./front-end/imgs")
	r.StaticFile("/", "./front-end/index.html")
	r.StaticFile("/index.html", "./front-end/index.html")
	r.StaticFile("/login.html", "./front-end/login.html")
	r.StaticFile("/login2.html", "./front-end/login2.html")
	r.StaticFile("/shop-list.html", "./front-end/shop-list.html")
	r.StaticFile("/shop-detail.html", "./front-end/shop-detail.html")
	r.StaticFile("/blog-detail.html", "./front-end/blog-detail.html")
	r.StaticFile("/blog-edit.html", "./front-end/blog-edit.html")
	r.StaticFile("/info.html", "./front-end/info.html")
	r.StaticFile("/info-edit.html", "./front-end/info-edit.html")
	r.StaticFile("/other-info.html", "./front-end/other-info.html")
	// 静态资源：css、js 等
	r.Static("/css", filepath.Join("front-end", "css"))
	r.Static("/js", filepath.Join("front-end", "js"))

	// API 路由组：前端 baseURL 为 /api
	apiGroup := r.Group("/api")
	{
		configAuthRoutes(apiGroup, handlers)
		configPublicRoutes(apiGroup, handlers)
		configStatisticsRoutes(apiGroup, handlers)
	}
}

func configAuthRoutes(apiGroup *gin.RouterGroup, handlers Handlers) {
	authGroup := apiGroup.Group("/")
	authGroup.Use(middleware.AuthRequired())
	{
		userController := authGroup.Group("/user")
		{
			userController.POST("/logout", handlers.User.Logout)
			userController.GET("/me", handlers.User.Me)
			userController.GET("/info/:id", handlers.User.Info)
			userController.GET("/:id", handlers.User.UserById) // other-info 需要：GET /user/:id（返回 id/nickName/icon）
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
		voucherOrderController.Use(middleware.SeckillRateLimit())
		{
			voucherOrderController.POST("/seckill/:id", handlers.VoucherOrder.SeckillVoucher)
		}

		blogController := authGroup.Group("/blog")
		{
			blogController.POST("", handlers.Blog.SaveBlog)
			blogController.PUT("/like/:id", handlers.Blog.LikeBlog)
			blogController.GET("/of/me", handlers.Blog.QueryMyBlog)
			blogController.GET("/of/user", handlers.Blog.QueryBlogByUserId) // other-info 需要：GET /blog/of/user?id=&current=
			blogController.GET("/of/follow", handlers.Blog.QueryBlogOfFollow)
			blogController.GET("/:id", handlers.Blog.GetBlogById)
			blogController.GET("/likes/:id", handlers.Blog.QueryUserLiked)
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
}

func configPublicRoutes(apiGroup *gin.RouterGroup, handlers Handlers) {
	publicGroup := apiGroup.Group("/")
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
}

func configStatisticsRoutes(apiGroup *gin.RouterGroup, handlers Handlers) {
	statisticsGroup := apiGroup.Group("/statistics")
	{
		statisticsGroup.GET("/uv", handlers.Statistics.QueryUV)
		statisticsGroup.GET("/uv/current", handlers.Statistics.QueryCurrentUV)
	}
}
