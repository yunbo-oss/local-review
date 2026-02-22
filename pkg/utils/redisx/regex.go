package redisx

import (
	"github.com/sirupsen/logrus"
	"regexp"
)

type RegexUtils struct{}

// RegexUtil 是全局正则工具实例
var RegexUtil = &RegexUtils{}

const (
	PHONE_REGEX       = `^(13[0-9]|14[01456879]|15[0-35-9]|16[2567]|17[0-8]|18[0-9]|19[0-35-9])\d{8}$`
	EMAIL_REGEX       = `^[a-zA-Z0-9_-]+@[a-zA-Z0-9_-]+(\\.[a-zA-Z0-9_-]+)+$`
	PASSWORD_REGEX    = `^\\w{4,32}$`
	VERITY_CODE_REGEX = `^[a-zA-Z\\d]{6}$`
)

func (*RegexUtils) IsPhoneValid(phone string) bool {
	re, err := regexp.Compile(PHONE_REGEX)
	if err != nil {
		logrus.Error("complie phone regex failed!")
		return false
	}
	return re.MatchString(phone)
}

func (*RegexUtils) IsEmailValid(email string) bool {
	re, err := regexp.Compile(EMAIL_REGEX)
	if err != nil {
		logrus.Error("compile email regex failed!")
		return false
	}
	return re.MatchString(email)
}

func (*RegexUtils) IsPassWordValid(password string) bool {
	re, err := regexp.Compile(PASSWORD_REGEX)
	if err != nil {
		logrus.Error("compile password failed!")
		return false
	}
	return re.MatchString(password)
}

func (*RegexUtils) IsVerifyCodeValid(verifyCode string) bool {
	re, err := regexp.Compile(VERITY_CODE_REGEX)
	if err != nil {
		logrus.Error("complie verify code regex failed!")
		return false
	}
	return re.MatchString(verifyCode)
}
