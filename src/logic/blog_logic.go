package logic

import (
	"context"
	"errors"
	"fmt"
	"local-review-go/src/config/redis"
	"local-review-go/src/httpx"
	"local-review-go/src/model"
	"local-review-go/src/utils/redisx"
	"strconv"
	"sync"
	"time"

	redisConfig "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

type BlogLogic interface {
	SaveBlog(ctx context.Context, userID int64, blog *model.Blog) (int64, error)
	LikeBlog(ctx context.Context, id, userID int64) error
	QueryUserLike(ctx context.Context, id int64) ([]UserBrief, error)
	QueryMyBlog(ctx context.Context, userID int64, current int) ([]model.Blog, error)
	QueryHotBlogs(ctx context.Context, current int) ([]model.Blog, error)
	GetBlogById(ctx context.Context, id int64) (model.Blog, error)
	QueryBlogOfFollow(ctx context.Context, maxTime int64, offset int, userID int64, pageSize int) (httpx.ScrollResult[model.Blog], error)
}

type blogLogic struct{}

func NewBlogLogic() BlogLogic {
	return &blogLogic{}
}

func (l *blogLogic) SaveBlog(ctx context.Context, userID int64, blog *model.Blog) (res int64, err error) {
	blog.CreateTime = time.Now()
	blog.UpdateTime = time.Now()

	id, err := blog.SaveBlog()
	if err != nil {
		logrus.Error("[Blog Service] failed to insert data!")
		return 0, fmt.Errorf("db save blog user=%d: %w", userID, err)
	}
	var f model.Follow
	follows, err := f.GetFollowsByFollowId(userID)
	if err != nil {
		return 0, fmt.Errorf("query followers of user %d: %w", userID, err)
	}

	if len(follows) == 0 {
		return
	}

	for _, value := range follows {
		followUserId := value.UserId

		redisKey := redisx.FEED_KEY + strconv.FormatInt(followUserId, 10)
		if err := redis.GetRedisClient().ZAdd(ctx, redisKey, redisConfig.Z{
			Member: blog.Id,
			Score:  float64(time.Now().Unix()),
		}).Err(); err != nil {
			logrus.Warnf("push blog %d to feed %d failed: %v", blog.Id, followUserId, err)
		}
	}

	res = id
	return
}

func (l *blogLogic) LikeBlog(ctx context.Context, id, userID int64) (err error) {
	userStr := strconv.FormatInt(userID, 10)
	redisKey := redisx.BLOG_LIKE_KEY + strconv.FormatInt(id, 10)
	_, err = redis.GetRedisClient().ZScore(ctx, redisKey, userStr).Result()

	flag := false

	if err != nil {
		if err == redisConfig.Nil {
			flag = true
		} else {
			return fmt.Errorf("zscore blog like cache blog=%d user=%d: %w", id, userID, err)
		}
	}

	var blog model.Blog
	blog.Id = id

	if flag {
		blog.IncrLike()
		err = redis.GetRedisClient().ZAdd(ctx, redisKey,
			redisConfig.Z{
				Score:  float64(time.Now().Unix()),
				Member: userStr,
			}).Err()
	} else {
		blog.DecrLike()
		err = redis.GetRedisClient().ZRem(ctx, redisKey, userStr).Err()
	}
	if err != nil {
		return fmt.Errorf("update blog like cache blog=%d user=%d: %w", id, userID, err)
	}
	return nil
}

func (l *blogLogic) QueryMyBlog(ctx context.Context, userID int64, current int) ([]model.Blog, error) {
	var blog model.Blog
	blog.UserId = userID
	blogs, err := blog.QueryBlogs(current)
	if err != nil {
		return nil, fmt.Errorf("db query my blogs user=%d page=%d: %w", userID, current, err)
	}
	return blogs, nil
}

func (l *blogLogic) QueryHotBlogs(ctx context.Context, current int) ([]model.Blog, error) {
	var blogUtils model.Blog
	blogs, err := blogUtils.QueryHots(current)
	if err != nil {
		return nil, fmt.Errorf("db query hot blogs page=%d: %w", current, err)
	}
	for i := range blogs {
		id := blogs[i].UserId
		user, err := new(model.User).GetUserById(id)
		if err != nil {
			logrus.Errorf("get user %d for blog %d failed: %v", id, blogs[i].Id, err)
			continue
		}
		blogs[i].Icon = user.Icon
		blogs[i].Name = user.NickName
	}

	return blogs, nil
}

func (l *blogLogic) GetBlogById(ctx context.Context, id int64) (model.Blog, error) {
	var blog model.Blog
	if err := blog.GetBlogById(id); err != nil {
		return model.Blog{}, fmt.Errorf("db get blog %d: %w", id, err)
	}

	userId := blog.UserId
	user, err := new(model.User).GetUserById(userId)
	if err != nil {
		return model.Blog{}, fmt.Errorf("get user %d for blog %d: %w", userId, id, err)
	}

	blog.Name = user.NickName
	blog.Icon = user.Icon

	return blog, nil
}

// QueryUserLike 查询点赞该博客最早的5个用户
func (l *blogLogic) QueryUserLike(ctx context.Context, id int64) ([]UserBrief, error) {
	redisKey := redisx.BLOG_LIKE_KEY + strconv.FormatInt(id, 10)

	idStrs, err := redis.GetRedisClient().ZRange(ctx, redisKey, 0, 4).Result()
	if err != nil {
		return []UserBrief{}, fmt.Errorf("zrange blog like %d: %w", id, err)
	}

	if len(idStrs) == 0 {
		return []UserBrief{}, nil
	}

	var ids []int64
	for _, value := range idStrs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return []UserBrief{}, fmt.Errorf("parse like uid %s: %w", value, err)
		}
		ids = append(ids, id)
	}

	users, err := new(model.User).GetUsersByIds(ids)
	if err != nil {
		return []UserBrief{}, fmt.Errorf("db get users by ids %v: %w", ids, err)
	}

	userDTOS := make([]UserBrief, len(users))
	for i := range users {
		userDTOS[i].Id = users[i].Id
		userDTOS[i].Icon = users[i].Icon
		userDTOS[i].NickName = users[i].NickName
	}
	return userDTOS, nil
}

