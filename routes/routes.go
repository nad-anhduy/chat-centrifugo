package routes

import (
	"be-chat-centrifugo/middleware"
	"be-chat-centrifugo/module/chat/transport/ginchat"
	"github.com/gin-gonic/gin"
)

// SetupRoutes registers all API routes with their handlers and middleware.
func SetupRoutes(
	r *gin.Engine,
	authHandler *ginchat.AuthHandler,
	chatHandler *ginchat.ChatHandler,
	convHandler *ginchat.ConversationHandler,
	userHandler *ginchat.UserHandler,
	jwtSecret string,
) {
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

		conversations := api.Group("/conversations")
		conversations.Use(middleware.RequireAuth(jwtSecret))
		{
			conversations.GET("", convHandler.ListConversations)
			conversations.POST("", convHandler.CreateGroup)
		}

		users := api.Group("/users")
		users.Use(middleware.RequireAuth(jwtSecret))
		{
			users.PUT("/me/public-key", userHandler.UpdatePublicKey)
			users.GET("/:id/public-key", userHandler.GetPublicKey)
		}
	}
}
