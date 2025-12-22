# 用户认证流程详解

## 整体流程概述

这是一个基于 **JWT (JSON Web Token)** 的无状态认证系统。整个流程分为以下几个步骤：

```
前端                         后端
 │                            │
 ├─1. 请求验证码──────────────>│
 │                            │ 生成6位验证码
 │                            │ 存储到Redis (key: login:code:{phone})
 │                            │ 有效期: 2分钟
 │<───────────返回成功─────────┤
 │                            │
 ├─2. 用户输入验证码           │
 │                            │
 ├─3. 登录请求(phone+code)───>│
 │                            │ 验证验证码 (从Redis获取)
 │                            │ 验证通过 → 生成JWT Token
 │                            │ Token包含: userId, nickName, icon
 │                            │ Token有效期: 7天
 │<───────────返回Token────────┤
 │                            │
 │ 4. 前端存储Token            │
 │    (localStorage/sessionStorage) │
 │                            │
 ├─5. 后续请求携带Token───────>│
 │   Header: authorization: {token} │
 │                            │ 中间件验证Token
 │                            │ 解析用户信息
 │                            │ 设置到上下文
 │<───────────返回数据─────────┤
 │                            │
```

## 详细步骤说明

### 步骤1: 发送验证码

**前端操作：**
```javascript
// 用户输入手机号，点击"获取验证码"
POST /user/code?phone=13800138000
```

**后端处理：**
1. 验证手机号格式
2. 生成6位随机验证码（100000-999999）
3. 存储到Redis：
   - Key: `login:code:13800138000`
   - Value: `123456`（示例）
   - TTL: 2分钟
4. 返回成功响应

**Redis存储示例：**
```
Key: login:code:13800138000
Value: 123456
TTL: 120秒
```

### 步骤2: 用户登录

**前端操作：**
```javascript
// 用户输入手机号和验证码，点击"登录"
POST /user/login
Content-Type: application/json

{
    "phone": "13800138000",
    "code": "123456"
}
```

**后端处理流程：**

```64:89:src/service/UserService.go
	var user model.User
	err = user.GetUserByPhone(loginInfo.Phone)
	if err != nil {
		user.Phone = loginInfo.Phone
		user.NickName = utils.USER_NICK_NAME_PREFIX + utils.RandomUtil.GenerateRandomStr(10)
		user.CreateTime = time.Now()
		user.UpdateTime = time.Now()
		err = user.SaveUser()
		if err != nil {
			return "", err
		}
	}

	var userDTO dto.UserDTO
	userDTO.Id = user.Id
	userDTO.Icon = user.Icon
	userDTO.NickName = user.NickName

	j := middleware.NewJWT()
	clamis := j.CreateClaims(userDTO)

	token, err := j.CreateToken(clamis)
	if err != nil {
		return "", errors.New("get token failed!")
	}

	return token, nil
```

1. **验证验证码**：从Redis获取存储的验证码，与用户输入的验证码比对
2. **用户处理**：
   - 如果用户不存在 → 自动创建新用户
   - 如果用户存在 → 获取用户信息
3. **生成JWT Token**：
   - 包含用户信息：`id`, `nickName`, `icon`
   - Token有效期：7天
   - 缓冲期：1天（过期后1天内仍可刷新）
4. **返回Token**：
   ```json
   {
       "success": true,
       "data": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
       "errorMsg": "",
       "total": 0
   }
   ```

### 步骤3: 前端存储Token

**前端操作（示例）：**
```javascript
// 登录成功后
const response = await fetch('/user/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ phone: '13800138000', code: '123456' })
});

const result = await response.json();
if (result.success) {
    // 存储Token到本地
    localStorage.setItem('token', result.data);
    // 或者使用 sessionStorage（关闭浏览器后失效）
    // sessionStorage.setItem('token', result.data);
}
```

**存储位置选择：**
- **localStorage**：持久存储，关闭浏览器后仍然存在
- **sessionStorage**：会话存储，关闭浏览器后清除
- **内存变量**：页面刷新后丢失（不推荐）

### 步骤4: 后续请求携带Token

**前端操作（示例）：**
```javascript
// 创建Shop请求
const token = localStorage.getItem('token');

fetch('/shop', {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json',
        'authorization': token  // 关键：在Header中携带Token
    },
    body: JSON.stringify({
        name: '测试店铺',
        typeId: 1,
        // ... 其他字段
    })
});
```

