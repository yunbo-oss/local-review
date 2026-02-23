package utils

// 各种正则表达式
const (
	PHONE_REGEX       = `^(13[0-9]|14[01456879]|15[0-35-9]|16[2567]|17[0-8]|18[0-9]|19[0-35-9])\d{8}$`
	EMAIL_REGEX       = `^[a-zA-Z0-9_-]+@[a-zA-Z0-9_-]+(\\.[a-zA-Z0-9_-]+)+$`
	PASSWORD_REGEX    = `^\\w{4,32}$`
	VERITY_CODE_REGEX = `^[a-zA-Z\\d]{6}$`
)
