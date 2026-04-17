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
	presenceHandler *ginchat.PresenceHandler,
	webhookHandler *ginchat.WebhookHandler,
	friendHandler *ginchat.FriendshipHandler,
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
			conversations.GET("/:id/messages", convHandler.GetMessages)
		}

		users := api.Group("/users")
		users.Use(middleware.RequireAuth(jwtSecret))
		{
			// Static routes first
			users.GET("/search", userHandler.SearchUsers)
			users.GET("/presence", presenceHandler.GetBulkPresence)
			users.POST("/heartbeat", presenceHandler.Heartbeat)

			// Parameterized routes last
			users.PUT("/me/public-key", userHandler.UpdatePublicKey)
			users.GET("/:id/public-key", userHandler.GetPublicKey)
		}

		friendships := api.Group("/friendships")
		friendships.Use(middleware.RequireAuth(jwtSecret))
		{
			friendships.POST("/request", friendHandler.Request)
			friendships.POST("/accept", friendHandler.Accept)
			friendships.POST("/reject", friendHandler.Reject)
			friendships.GET("/pending", friendHandler.Pending)
		}

		// Centrifugo proxy webhooks — no JWT auth middleware.
		// These are called by Centrifugo server, not by end-users.
		webhooks := api.Group("/centrifugo")
		{
			webhooks.POST("/connect", webhookHandler.OnConnect)
			webhooks.POST("/disconnect", webhookHandler.OnDisconnect)
		}
	}
}
