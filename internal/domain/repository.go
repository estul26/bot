package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	defaultListLimit = 10
	maxListLimit     = 50
)

// ErrUserNotFound is returned when a role update targets a user that has not
// been registered with the bot.
var ErrUserNotFound = errors.New("user not found")

type userCollection interface {
	InsertOne(ctx context.Context, document interface{}, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error)
	FindOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	Find(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error)
	UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
}

type groupCollection interface {
	InsertOne(ctx context.Context, document interface{}, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error)
	FindOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult
	Find(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error)
}

// UserRepository persists and retrieves users in MongoDB.
type UserRepository struct {
	collection userCollection
}

// NewUserRepository constructs a UserRepository.
func NewUserRepository(collection userCollection) *UserRepository {
	return &UserRepository{collection: collection}
}

// Create inserts a user with populated timestamps, defaulting the role to
// RoleUser when omitted.
func (r *UserRepository) Create(ctx context.Context, user User) (User, error) {
	if r == nil || r.collection == nil {
		return User{}, errors.New("user repository is not initialized")
	}
	if ctx == nil {
		return User{}, errors.New("context is required")
	}
	if user.UserID == 0 {
		return User{}, errors.New("user_id is required")
	}
	if user.Role == "" {
		user.Role = RoleUser
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if user.LastSeenAt.IsZero() {
		user.LastSeenAt = now
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	if _, err := r.collection.InsertOne(ctx, user); err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}

	return user, nil
}

// GetByID fetches a user by Telegram user_id.
func (r *UserRepository) GetByID(ctx context.Context, userID int64) (User, error) {
	if r == nil || r.collection == nil {
		return User{}, errors.New("user repository is not initialized")
	}
	if ctx == nil {
		return User{}, errors.New("context is required")
	}
	if userID == 0 {
		return User{}, errors.New("user_id is required")
	}

	result := r.collection.FindOne(ctx, bson.M{"user_id": userID})
	if result == nil {
		return User{}, errors.New("find user returned no result")
	}
	if err := result.Err(); err != nil {
		return User{}, fmt.Errorf("find user: %w", err)
	}

	var user User
	if err := result.Decode(&user); err != nil {
		return User{}, fmt.Errorf("decode user: %w", err)
	}

	return user, nil
}

// List returns recently seen users, sorted by last_seen_at descending.
func (r *UserRepository) List(ctx context.Context, limit int) ([]User, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("user repository is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().
		SetSort(bson.D{{Key: "last_seen_at", Value: -1}}).
		SetLimit(int64(normalizeListLimit(limit))),
	)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	var users []User
	if err := cursor.All(ctx, &users); err != nil {
		return nil, fmt.Errorf("decode users: %w", err)
	}

	return users, nil
}

// SetRole changes an existing user's role. The owner role is reserved for the
// configured BOT_OWNER bootstrap flow.
func (r *UserRepository) SetRole(ctx context.Context, userID int64, role string) error {
	if r == nil || r.collection == nil {
		return errors.New("user repository is not initialized")
	}
	if ctx == nil {
		return errors.New("context is required")
	}
	if userID == 0 {
		return errors.New("user_id is required")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role != RoleAdmin && role != RoleUser {
		return fmt.Errorf("invalid role %q: must be %q or %q", role, RoleAdmin, RoleUser)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	result, err := r.collection.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": bson.M{
			"role":       role,
			"updated_at": now,
		}},
	)
	if err != nil {
		return fmt.Errorf("set user role: %w", err)
	}
	if result == nil || result.MatchedCount == 0 {
		return ErrUserNotFound
	}

	return nil
}

// GroupRepository persists and retrieves groups in MongoDB.
type GroupRepository struct {
	collection groupCollection
}

// NewGroupRepository constructs a GroupRepository.
func NewGroupRepository(collection groupCollection) *GroupRepository {
	return &GroupRepository{collection: collection}
}

// Create inserts a group with the current time for joined/last seen when not
// already populated.
func (r *GroupRepository) Create(ctx context.Context, group Group) (Group, error) {
	if r == nil || r.collection == nil {
		return Group{}, errors.New("group repository is not initialized")
	}
	if ctx == nil {
		return Group{}, errors.New("context is required")
	}
	if group.ChatID == 0 {
		return Group{}, errors.New("chat_id is required")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if group.JoinedAt.IsZero() {
		group.JoinedAt = now
	}
	if group.LastSeenAt.IsZero() {
		group.LastSeenAt = group.JoinedAt
	}

	if _, err := r.collection.InsertOne(ctx, group); err != nil {
		return Group{}, fmt.Errorf("insert group: %w", err)
	}

	return group, nil
}

// GetByChatID fetches a group by chat_id.
func (r *GroupRepository) GetByChatID(ctx context.Context, chatID int64) (Group, error) {
	if r == nil || r.collection == nil {
		return Group{}, errors.New("group repository is not initialized")
	}
	if ctx == nil {
		return Group{}, errors.New("context is required")
	}
	if chatID == 0 {
		return Group{}, errors.New("chat_id is required")
	}

	result := r.collection.FindOne(ctx, bson.M{"chat_id": chatID})
	if result == nil {
		return Group{}, errors.New("find group returned no result")
	}
	if err := result.Err(); err != nil {
		return Group{}, fmt.Errorf("find group: %w", err)
	}

	var group Group
	if err := result.Decode(&group); err != nil {
		return Group{}, fmt.Errorf("decode group: %w", err)
	}

	return group, nil
}

// List returns recently seen groups, sorted by last_seen_at descending.
func (r *GroupRepository) List(ctx context.Context, limit int) ([]Group, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("group repository is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	cursor, err := r.collection.Find(ctx, bson.D{}, options.Find().
		SetSort(bson.D{{Key: "last_seen_at", Value: -1}}).
		SetLimit(int64(normalizeListLimit(limit))),
	)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	var groups []Group
	if err := cursor.All(ctx, &groups); err != nil {
		return nil, fmt.Errorf("decode groups: %w", err)
	}

	return groups, nil
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}
