package routes

import (
	"be-chat-centrifugo/middleware"
	"be-chat-centrifugo/module/chat/transport/ginchat"
	"github.com/gin-gonic/gin"
)

func SetupRoutes(r *gin.Engine, authHandler *ginchat.AuthHandler, chatHandler *ginchat.ChatHandler, jwtSecret string) {
	api := r.Group("/api/v1")
	{
		auth := api.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
		}

		chat := api.Group("/chat")
		chat.Use(middleware.RequireAuth(jwtSecret))
		{
			chat.POST("/messages", chatHandler.SendMessage)
		}
	}
}
