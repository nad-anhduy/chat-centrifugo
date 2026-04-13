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
	username2 := fmt.Sprintf("testuser2_%d", time.Now().UnixNano())
	password := "password123"

	// Init Repositories
	postgresStore := storage.NewPostgresStore(db)
	scyllaStore := storage.NewScyllaStore(session)
	mockPub := &mockPublisher{}

	jwtSecret := "TEST_SECRET"

	// Init Business layers — postgresStore satisfies both ConversationStorage and UserStorage
	authBiz := business.NewAuthBusiness(postgresStore, jwtSecret)
	chatBiz := business.NewChatBusiness(scyllaStore, mockPub, postgresStore, postgresStore)

	// Init Handlers
	authHandler := ginchat.NewAuthHandler(authBiz)
	chatHandler := ginchat.NewChatHandler(chatBiz)
	convHandler := ginchat.NewConversationHandler(chatBiz)
	userHandler := ginchat.NewUserHandler(chatBiz)

	// Setup Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	routes.SetupRoutes(r, authHandler, chatHandler, convHandler, userHandler, jwtSecret)

	var token string
	var userID string
	var user2ID string
	var convID string

	t.Run("Test Case 1: Register, Login, verify JWT", func(t *testing.T) {
		// --- Register user 1 ---
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

		// --- Register user 2 (for group tests) ---
		regReq2 := business.RegisterReq{
			Username:  username2,
			Password:  password,
			PublicKey: "sample_public_key_2",
		}
		b2, _ := json.Marshal(regReq2)
		req2, _ := http.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBuffer(b2))
		req2.Header.Set("Content-Type", "application/json")

		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Fatalf("expected status 200 for register user2, got %d. Body: %s", w2.Code, w2.Body.String())
		}

		// --- Login user 1 ---
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

		// Retrieve userIDs from database
		user, err := postgresStore.GetUserByUsername(context.Background(), username)
		if err != nil {
			t.Fatalf("failed to fetch created user: %v", err)
		}
		userID = user.ID

		user2, err := postgresStore.GetUserByUsername(context.Background(), username2)
		if err != nil {
			t.Fatalf("failed to fetch created user2: %v", err)
		}
		user2ID = user2.ID
	})

	t.Run("Test Case 2: Create Group via API, verify all participants", func(t *testing.T) {
		groupReq := business.CreateGroupReq{
			Name:      "Test Group",
			MemberIDs: []string{userID, user2ID},
		}
		b, _ := json.Marshal(groupReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/conversations", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status 201 for create group, got %d. Body: %s", w.Code, w.Body.String())
		}

		// Parse response to get conversation ID
		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode create group response: %v", err)
		}

		convData, ok := resp["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected 'data' object in response, got: %v", resp)
		}
		convID = convData["id"].(string)

		if convID == "" {
			t.Fatalf("expected conversation ID to be populated")
		}

		// Verify ALL participants are saved (creator + members, deduplicated)
		var count int64
		err := db.Model(&model.Participant{}).
			Where("conversation_id = ?", convID).
			Count(&count).Error
		if err != nil {
			t.Fatalf("failed to query participant count: %v", err)
		}
		// Creator (userID) is in MemberIDs, so after dedup we expect exactly 2
		if count != 2 {
			t.Fatalf("expected 2 participants in group, got %d", count)
		}
	})

	t.Run("Test Case 3: Send message, verify saved to Scylla and published with sender_name", func(t *testing.T) {
		sendReq := business.SendMessageReq{
			ConversationID:   convID,
			ContentEncrypted: "encrypted_test_payload",
		}
		b, _ := json.Marshal(sendReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/v1/chat/messages", bytes.NewBuffer(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)

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

		if count != 1 {
			t.Fatalf("expected 1 message in scylla for this conversation, got %d", count)
		}

		// Verify payload sent to Centrifugo mock contains enriched fields
		if len(mockPub.PublishedMessages) != 1 {
			t.Fatalf("expected 1 published message to centrifugo, got %d", len(mockPub.PublishedMessages))
		}

		// The published payload is now a *dto.CentrifugoMessagePayload, not *model.Message.
		// We verify by checking the JSON representation contains sender_name.
		pubJSON, err := json.Marshal(mockPub.PublishedMessages[0])
		if err != nil {
			t.Fatalf("failed to marshal published message: %v", err)
		}

		var pubMap map[string]interface{}
		if err := json.Unmarshal(pubJSON, &pubMap); err != nil {
			t.Fatalf("failed to unmarshal published message: %v", err)
		}

		if pubMap["sender_name"] == nil || pubMap["sender_name"] == "" {
			t.Fatalf("expected sender_name in published payload, got: %v", pubMap)
		}
		if pubMap["conversation_id"] != convID {
			t.Fatalf("published conversation_id doesn't match: got %v, want %s", pubMap["conversation_id"], convID)
		}
		if pubMap["content_encrypted"] != "encrypted_test_payload" {
			t.Fatalf("published content_encrypted doesn't match: got %v", pubMap["content_encrypted"])
		}
	})

	t.Run("Test Case 4: GET /api/v1/conversations returns user's conversations", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for list conversations, got %d. Body: %s", w.Code, w.Body.String())
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode list conversations response: %v", err)
		}

		data, ok := resp["data"].([]interface{})
		if !ok {
			t.Fatalf("expected 'data' array in response, got: %v", resp)
		}

		if len(data) == 0 {
			t.Fatalf("expected at least 1 conversation for user, got 0")
		}

		// Verify the group conversation we created is in the list
		found := false
		for _, item := range data {
			conv, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if conv["id"] == convID {
				found = true
				if conv["name"] != "Test Group" {
					t.Fatalf("expected conversation name 'Test Group', got %v", conv["name"])
				}
				break
			}
		}

		if !found {
			t.Fatalf("conversation %s not found in user's conversation list", convID)
		}
	})
}
