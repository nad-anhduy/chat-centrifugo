package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/centrifugal/gocent/v3"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"be-chat-centrifugo/config"
	"be-chat-centrifugo/module/chat/business"
	"be-chat-centrifugo/module/chat/model"
	"be-chat-centrifugo/module/chat/storage"
	"be-chat-centrifugo/module/chat/transport/ginchat"
	"be-chat-centrifugo/pkg/centrifugo"
	"be-chat-centrifugo/routes"
)

func EnsureKeyspace(session *gocql.Session, keyspace string) error {
	query := fmt.Sprintf("CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};", keyspace)
	return session.Query(query).Exec()
}

func main() {
	cfg, err := config.LoadConfig("config/config.yaml")
	if err != nil {
		log.Printf("Could not load config file, falling back to ENV: %v", err)
	}

	// 1. Setup Postgres
	db, err := gorm.Open(postgres.Open(cfg.PostgresDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}

	err = db.AutoMigrate(&model.User{}, &model.Conversation{}, &model.Participant{}, &model.Friendship{})
	if err != nil {
		log.Fatalf("failed to auto migrate postgres: %v", err)
	}

	// 2. Setup ScyllaDB
	cluster := gocql.NewCluster(strings.Split(cfg.ScyllaHosts, ",")...)
	cluster.Consistency = gocql.Quorum

	var initSession *gocql.Session
	var errScylla error
	for i := 1; i <= 15; i++ {
		initSession, errScylla = cluster.CreateSession()
		if errScylla == nil {
			break
		}
		log.Printf("Failed to connect to ScyllaDB (attempt %d/15): %v. Retrying in 5 seconds...", i, errScylla)
		time.Sleep(5 * time.Second)
	}
	if errScylla != nil {
		log.Fatalf("failed to connect to scylladb after 15 attempts: %v", errScylla)
	}

	if err := EnsureKeyspace(initSession, cfg.ScyllaKeyspace); err != nil {
		log.Fatalf("failed to execute EnsureKeyspace: %v", err)
	}
	initSession.Close()

	// Re-connect with keyspace active
	cluster.Keyspace = cfg.ScyllaKeyspace
	session, err := cluster.CreateSession()
	if err != nil {
		log.Fatalf("failed to connect to scylladb with keyspace: %v", err)
	}
	defer session.Close()

	// 3. Setup Redis
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to parse redis URL: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)

	// Parse presence TTL from config (default: 5m).
	presenceTTL := 5 * time.Minute
	if cfg.RedisPresenceTTL != "" {
		parsed, err := time.ParseDuration(cfg.RedisPresenceTTL)
		if err != nil {
			log.Fatalf("invalid REDIS_PRESENCE_TTL %q: %v", cfg.RedisPresenceTTL, err)
		}
		presenceTTL = parsed
	}

	// 4. Setup Centrifugo
	c := gocent.New(gocent.Config{
		Addr: cfg.CentrifugoAPI,
		Key:  cfg.CentrifugoKey,
	})

	// 5. Initialize Repositories (Dependencies)
	postgresStore := storage.NewPostgresStore(db)
	scyllaStore := storage.NewScyllaStore(session)
	redisStore := storage.NewRedisStore(redisClient)
	publisher := centrifugo.NewPublisher(c)

	// 6. Initialize Services (Business Layer)
	authBiz := business.NewAuthBusiness(postgresStore, cfg.JWTSecret)
	chatBiz := business.NewChatBusiness(scyllaStore, publisher, postgresStore, postgresStore, redisStore, presenceTTL)
	friendBiz := business.NewFriendshipBusiness(postgresStore, postgresStore, postgresStore, publisher)

	// 7. Initialize Handlers (Transport Layer)
	authHandler := ginchat.NewAuthHandler(authBiz)
	chatHandler := ginchat.NewChatHandler(chatBiz)
	convHandler := ginchat.NewConversationHandler(chatBiz)
	userHandler := ginchat.NewUserHandler(chatBiz, friendBiz)
	friendHandler := ginchat.NewFriendshipHandler(friendBiz)
	presenceHandler := ginchat.NewPresenceHandler(chatBiz)
	webhookHandler := ginchat.NewWebhookHandler(chatBiz)

	// 8. Setup Gin routing
	r := gin.Default()

	// CORS middleware for test_client.html (browser cross-origin requests)
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	routes.SetupRoutes(r, authHandler, chatHandler, convHandler, userHandler, presenceHandler, webhookHandler, friendHandler, cfg.JWTSecret)

	routes.RegisterHealthCheck(r, db, session, c)

	// 9. Start server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	err = r.Run(":" + port)
	if err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
