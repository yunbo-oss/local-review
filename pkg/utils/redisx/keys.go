package redisx

import "strconv"

// Redis key 常量集中管理
const (
	LOGIN_CODE_KEY            = "login:code:"
	CACHE_SHOP_KEY            = "cache:shop:"
	CACHE_SHOP_LIST           = "shop:list"
	CACHE_LOCK_KEY            = "shop:lock:"
	SECKILL_STOCK_KEY         = "seckill:stock:"
	CACHE_SECKILL_VOUCHER_KEY = "cache:seckill:voucher:"
	BLOG_LIKE_KEY             = "blog:like:"
	FOLLOW_USER_KEY           = "follow:"
	FEED_KEY                  = "feed:"
	SHOP_GEO_KEY              = "shop:geo:"
	USER_SIGN_KEY             = "sign:"
	DISTRIBUTED_LOCK_KEY      = "lock:voucher:"
	LOCK_REBUILD_STOCK        = "lock:rebuild:stock:"
	UVKeyPrefix               = "uv:"

	// RAG 向量检索

	// Agent 记忆（002）
	AGENT_SESSION_PREFIX         = "agent:sess:" // List：agent:sess:{userId}:{sessionId}
	AGENT_SESSION_SUMMARY_PREFIX = "agent:sess:summary:"
	AGENT_PROFILE_PREFIX         = "agent:profile:" // Hash：agent:profile:{userId}（遗留事实源，迁移后改为缓存）

	// Agent 003：缓存与限流
	AGENT_PROFILE_CACHE_PREFIX = "agent:profile:cache:" // profile Cache Aside
	AGENT_RATE_LIMIT_PREFIX    = "agent:rl:"            // 用户级滑动窗口限流
	AGENT_CHECKPOINT_PREFIX    = "agent:checkpoint:v2:" // 短期 V2 AgentState，支持跨实例恢复
)

// AgentSessionKey 短期会话 List key
func AgentSessionKey(userID int64, sessionID string) string {
	return AGENT_SESSION_PREFIX + strconv.FormatInt(userID, 10) + ":" + sessionID
}

func AgentSessionSummaryKey(userID int64, sessionID string) string {
	return AGENT_SESSION_SUMMARY_PREFIX + strconv.FormatInt(userID, 10) + ":" + sessionID
}

// AgentProfileKey 长期偏好 Hash key（遗留 / 兼容）
func AgentProfileKey(userID int64) string {
	return AGENT_PROFILE_PREFIX + strconv.FormatInt(userID, 10)
}

// AgentProfileCacheKey PostgreSQL profile 的 Redis 缓存 key
func AgentProfileCacheKey(userID int64) string {
	return AGENT_PROFILE_CACHE_PREFIX + strconv.FormatInt(userID, 10)
}

// AgentRateLimitKey 用户推荐限流 key
func AgentRateLimitKey(userID int64) string {
	return AGENT_RATE_LIMIT_PREFIX + strconv.FormatInt(userID, 10)
}

func AgentCheckpointKey(runID string) string {
	return AGENT_CHECKPOINT_PREFIX + runID
}

const (
	LOGIN_VERIFY_CODE_TTL     = 2        // 分钟
	CACHE_SECKILL_VOUCHER_TTL = 5 * 60   // 秒，秒杀优惠券缓存 5 分钟
	HOT_KEY_EXISTS_TIME       = 10       // 秒
	REDIS_LOCK_VALUE          = "locked" // 默认锁值
	USER_NICK_NAME_PREFIX     = "user_"  // 随机昵称前缀
	MAXPAGESIZE               = 10       // 默认分页
	DEFAULTPAGESIZE           = 5
)
