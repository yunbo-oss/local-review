package middleware

import (
	"errors"
	"fmt"
	"local-review-go/src/config"
	"local-review-go/src/dto"
	"local-review-go/src/httpx"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

var control = &singleflight.Group{}

// 全局JWT实例（单例模式）
var jwtInstance = NewJWT()

const (
	JWT_ISSUER         = "loser"
	JWT_TOKEN_KEY      = "authorization"
	TokenRefreshBuffer = 30 * time.Minute // 刷新阈值
	DefaultBufferTime  = 86400            // 缓冲期秒数(1天)
)

var (
	// JWT_SECRET_KEY 从环境变量读取，如果没有设置则使用默认值（生产环境必须设置）
	JWT_SECRET_KEY = getJWTSecret()
)

func getJWTSecret() string {
	secret := config.GetEnv("JWT_SECRET_KEY", "local-review-key-change-in-production")
	if secret == "local-review-key-change-in-production" {
		logrus.Warn("Using default JWT secret key! Please set JWT_SECRET_KEY environment variable in production!")
	}
	return secret
}

var (
	TokenExpired     = errors.New("token is expired")
	TokenNotValidYet = errors.New("token is not active yet")
	TokenMalformed   = errors.New("not a valid token")
	TokenInvalid     = errors.New("could not handle this token")
)

type CustomClaims struct {
	dto.UserDTO
	BufferTime int64
	jwt.RegisteredClaims
}

type JWT struct {
	SigningKey []byte
}

func NewJWT() *JWT {
	return &JWT{
		SigningKey: []byte(JWT_SECRET_KEY),
	}
}

func (j *JWT) CreateClaims(userInfo dto.UserDTO) CustomClaims {
	now := time.Now()
	return CustomClaims{
		UserDTO:    userInfo,
		BufferTime: DefaultBufferTime,
		RegisteredClaims: jwt.RegisteredClaims{
			NotBefore: jwt.NewNumericDate(now.Add(-10 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
			Issuer:    JWT_ISSUER,
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
}

func (j *JWT) CreateToken(claims CustomClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.SigningKey)
}

func (j *JWT) CreateTokenByOldToken(oldToken string, claims CustomClaims) (string, error) {
	v, err, _ := control.Do("JWT:"+oldToken, func() (interface{}, error) {
		return j.CreateToken(claims)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (j *JWT) ParseToken(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return j.SigningKey, nil
	})

	if err != nil {
		logrus.WithFields(logrus.Fields{
			"token": tokenStr,
			"error": err.Error(),
		}).Warn("JWT解析失败")

		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, TokenMalformed
		} else if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, TokenExpired
		} else if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, TokenNotValidYet
		}
		return nil, TokenInvalid
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, TokenInvalid
}

func (j *JWT) RefreshTokenWithControl(oldToken string, userDTO dto.UserDTO) (string, error) {
	// 先验证旧Token是否被篡改（忽略过期错误）
	if _, err := j.ParseToken(oldToken); err != nil && !errors.Is(err, TokenExpired) {
		return "", fmt.Errorf("无效的旧Token: %w", err)
	}
	newClaims := j.CreateClaims(userDTO)
	return j.CreateTokenByOldToken(oldToken, newClaims)
}

func GlobalTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Request.Header.Get(JWT_TOKEN_KEY)
		if token == "" {
			c.Next()
			return
		}

		claims, err := jwtInstance.ParseToken(token)
		shouldRefresh := false

		// 检查是否需要刷新
		if errors.Is(err, TokenExpired) && claims != nil {
			// 缓冲期内刷新
			bufferDeadline := claims.ExpiresAt.Add(time.Duration(claims.BufferTime) * time.Second)
			shouldRefresh = time.Now().Before(bufferDeadline)
		} else if err == nil && claims != nil {
			// 有效Token设置上下文
			c.Set("claims", claims)

			// 检查是否需要静默刷新
			if time.Until(claims.ExpiresAt.Time) < TokenRefreshBuffer {
				shouldRefresh = true
			}
		}

		// 统一处理刷新逻辑
		if shouldRefresh && claims != nil {
			newToken, refreshErr := jwtInstance.RefreshTokenWithControl(token, claims.UserDTO)
			if refreshErr == nil {
				c.Header("X-New-Token", newToken)
				c.Request.Header.Set(JWT_TOKEN_KEY, newToken)

				// 直接使用新claims（避免重复解析）
				newClaims := jwtInstance.CreateClaims(claims.UserDTO)
				c.Set("claims", &newClaims)
				logrus.Info("Token刷新成功")
			} else {
				logrus.WithError(refreshErr).Warn("Token刷新失败")
			}
		}

		c.Next()
	}
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists || claims == nil {
			c.JSON(http.StatusUnauthorized, httpx.Fail[string]("请先登录"))
			c.Abort()
			return
		}

		if _, ok := claims.(*CustomClaims); !ok {
			c.JSON(http.StatusUnauthorized, httpx.Fail[string]("无效的用户凭证"))
			c.Abort()
			return
		}

		c.Next()
	}
}

func GetUserInfo(c *gin.Context) (dto.UserDTO, error) {
	claims, exists := c.Get("claims")
	if !exists {
		return dto.UserDTO{}, errors.New("请求未经验证")
	}

	customClaims, ok := claims.(*CustomClaims)
	if !ok {
		return dto.UserDTO{}, errors.New("claims类型错误")
	}

	return customClaims.UserDTO, nil
}
