package utils

import (
	"github.com/sirupsen/logrus"
	"regexp"
)

type RegexUtils struct {
}

var RegexUtil *RegexUtils

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
