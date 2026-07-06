// Package user provides helpers for user registration and lifecycle updates.
package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"bot/internal/domain"
	"bot/internal/logging"
)

type userCollection interface {
	UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
}

// Registrar ensures users are present in the database and keeps their
// last-seen timestamp updated on every interaction.
type Registrar struct {
	users  userCollection
	logger *logrus.Entry
}

// NewRegistrar constructs a Registrar for the provided users collection.
func NewRegistrar(users userCollection, logger *logrus.Entry) *Registrar {
	if logger == nil {
		logger = logging.Logger()
	}

	return &Registrar{
		users:  users,
		logger: logger,
	}
}

// EnsureUser upserts the user record with a default role if missing and updates
// last_seen_at/updated_at on every call.
func (r *Registrar) EnsureUser(ctx context.Context, userID int64) (bool, error) {
	if r == nil || r.users == nil {
		return false, errors.New("user registrar is not initialized")
	}
	if ctx == nil {
		return false, errors.New("context is required")
	}
	if userID == 0 {
		return false, errors.New("user id is required")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	update := bson.M{
		"$set": bson.M{
			"updated_at":   now,
			"last_seen_at": now,
		},
		"$setOnInsert": bson.M{
			"user_id":    userID,
			"role":       domain.RoleUser,
			"created_at": now,
		},
	}

	result, err := r.users.UpdateOne(ctx,
		bson.M{"user_id": userID},
		update,
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return false, fmt.Errorf("ensure user: %w", err)
	}

	created := result != nil && result.UpsertedCount > 0
	if created {
		r.logger.WithFields(logging.Fields{
			"event":   "user_registered",
			"user_id": userID,
		}).Info("registered new user")
		return true, nil
	}

	r.logger.WithFields(logging.Fields{
		"event":   "user_seen",
		"user_id": userID,
	}).Debug("updated user last seen")

	return false, nil
}
