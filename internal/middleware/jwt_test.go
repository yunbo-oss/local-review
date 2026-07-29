package middleware

import "testing"

func TestGeratateToken(t *testing.T) {
	j := NewJWT()
	var userDTO AuthUser
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
	j := NewJWT()
	claims := j.CreateClaims(AuthUser{
		Id:       2,
		Icon:     "icon2",
		NickName: "user2",
	})
	token, err := j.CreateToken(claims)
	if err != nil {
		t.Fatalf("create token failed: %v", err)
	}

	parsed, err := j.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token failed: %v", err)
	}
	if parsed.AuthUser.Id != claims.AuthUser.Id {
		t.Fatalf("expected id %d, got %d", claims.AuthUser.Id, parsed.AuthUser.Id)
	}
}

func TestNormalizeAuthorizationToken(t *testing.T) {
	tests := map[string]string{
		"raw token remains compatible": "abc.def.ghi",
		"bearer token":                 "abc.def.ghi",
		"case insensitive bearer":      "abc.def.ghi",
		"empty bearer":                 "",
	}
	inputs := map[string]string{
		"raw token remains compatible": "abc.def.ghi",
		"bearer token":                 "Bearer abc.def.ghi",
		"case insensitive bearer":      "bEaReR   abc.def.ghi ",
		"empty bearer":                 "Bearer ",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeAuthorizationToken(inputs[name]); got != want {
				t.Fatalf("normalizeAuthorizationToken() = %q, want %q", got, want)
			}
		})
	}
}
