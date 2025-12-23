package service

import (
	"context"
	"errors"
	"fmt"
	redisClient "local-review-go/src/config/redis"
	"local-review-go/src/dto"
	"local-review-go/src/middleware"
	"local-review-go/src/model"
	"local-review-go/src/utils"
	"time"
)

type UserService struct {
}

var UserManager *UserService

func (*UserService) GetUserById(id int64) (model.User, error) {
	var userUtils model.User
	user, err := userUtils.GetUserById(id)
	return user, err
}

func (*UserService) SaveCode(ctx context.Context, phone string) error {
	if !utils.RegexUtil.IsPhoneValid(phone) {
		return errors.New("phone number is valid")
	}

	verifyCode := utils.RandomUtil.GenerateVerifyCode()
	err := redisClient.GetRedisClient().Set(ctx, utils.LOGIN_CODE_KEY+phone, verifyCode, time.Minute*utils.LOGIN_VERIFY_CODE_TTL).Err()
	return err
}

func (*UserService) Login(ctx context.Context, loginInfo *dto.LoginFormDto) (string, error) {
	if !utils.RegexUtil.IsPhoneValid(loginInfo.Phone) {
		return "", errors.New("not a valid phone")
	}

	// if !utils.RegexUtil.IsPassWordValid(loginInfo.Password) {
	// 	return "", errors.New("not a valid password")
	// }

	// if !utils.RegexUtil.IsVerifyCodeValid(loginInfo.Code) {
	// 	return "", errors.New("not a valid verify code")
	// }

	cacheCode, err := redisClient.GetRedisClient().Get(ctx, utils.LOGIN_CODE_KEY+loginInfo.Phone).Result()
	if err != nil {
		return "", err
	}

	if cacheCode != loginInfo.Code {
		return "", errors.New("a wrong verify code!")
	}

	var user model.User
	err = user.GetUserByPhone(loginInfo.Phone)
	if err != nil {
		user.Phone = loginInfo.Phone
		user.NickName = utils.USER_NICK_NAME_PREFIX + utils.RandomUtil.GenerateRandomStr(10)
		user.CreateTime = time.Now()
		user.UpdateTime = time.Now()
		err = user.SaveUser()
		if err != nil {
			return "", err
		}
	}

	var userDTO dto.UserDTO
	userDTO.Id = user.Id
	userDTO.Icon = user.Icon
	userDTO.NickName = user.NickName

	j := middleware.NewJWT()
	clamis := j.CreateClaims(userDTO)

	token, err := j.CreateToken(clamis)
	if err != nil {
		return "", errors.New("get token failed!")
	}

	return token, nil
}

// Sign 用户签到
func (s *UserService) Sign(ctx context.Context, userID int64) error {
	// 1. 获取当前日期
	now := time.Now()
	year, month := now.Year(), now.Month()
	day := now.Day()

	// 2. 构建Redis Key (sign:userID:yyyyMM)
	key := fmt.Sprintf("%s%d:%04d%02d", utils.USER_SIGN_KEY, userID, year, month)

	// 3. 设置对应bit位为1（偏移量从0开始）
	err := redisClient.GetRedisClient().SetBit(ctx, key, int64(day-1), 1).Err()
	if err != nil {
		return fmt.Errorf("签到失败: %v", err)
	}

	return nil
}

// GetSignCount 获取当月连续签到天数
func (s *UserService) GetSignCount(ctx context.Context, userID int64) (int, error) {
	// 1. 获取当前日期
	now := time.Now()
	year, month := now.Year(), now.Month()
	day := now.Day()

	// 2. 构建Redis Key
	key := fmt.Sprintf("%s%d:%04d%02d", utils.USER_SIGN_KEY, userID, year, month)

	// 3. 执行BITFIELD命令获取位图数据
	result, err := redisClient.GetRedisClient().Do(ctx,
		"BITFIELD", key,
		"GET", fmt.Sprintf("u%d", day), "0").Int64Slice()
	if err != nil {
		return 0, fmt.Errorf("获取签到数据失败: %v", err)
	}

	if len(result) == 0 || result[0] == 0 {
		return 0, nil // 无签到记录
	}

	// 4. 计算连续签到天数
	num := result[0]
	count := 0
	for {
		if (num & 1) == 0 {
			break
		}
		count++
		num >>= 1 // Go中无符号右移就是>>
	}

	return count, nil
}

// GetSignStatus 获取当月签到状态（返回每日签到情况）
func (s *UserService) GetSignStatus(userID int64, year int, month time.Month) (map[int]bool, error) {
	// 1. 构建Key
	key := fmt.Sprintf("%s%d:%04d%02d", utils.USER_SIGN_KEY, userID, year, month)

	// 2. 获取当月天数
	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()

	// 3. 获取所有bit位
	ctx := context.Background()
	status := make(map[int]bool, daysInMonth)
	for day := 1; day <= daysInMonth; day++ {
		bit, err := redisClient.GetRedisClient().GetBit(ctx, key, int64(day-1)).Result()
		if err != nil {
			return nil, fmt.Errorf("获取签到状态失败: %v", err)
		}
		status[day] = bit == 1
	}

	return status, nil
}
