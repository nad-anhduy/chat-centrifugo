package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"be-chat-centrifugo/module/chat/business"
	"be-chat-centrifugo/module/chat/model"
	"be-chat-centrifugo/module/chat/storage"
	"be-chat-centrifugo/module/chat/transport/ginchat"
	"be-chat-centrifugo/routes"

	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// mockPublisher implements MessagePublisher to mock Centrifugo API
type mockPublisher struct {
	PublishedMessages []interface{}
}

func (m *mockPublisher) PublishMessage(ctx context.Context, channel string, data interface{}) error {
	m.PublishedMessages = append(m.PublishedMessages, data)
	return nil
}

// setupTestDB connects to the running local Docker containers (Postgres & ScyllaDB)
func setupTestDB(t *testing.T) (*gorm.DB, *gocql.Session) {
	// 1. Setup Postgres
	dsn := "host=localhost user=admin password=admin dbname=chat_db port=5432 sslmode=disable TimeZone=Asia/Ho_Chi_Minh"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect to postgres (make sure container is running): %v", err)
	}

	db.Exec("DROP TABLE IF EXISTS participants;")
	db.Exec("DROP TABLE IF EXISTS conversations;")
	db.Exec("DROP TABLE IF EXISTS users;")

	err = db.AutoMigrate(&model.User{}, &model.Conversation{}, &model.Participant{})
	if err != nil {
		t.Fatalf("failed to auto migrate postgres: %v", err)
	}

	// 2. Setup Scylla
	cluster := gocql.NewCluster("localhost")
	cluster.Keyspace = "chat_logs"
	cluster.Consistency = gocql.Quorum
	session, err := cluster.CreateSession()
	if err != nil {
		t.Fatalf("failed to connect to scylladb (make sure container is running): %v", err)
	}

	return db, session
}

func TestChatServiceIntegration(t *testing.T) {
	db, session := setupTestDB(t)
	defer session.Close()

	// Use unique username to isolate test execution
	username := fmt.Sprintf("testuser_%d", time.Now().UnixNano())
	password := "password123"

	// Init Repositories
	postgresStore := storage.NewPostgresStore(db)
	scyllaStore := storage.NewScyllaStore(session)
	mockPub := &mockPublisher{}

	jwtSecret := "TEST_SECRET"

	// Init Business layers
	authBiz := business.NewAuthBusiness(postgresStore, jwtSecret)
	chatBiz := business.NewChatBusiness(scyllaStore, mockPub, postgresStore)

	// Init Handlers
	authHandler := ginchat.NewAuthHandler(authBiz)
	chatHandler := ginchat.NewChatHandler(chatBiz)

	// Setup Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	routes.SetupRoutes(r, authHandler, chatHandler, jwtSecret)

	var token string
	var userID string
	var convID string

	t.Run("Test Case 1: Register, Login, verify JWT", func(t *testing.T) {
		// --- Action 1: Register ---
		regReq := business.RegisterReq{
			Username:  username,
			Password:  password,
			PublicKey: "sample_public_key",
		}
		b, _ := json.Marshal(regReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for register, got %d. Body: %s", w.Code, w.Body.String())
		}

		// --- Action 2: Login ---
		loginReq := business.LoginReq{
			Username: username,
			Password: password,
		}
		b, _ = json.Marshal(loginReq)
		req, _ = http.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")

		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for login, got %d. Body: %s", w.Code, w.Body.String())
		}

		// --- Verify JWT ---
		var loginResp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &loginResp); err != nil {
			t.Fatalf("failed to decode login response: %v", err)
		}

		tokenRaw, ok := loginResp["data"].(string)
		if !ok || tokenRaw == "" {
			t.Fatalf("expected token in login response, got: %v", loginResp)
		}
		token = tokenRaw

		// Retrieve userID from database to use in the subsequent tests
		user, err := postgresStore.GetUserByUsername(context.Background(), username)
		if err != nil {
			t.Fatalf("failed to fetch created user: %v", err)
		}
		userID = user.ID
	})

	t.Run("Test Case 2: Create Group, save Participant to Postgres", func(t *testing.T) {
		// Note: since there's no API route for Create Group, we directly test the business/storage workflow.
		// Create Conversation
		conv := &model.Conversation{
			Name: "Test Group",
			Type: model.ConversationTypeGroup,
		}
		err := postgresStore.CreateConversation(context.Background(), conv)
		if err != nil {
			t.Fatalf("failed to create conversation: %v", err)
		}

		// Wait briefly or check immediately
		if conv.ID == "" {
			t.Fatalf("expected conversation ID to be populated by DB default gen_random_uuid")
		}
		convID = conv.ID

		// Add Participant
		participant := &model.Participant{
			ConversationID: convID,
			UserID:         userID,
			JoinedAt:       time.Now(),
		}
		err = postgresStore.AddParticipant(context.Background(), participant)
		if err != nil {
			t.Fatalf("failed to add participant: %v", err)
		}

		// Verify Participant is saved
		var count int64
		err = db.Model(&model.Participant{}).
			Where("conversation_id = ? AND user_id = ?", convID, userID).
			Count(&count).Error
		if err != nil {
			t.Fatalf("failed to query participant count: %v", err)
		}
		if count == 0 {
			t.Fatalf("failed to verify that participant was saved in Postgres")
		}
	})

	t.Run("Test Case 3: Send message, verify saved to Scylla and published", func(t *testing.T) {
		// Send Message logic
		sendReq := business.SendMessageReq{
			ConversationID:   convID,
			ContentEncrypted: "encrypted_test_payload",
		}
		b, _ := json.Marshal(sendReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/chat/messages", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token) // Verify JWT via middleware

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for send message, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Verify message saved to ScyllaDB
		var count int
		q := session.Query("SELECT COUNT(1) FROM messages WHERE conversation_id = ? ALLOW FILTERING", convID)
		err := q.Scan(&count)
		if err != nil {
			t.Fatalf("failed to query message count in scylla: %v", err)
		}

		// In a real environment, depending on the test suite execution history, this conversation ID
		// should precisely have 1 message since it was freshly generated.
		if count != 1 {
			t.Fatalf("expected 1 message in scylla for this conversation, got %d", count)
		}

		// Verify payload sent to Centrifugo mock
		if len(mockPub.PublishedMessages) != 1 {
			t.Fatalf("expected 1 published message to centrifugo, got %d", len(mockPub.PublishedMessages))
		}

		publishedMsg, ok := mockPub.PublishedMessages[0].(*model.Message)
		if !ok {
			t.Fatalf("expected type *model.Message to be published")
		}

		if publishedMsg.ConversationID != convID || publishedMsg.ContentEncrypted != "encrypted_test_payload" {
			t.Fatalf("published message content doesn't match the sent request")
		}
	})
}
