package service

import "local-review-go/src/model"

type UserInfoService struct {
}

var UserInfoManager *UserInfoService

func (*UserInfoService) GetUserInfoById(id int64) (model.UserInfo, error) {
	var userInfoUtils model.UserInfo
	return userInfoUtils.GetUserInfoById(id)
}
