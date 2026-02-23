package utils

import (
	"math/rand"
	"strconv"
)

type RandomUtils struct {
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

var RandomUtil *RandomUtils

func (*RandomUtils) GenerateVerifyCode() string {
	verifyCode := rand.Intn(900000) + 100000
	return strconv.Itoa(verifyCode)
}

func (*RandomUtils) GenerateRandomStr(l int) string {
	b := make([]byte, l)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
