package logic

import (
	"context"
	"errors"
	"fmt"
	"local-review-go/src/config/redis"
	"local-review-go/src/middleware"
	"local-review-go/src/model"
	"local-review-go/src/utils/redisx"
	"time"
)

type UserLogic interface {
	SendCode(ctx context.Context, phone string) error
	Login(ctx context.Context, phone, code string) (string, error)
	Sign(ctx context.Context, userID int64) error
	GetSignCount(ctx context.Context, userID int64) (int, error)
	GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error)
}

// UserBrief 用于对外返回/内部传递的用户简要信息
type UserBrief struct {
	Id       int64  `json:"id"`
	NickName string `json:"nickName"`
	Icon     string `json:"icon"`
}

type userLogic struct{}

func NewUserLogic() UserLogic {
	return &userLogic{}
}

func (l *userLogic) SendCode(ctx context.Context, phone string) error {
	if !redisx.RegexUtil.IsPhoneValid(phone) {
		return errors.New("phone number is valid")
	}

	verifyCode := redisx.RandomUtil.GenerateVerifyCode()
	if err := redis.GetRedisClient().Set(ctx, redisx.LOGIN_CODE_KEY+phone, verifyCode, time.Minute*redisx.LOGIN_VERIFY_CODE_TTL).Err(); err != nil {
		return fmt.Errorf("set login code for %s: %w", phone, err)
	}
	return nil
}

func (l *userLogic) Login(ctx context.Context, phone, code string) (string, error) {
	if !redisx.RegexUtil.IsPhoneValid(phone) {
		return "", errors.New("not a valid phone")
	}

	cacheCode, err := redis.GetRedisClient().Get(ctx, redisx.LOGIN_CODE_KEY+phone).Result()
	if err != nil {
		return "", fmt.Errorf("get login code for %s: %w", phone, err)
	}

	if cacheCode != code {
		return "", errors.New("a wrong verify code!")
	}

	var user model.User
	err = user.GetUserByPhone(phone)
	if err != nil {
		user.Phone = phone
		user.NickName = redisx.USER_NICK_NAME_PREFIX + redisx.RandomUtil.GenerateRandomStr(10)
		user.CreateTime = time.Now()
		user.UpdateTime = time.Now()
		if err = user.SaveUser(); err != nil {
			return "", fmt.Errorf("create user %s: %w", phone, err)
		}
	}

	var authUser middleware.AuthUser
	authUser.Id = user.Id
	authUser.Icon = user.Icon
	authUser.NickName = user.NickName

	j := middleware.NewJWT()
	claims := j.CreateClaims(authUser)

	token, err := j.CreateToken(claims)
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}

	return token, nil
}

// Sign 用户签到
func (l *userLogic) Sign(ctx context.Context, userID int64) error {
	now := time.Now()
	year, month := now.Year(), now.Month()
	day := now.Day()

	key := fmt.Sprintf("%s%d:%04d%02d", redisx.USER_SIGN_KEY, userID, year, month)
	if err := redis.GetRedisClient().SetBit(ctx, key, int64(day-1), 1).Err(); err != nil {
		return fmt.Errorf("set sign bit for user %d: %w", userID, err)
	}
	return nil
}

// GetSignCount 获取当月连续签到天数
func (l *userLogic) GetSignCount(ctx context.Context, userID int64) (int, error) {
	now := time.Now()
	year, month := now.Year(), now.Month()
	day := now.Day()

	key := fmt.Sprintf("%s%d:%04d%02d", redisx.USER_SIGN_KEY, userID, year, month)

	result, err := redis.GetRedisClient().Do(ctx,
		"BITFIELD", key,
		"GET", fmt.Sprintf("u%d", day), "0").Int64Slice()
	if err != nil {
		return 0, fmt.Errorf("get sign bitfield user=%d: %w", userID, err)
	}

	if len(result) == 0 || result[0] == 0 {
		return 0, nil // 无签到记录
	}

	num := result[0]
	count := 0
	for {
		if (num & 1) == 0 {
			break
		}
		count++
		num >>= 1
	}

	return count, nil
}

func (l *userLogic) GetUserInfo(ctx context.Context, id int64) (model.UserInfo, error) {
	var userInfoUtils model.UserInfo
	info, err := userInfoUtils.GetUserInfoById(id)
	if err != nil {
		return model.UserInfo{}, fmt.Errorf("db get user info %d: %w", id, err)
	}
	return info, nil
}
