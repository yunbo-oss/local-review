package logic

import (
	"context"
	"fmt"
	"local-review-go/src/config/mysql"
	"local-review-go/src/config/redis"
	"local-review-go/src/model"
	"local-review-go/src/repository"
	repoInterfaces "local-review-go/src/repository/interface"
	"local-review-go/src/utils/redisx"
	"strconv"

	"github.com/sirupsen/logrus"
)

type FollowLogic interface {
	Follow(ctx context.Context, id, userID int64, isFollow bool) error
	FollowCommons(ctx context.Context, id, userID int64) ([]UserBrief, error)
	IsFollow(ctx context.Context, id, userID int64) (bool, error)
}

type followLogic struct {
	userRepo   repoInterfaces.UserRepo
	followRepo repoInterfaces.FollowRepo
}

// FollowLogicDeps 用于实例化 followLogic 的依赖
type FollowLogicDeps struct {
	UserRepo   repoInterfaces.UserRepo
	FollowRepo repoInterfaces.FollowRepo
}

func NewFollowLogic(deps FollowLogicDeps) FollowLogic {
	userRepo := deps.UserRepo
	if userRepo == nil {
		userRepo = repository.NewUserRepo(mysql.GetMysqlDB())
	}
	followRepo := deps.FollowRepo
	if followRepo == nil {
		followRepo = repository.NewFollowRepo(mysql.GetMysqlDB())
	}
	return &followLogic{userRepo: userRepo, followRepo: followRepo}
}

func (l *followLogic) Follow(ctx context.Context, id, userID int64, isFollow bool) error {
	redisKey := redisx.FOLLOW_USER_KEY + strconv.FormatInt(userID, 10)

	if isFollow {
		if err := l.followRepo.Delete(ctx, userID, id); err != nil {
			return fmt.Errorf("remove follow user=%d target=%d: %w", userID, id, err)
		}
		if _, err := redis.GetRedisClient().SRem(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Redis SRem failed: %v", err)
		}
	} else {
		follow := &model.Follow{UserId: userID, FollowUserId: id}
		if err := l.followRepo.Create(ctx, follow); err != nil {
			return fmt.Errorf("save follow user=%d target=%d: %w", userID, id, err)
		}
		if _, err := redis.GetRedisClient().SAdd(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Redis SAdd failed: %v", err)
		}
	}
	return nil
}

func (l *followLogic) FollowCommons(ctx context.Context, id, userID int64) ([]UserBrief, error) {
	redisKeySelf := redisx.FOLLOW_USER_KEY + strconv.FormatInt(userID, 10)
	redisKeyTarget := redisx.FOLLOW_USER_KEY + strconv.FormatInt(id, 10)

	idStrs, err := redis.GetRedisClient().SInter(ctx, redisKeySelf, redisKeyTarget).Result()
	if err != nil {
		return []UserBrief{}, fmt.Errorf("sinter follow sets: %w", err)
	}

	if idStrs == nil || len(idStrs) == 0 {
		return []UserBrief{}, nil
	}

	var ids []int64
	for _, value := range idStrs {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return []UserBrief{}, fmt.Errorf("parse follow id %s: %w", value, err)
		}
		ids = append(ids, id)
	}

	users, err := l.userRepo.GetByIDs(ctx, ids)
	if err != nil {
		return []UserBrief{}, fmt.Errorf("query users by ids: %w", err)
	}

	userDTOs := make([]UserBrief, len(users))
	for i := range users {
		userDTOs[i].Id = users[i].Id
		userDTOs[i].Icon = users[i].Icon
		userDTOs[i].NickName = users[i].NickName
	}
	return userDTOs, nil
}

func (l *followLogic) IsFollow(ctx context.Context, id, userID int64) (bool, error) {
	redisKey := redisx.FOLLOW_USER_KEY + strconv.FormatInt(userID, 10)

	exists, err := redis.GetRedisClient().SIsMember(ctx, redisKey, id).Result()
	if err == nil {
		return exists, nil
	}

	dbExists, err := l.followRepo.Exists(ctx, userID, id)
	if err != nil {
		return false, fmt.Errorf("db check follow user=%d target=%d: %w", userID, id, err)
	}

	if dbExists {
		if _, err := redis.GetRedisClient().SAdd(ctx, redisKey, id).Result(); err != nil {
			logrus.Errorf("Failed to update Redis cache: %v", err)
		}
	}

	return dbExists, nil
}