**请求头格式：**
```
POST /shop HTTP/1.1
Host: localhost:8088
Content-Type: application/json
authorization: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

### 步骤5: 后端验证Token

**后端中间件处理流程：**

#### 5.1 全局Token中间件（GlobalTokenMiddleware）

```130:174:src/middleware/jwt.go
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
```

**功能：**
1. 从请求头获取Token（`authorization`字段）
2. 如果没有Token → 继续执行（允许匿名访问）
3. 如果有Token → 解析并验证
4. **Token自动刷新**：
   - 如果Token即将过期（30分钟内）→ 自动刷新
   - 如果Token已过期但在缓冲期内（1天）→ 自动刷新
   - 新Token通过响应头 `X-New-Token` 返回

#### 5.2 认证必需中间件（AuthRequired）

```176:193:src/middleware/jwt.go
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists || claims == nil {
			c.JSON(http.StatusUnauthorized, dto.Fail[string]("请先登录"))
			c.Abort()
			return
		}

		if _, ok := claims.(*CustomClaims); !ok {
			c.JSON(http.StatusUnauthorized, dto.Fail[string]("无效的用户凭证"))
			c.Abort()
			return
		}

		c.Next()
	}
}
```

**功能：**
1. 检查上下文中是否有用户信息（由GlobalTokenMiddleware设置）
2. 如果没有 → 返回401未授权错误
3. 如果有 → 允许继续访问

**路由配置：**

```34:42:src/handler/router.go
		shopController := authGroup.Group("/shop")

		{
			shopController.GET("/:id", shopHandler.QueryShopById)
			shopController.POST("", shopHandler.SaveShop)
			shopController.PUT("", shopHandler.UpdateShop)
			shopController.GET("/of/type", shopHandler.QueryShopByType)
			shopController.GET("/of/name", shopHandler.QueryShopByName)
		}
```

所有 `/shop` 开头的路由都需要认证（使用 `AuthRequired` 中间件）。

## Token结构

JWT Token包含以下信息：

```json
{
  "id": 1,
  "nickName": "user_zpwNDL8jgg",
  "icon": "",
  "BufferTime": 86400,
  "iss": "loser",
  "exp": 1766980159,
  "nbf": 1766374759,
  "iat": 1766375359
}
```

- **id**: 用户ID
- **nickName**: 用户昵称
- **icon**: 用户头像
- **exp**: Token过期时间（Unix时间戳）
- **BufferTime**: 缓冲期（秒），过期后仍可刷新

## 前端最佳实践

### 1. 统一请求拦截器

```javascript
// axios示例
axios.interceptors.request.use(config => {
    const token = localStorage.getItem('token');
    if (token) {
        config.headers.authorization = token;
    }
    return config;
});

// fetch示例
const apiCall = async (url, options = {}) => {
    const token = localStorage.getItem('token');
    const headers = {
        'Content-Type': 'application/json',
        ...options.headers
    };
    
    if (token) {
        headers.authorization = token;
    }
    
    const response = await fetch(url, {
        ...options,
        headers
    });
    
    // 检查是否有新Token（自动刷新）
    const newToken = response.headers.get('X-New-Token');
    if (newToken) {
        localStorage.setItem('token', newToken);
    }
    
    return response;
};
```

### 2. 处理Token过期

```javascript
axios.interceptors.response.use(
    response => {
        // 检查是否有新Token
        const newToken = response.headers['x-new-token'];
        if (newToken) {
            localStorage.setItem('token', newToken);
        }
        return response;
    },
    error => {
        if (error.response?.status === 401) {
            // Token无效或过期
            localStorage.removeItem('token');
            // 跳转到登录页
            window.location.href = '/login';
        }
        return Promise.reject(error);
    }
);
```

### 3. 自动刷新Token

后端已经实现了自动刷新机制，前端只需要：
1. 检查响应头中的 `X-New-Token`
2. 如果有新Token，更新本地存储的Token

## 安全注意事项

1. **HTTPS传输**：生产环境必须使用HTTPS，防止Token被窃取
2. **Token存储**：避免在Cookie中存储Token（防止XSS攻击）
3. **Token过期**：设置合理的过期时间（当前7天）
4. **验证码有效期**：验证码2分钟过期，防止暴力破解
5. **Token刷新**：使用缓冲期机制，避免频繁重新登录

## 总结

**完整流程：**
1. ✅ 用户输入手机号 → 后端发送验证码到Redis
2. ✅ 用户输入验证码 → 后端验证 → 返回JWT Token
3. ✅ 前端存储Token到localStorage
4. ✅ 后续请求在Header中携带Token
5. ✅ 后端中间件验证Token → 允许访问受保护资源

**关键点：**
- Token是无状态的，不需要在服务端存储会话
- Token包含用户信息，后端可以直接解析获取用户ID
- Token自动刷新机制，提升用户体验
- 使用中间件统一处理认证逻辑

