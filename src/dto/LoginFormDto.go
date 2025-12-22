package dto

type LoginFormDto struct {
	Phone    string `json:"phone" binding:"required" validate:"required,min=11,max=11"`
	Code     string `json:"code" binding:"required" validate:"required,len=6"`
	Password string `json:"password" validate:"omitempty,min=6,max=20"`
}
