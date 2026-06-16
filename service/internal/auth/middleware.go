package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func APIMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/api/login" {
			c.Next()
			return
		}
		if !strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if strings.TrimSpace(token) == "" {
			token = c.Query("token")
		}
		if !IsValidToken(strings.TrimSpace(token)) {
			c.JSON(http.StatusOK, gin.H{"code": 1001, "msg": "登录失效或已在其他设备登录", "data": nil})
			c.Abort()
			return
		}
		c.Next()
	}
}

func RootHandler(indexDir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !HasValidSession() {
			c.Redirect(http.StatusFound, "/login.html")
			return
		}
		c.File(indexDir + "/index.html")
	}
}
