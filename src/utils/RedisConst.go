package utils

const (
	LOGIN_CODE_KEY       = "login:code:"
	CACHE_SHOP_KEY       = "cache:shop:"
	CACHE_SHOP_LIST      = "shop:list"
	CACHE_LOCK_KEY       = "shop:lock:"
	SECKILL_STOCK_KEY    = "seckill:stock:"
	BLOG_LIKE_KEY        = "blog:like:"
	FOLLOW_USER_KEY      = "follow:"
	FEED_KEY             = "feed:"
	SHOP_GEO_KEY         = "shop:geo:"
	USER_SIGN_KEY        = "sign:"
	DISTRIBUTED_LOCK_KEY = "lock:voucher:"
	UVKeyPrefix          = "uv:"
)

const (
	REDIS_LOCK_VALUE = "locked"
)

const (
	LOGIN_VERIFY_CODE_TTL = 2
	HOT_KEY_EXISTS_TIME   = 10
)
