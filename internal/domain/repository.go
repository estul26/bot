package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type insertFindCollection interface {
	InsertOne(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
	FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult
}

// UserRepository persists and retrieves users in MongoDB.
type UserRepository struct {
	collection insertFindCollection
}

// NewUserRepository constructs a UserRepository.
func NewUserRepository(collection insertFindCollection) *UserRepository {
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

// GroupRepository persists and retrieves groups in MongoDB.
type GroupRepository struct {
	collection insertFindCollection
}

// NewGroupRepository constructs a GroupRepository.
func NewGroupRepository(collection insertFindCollection) *GroupRepository {
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
