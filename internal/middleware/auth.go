package middleware

import "github.com/gin-gonic/gin"

func RequireAdmin(adminID uint64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Set("user_id", adminID)
		ctx.Next()
	}
}
