package middleware

import (
	"local-review-go/src/dto"
	"testing"
)

func TestGeratateToken(t *testing.T) {
	j := NewJWT()
	var userDTO dto.UserDTO
	userDTO.Icon = "fire_icon"
	userDTO.Id = 1
	userDTO.NickName = "fireshine"

	clamis := j.CreateClaims(userDTO)
	token, err := j.CreateToken(clamis)

	if err != nil {
		t.Fatal("expected no err")
	}

	if token == "" {
		t.Fatal("expected a token , but get a empty token")
	}

	t.Log(token)
}

func TestParseToken(t *testing.T) {
	str := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpZCI6MSwibmlja05hbWUiOiJmaXJlc2hpbmUiLCJpY29uIjoiZmlyZV9pY29uIiwiQnVmZmVyVGltZSI6ODY0MDAsImlzcyI6Imxvc2VyIiwiZXhwIjoxNzUwNjQ1NTI5LCJuYmYiOjE3NTAwMzk3MjksImlhdCI6MTc1MDA0MDcyOX0.ml7zibzOYFGcxTW2YouNYBg8Sxl4UgkAFK528oUazX8"
	j := NewJWT()
	clamis, err := j.ParseToken(str)
	if err != nil {
		t.Fatal("err!")
	}
	user := clamis.UserDTO
	t.Log(user)
}