func (l *blogLogic) QueryBlogOfFollow(ctx context.Context, maxTime int64, offset int, userID int64, pageSize int) (httpx.ScrollResult[model.Blog], error) {
	redisKey := redisx.FEED_KEY + strconv.FormatInt(userID, 10)

	result, err := redis.GetRedisClient().ZRevRangeByScoreWithScores(ctx, redisKey,
		&redisConfig.ZRangeBy{
			Min:    "0",
			Max:    strconv.FormatInt(maxTime, 10),
			Offset: int64(offset),
			Count:  int64(pageSize),
		}).Result()
	if err != nil || len(result) == 0 {
		if err != nil {
			return httpx.ScrollResult[model.Blog]{}, fmt.Errorf("zrevrangebyscore feed %d: %w", userID, err)
		}
		return httpx.ScrollResult[model.Blog]{}, nil
	}

	var (
		ids     []int64
		minTime = int64(0)
		os      = 0
	)
	for _, value := range result {
		id := value.Member.(int64)
		ids = append(ids, id)

		score := int64(value.Score)
		if score == minTime {
			os++
		} else {
			minTime = score
			os = 1
		}
	}

	blogs, err := new(model.Blog).QueryBlogByIds(ids)
	if err != nil {
		return httpx.ScrollResult[model.Blog]{}, fmt.Errorf("db get blogs by ids %v: %w", ids, err)
	}

	var wg sync.WaitGroup
	for i := range blogs {
		wg.Add(2)
		go func(b *model.Blog) {
			defer wg.Done()
			if err := createBlogUser(b); err != nil {
				logrus.Warnf("Fill user failed for blog %d: %v", b.Id, err)
			}
		}(&blogs[i])

		go func(b *model.Blog) {
			defer wg.Done()
			isBlogLiked(ctx, userID, b)
		}(&blogs[i])
	}
	wg.Wait()

	return httpx.ScrollResult[model.Blog]{
		Data:    blogs,
		MinTime: minTime,
		Offset:  os,
	}, nil
}

func createBlogUser(blog *model.Blog) error {
	userId := blog.UserId
	user, err := new(model.User).GetUserById(userId)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %w", blog.UserId, err)
	}
	blog.Name = user.NickName
	blog.Icon = user.Icon
	return nil
}

func isBlogLiked(ctx context.Context, userID int64, blog *model.Blog) {
	redisKey := redisx.BLOG_LIKE_KEY + strconv.FormatInt(blog.Id, 10)
	err := redis.GetRedisClient().ZScore(ctx, redisKey, strconv.FormatInt(userID, 10)).Err()
	blog.IsLike = !errors.Is(err, redisConfig.Nil)
}
