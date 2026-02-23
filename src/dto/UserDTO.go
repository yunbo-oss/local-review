package dto

type UserDTO struct {
	Id       int64  `json:"id"`
	NickName string `json:"nickName"`
	Icon     string `json:"icon"`
}
