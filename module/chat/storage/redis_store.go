package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"be-chat-centrifugo/module/chat/model"

	"github.com/redis/go-redis/v9"
)

// redisPresenceValue is the internal JSON structure stored in Redis.
type redisPresenceValue struct {
	Status   model.PresenceStatus `json:"status"`
	LastSeen time.Time            `json:"last_seen"`
}

type redisStore struct {
	client *redis.Client
}

// NewRedisStore creates a new Redis-backed presence store.
func NewRedisStore(client *redis.Client) *redisStore {
	return &redisStore{client: client}
}

func presenceKey(userID string) string {
	return fmt.Sprintf("presence:%s", userID)
}

// SetOnline marks a user as online in Redis with the given TTL.
// The key auto-expires if no heartbeat refresh occurs within TTL,
// causing the user to appear offline on next lookup.
func (s *redisStore) SetOnline(ctx context.Context, userID string, ttl time.Duration) error {
	val := redisPresenceValue{
		Status:   model.PresenceStatusOnline,
		LastSeen: time.Now(),
	}

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal presence for user %q: %w", userID, err)
	}

	if err := s.client.Set(ctx, presenceKey(userID), data, ttl).Err(); err != nil {
		return fmt.Errorf("set presence online for user %q: %w", userID, err)
	}

	return nil
}

// SetOffline marks a user as offline in Redis.
// The key is kept with a 24h TTL so last_seen remains queryable.
func (s *redisStore) SetOffline(ctx context.Context, userID string) error {
	val := redisPresenceValue{
		Status:   model.PresenceStatusOffline,
		LastSeen: time.Now(),
	}

	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("marshal presence for user %q: %w", userID, err)
	}

	// Keep offline records for 24h so "last seen" is available.
	if err := s.client.Set(ctx, presenceKey(userID), data, 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("set presence offline for user %q: %w", userID, err)
	}

	return nil
}

// GetPresence retrieves the presence status for a single user.
// If the key does not exist, the user is considered offline with zero LastSeen.
func (s *redisStore) GetPresence(ctx context.Context, userID string) (*model.PresenceInfo, error) {
	data, err := s.client.Get(ctx, presenceKey(userID)).Bytes()
	if err == redis.Nil {
		return &model.PresenceInfo{
			UserID:   userID,
			Status:   model.PresenceStatusOffline,
			LastSeen: time.Time{},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get presence for user %q: %w", userID, err)
	}

	var val redisPresenceValue
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, fmt.Errorf("unmarshal presence for user %q: %w", userID, err)
	}

	return &model.PresenceInfo{
		UserID:   userID,
		Status:   val.Status,
		LastSeen: val.LastSeen,
	}, nil
}

// GetBulkPresence retrieves presence for multiple users in a single MGET call.
// Missing keys default to offline status.
func (s *redisStore) GetBulkPresence(ctx context.Context, userIDs []string) ([]model.PresenceInfo, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	keys := make([]string, len(userIDs))
	for i, id := range userIDs {
		keys[i] = presenceKey(id)
	}

	results, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("bulk get presence: %w", err)
	}

	presences := make([]model.PresenceInfo, len(userIDs))
	for i, raw := range results {
		presences[i] = model.PresenceInfo{
			UserID:   userIDs[i],
			Status:   model.PresenceStatusOffline,
			LastSeen: time.Time{},
		}

		if raw == nil {
			continue
		}

		str, ok := raw.(string)
		if !ok {
			continue
		}

		var val redisPresenceValue
		if err := json.Unmarshal([]byte(str), &val); err != nil {
			continue // graceful degradation: treat corrupted data as offline
		}

		presences[i].Status = val.Status
		presences[i].LastSeen = val.LastSeen
	}

	return presences, nil
}
