package service

import (
	"context"
	"github.com/sirupsen/logrus"
	"local-review-go/src/config/redis"
	"local-review-go/src/dto"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"strconv"
	"time"
)

type FollowService struct {
}

var FollowManager *FollowService

func (*FollowService) Follow(id int64, userId int64, isFollow bool) error {
	redisKey := utils.FOLLOW_USER_KEY + strconv.FormatInt(userId, 10)
	ctx := context.Background()

	if isFollow {
		// 删除数据库记录
		var f model.Follow
		if err := f.RemoveUserFollow(id, userId); err != nil {
			return err
		}
		// 从Redis集合中移除
		if _, err := redis.GetRedisClient().SRem(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Redis SRem failed: %v", err)
		}
	} else {
		// 添加数据库记录
		var f model.Follow
		f.UserId = userId
		f.FollowUserId = id
		f.CreateTime = time.Now()
		if err := f.SaveUserFollow(); err != nil {
			return err
		}
		// 向Redis集合添加
		if _, err := redis.GetRedisClient().SAdd(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Redis SAdd failed: %v", err)
		}
	}
	return nil
}

func (*FollowService) FollowCommons(id int64, userId int64) ([]dto.UserDTO, error) {
	redisKeySelf := utils.FOLLOW_USER_KEY + strconv.FormatInt(userId, 10)
	redisKeyTarget := utils.FOLLOW_USER_KEY + strconv.FormatInt(id, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	idStrs, err := redis.GetRedisClient().SInter(ctx, redisKeySelf, redisKeyTarget).Result()
	if err != nil {
		return []dto.UserDTO{}, err
	}

	if idStrs == nil || len(idStrs) == 0 {
		return []dto.UserDTO{}, nil
	}

	var ids []int64
	for _, value := range idStrs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return []dto.UserDTO{}, nil
		}
		ids = append(ids, id)
	}

	var userUtils model.User
	users, err := userUtils.GetUsersByIds(ids)
	if err != nil {
		return []dto.UserDTO{}, nil
	}

	userDTOs := make([]dto.UserDTO, len(users))
	for i := range users {
		userDTOs[i].Id = users[i].Id
		userDTOs[i].Icon = users[i].Icon
		userDTOs[i].NickName = users[i].NickName
	}
	return userDTOs, nil
}

func (*FollowService) IsFollow(id int64, userId int64) (bool, error) {
	redisKey := utils.FOLLOW_USER_KEY + strconv.FormatInt(userId, 10)
	ctx := context.Background()

	// 先尝试从 Redis 缓存中查询
	exists, err := redis.GetRedisClient().SIsMember(ctx, redisKey, id).Result()
	if err == nil {
		// 缓存命中，直接返回结果
		return exists, nil
	}

	// 缓存未命中或出错，回退到数据库查询
	var f model.Follow
	f.UserId = userId
	f.FollowUserId = id
	count, err := f.IsFollowing()
	if err != nil {
		return false, err
	}

	// 将结果回填到 Redis 缓存
	if count > 0 {
		if _, err := redis.GetRedisClient().SAdd(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Failed to update Redis cache: %v", err)
		}
	}

	return count > 0, nil
}
