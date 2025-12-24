package service

import (
	"context"
	"errors"
	"fmt"
	"local-review-go/src/config/redis"
	"local-review-go/src/dto"
	"local-review-go/src/httpx"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"strconv"
	"sync"
	"time"

	redisConfig "github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

type BlogService struct {
}

var BlogManager *BlogService

func (*BlogService) SaveBlog(ctx context.Context, userId int64, blog *model.Blog) (res int64, err error) {
	blog.CreateTime = time.Now()
	blog.UpdateTime = time.Now()

	id, err := blog.SaveBlog()
	if err != nil {
		logrus.Error("[Blog Service] failed to insert data!")
		return
	}
	var f model.Follow
	follows, err := f.GetFollowsByFollowId(userId)
	if err != nil {
		return
	}

	if len(follows) == 0 {
		return
	}

	for _, value := range follows {
		followUserId := value.UserId

		redisKey := utils.FEED_KEY + strconv.FormatInt(followUserId, 10)
		redis.GetRedisClient().ZAdd(ctx, redisKey, redisConfig.Z{
			Member: blog.Id,
			Score:  float64(time.Now().Unix()),
		})
	}

	res = id
	return
}

func (*BlogService) LikeBlog(ctx context.Context, id int64, userId int64) (err error) {
	// var blog model.Blog
	// blog.Id = id
	// err = blog.IncreseLike()
	// return
	userStr := strconv.FormatInt(userId, 10)
	redisKey := utils.BLOG_LIKE_KEY + strconv.FormatInt(id, 10)
	_, err = redis.GetRedisClient().ZScore(ctx, redisKey, userStr).Result()

	flag := false

	if err != nil {
		if err == redisConfig.Nil {
			flag = true
		} else {
			return err
		}
	}

	var blog model.Blog
	blog.Id = id

	if flag {
		// add like
		blog.IncrLike()
		// add the user
		err = redis.GetRedisClient().ZAdd(ctx, redisKey,
			redisConfig.Z{
				Score:  float64(time.Now().Unix()),
				Member: userStr,
			}).Err()
	} else {
		// have the data
		blog.DecrLike()
		err = redis.GetRedisClient().ZRem(ctx, redisKey, userStr).Err()
	}
	return err
}

func (*BlogService) QueryMyBlog(ctx context.Context, id int64, current int) ([]model.Blog, error) {
	var blog model.Blog
	blog.UserId = id
	blogs, err := blog.QueryBlogs(current)
	return blogs, err
}

func (*BlogService) QueryHotBlogs(ctx context.Context, current int) ([]model.Blog, error) {
	var blogUtils model.Blog
	blogs, err := blogUtils.QueryHots(current)
	if err != nil {
		return nil, err
	}
	for i := range blogs {
		id := blogs[i].UserId
		user, err := UserManager.GetUserById(id)
		if err != nil {
			logrus.Error(err.Error())
			continue
		}
		blogs[i].Icon = user.Icon
		blogs[i].Name = user.NickName
	}

	return blogs, nil
}

func (*BlogService) GetBlogById(ctx context.Context, id int64) (model.Blog, error) {
	var blog model.Blog
	err := blog.GetBlogById(id)
	if err != nil {
		return model.Blog{}, err
	}

	userId := blog.UserId
	user, err := UserManager.GetUserById(userId)

	if err != nil {
		return model.Blog{}, err
	}

	blog.Name = user.NickName
	blog.Icon = user.Icon

	return blog, err
}

// QueryUserLike 查询点赞该博客最早的5个用户
func (*BlogService) QueryUserLike(ctx context.Context, id int64) ([]dto.UserDTO, error) {
	// get the redis key
	redisKey := utils.BLOG_LIKE_KEY + strconv.FormatInt(id, 10)

	idStrs, err := redis.GetRedisClient().ZRange(ctx, redisKey, 0, 4).Result()
	if err != nil {
		return []dto.UserDTO{}, err
	}

	if len(idStrs) == 0 {
		return []dto.UserDTO{}, err
	}

	var ids []int64
	for _, value := range idStrs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return []dto.UserDTO{}, err
		}
		ids = append(ids, id)
	}

	var userUtils model.User
	users, err := userUtils.GetUsersByIds(ids)
	if err != nil {
		return []dto.UserDTO{}, err
	}

	userDTOS := make([]dto.UserDTO, len(users))
	for i := range users {
		userDTOS[i].Id = users[i].Id
		userDTOS[i].Icon = users[i].Icon
		userDTOS[i].NickName = users[i].NickName
	}
	return userDTOS, nil
}

func (*BlogService) QueryBlogOfFollow(ctx context.Context, maxTime int64, offset int, userId int64, pageSize int) (httpx.ScrollResult[model.Blog], error) {
	redisKey := utils.FEED_KEY + strconv.FormatInt(userId, 10)

	// 1. 从 Redis 获取博客 ID
	result, err := redis.GetRedisClient().ZRevRangeByScoreWithScores(ctx, redisKey,
		&redisConfig.ZRangeBy{
			Min:    "0",
			Max:    strconv.FormatInt(maxTime, 10),
			Offset: int64(offset),
			Count:  int64(pageSize),
		}).Result()
	if err != nil || len(result) == 0 {
		return httpx.ScrollResult[model.Blog]{}, err
	}

	// 2. 提取 ID 并计算 minTime/offset
	var (
		ids     []int64
		minTime = int64(0)
		os      = 0 // the number of equal number
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

	// 3. 批量查询博客详情
	var blogUtils model.Blog
	blogs, err := blogUtils.QueryBlogByIds(ids)
	if err != nil {
		return httpx.ScrollResult[model.Blog]{}, err
	}

	// 4. 并发填充用户信息和点赞状态
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
			isBlogLiked(ctx, userId, b)
		}(&blogs[i])
	}
	wg.Wait()

	// 5. 返回结果
	return httpx.ScrollResult[model.Blog]{
		Data:    blogs,
		MinTime: minTime,
		Offset:  os,
	}, nil
}

func createBlogUser(blog *model.Blog) error {
	userId := blog.UserId
	var userUtils model.User
	user, err := userUtils.GetUserById(userId)

	if err != nil {
		return fmt.Errorf("failed to get user %d: %v", blog.UserId, err)
	}
	blog.Name = user.NickName
	blog.Icon = user.Icon
	return nil
}

func isBlogLiked(ctx context.Context, userId int64, blog *model.Blog) {
	// Key 应基于博客ID，而非用户ID
	redisKey := utils.BLOG_LIKE_KEY + strconv.FormatInt(blog.Id, 10)

	// 查询用户ID是否在博客的点赞集合中
	err := redis.GetRedisClient().ZScore(ctx, redisKey, strconv.FormatInt(userId, 10)).Err()
	blog.IsLike = !errors.Is(err, redisConfig.Nil) // 存在即表示已点赞
}
