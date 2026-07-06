// Package owner provides startup helpers for ensuring the configured bot owner
// exists in the database with the correct role.
package owner

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
	UpdateMany(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error)
	UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error)
}

// Registrar bootstraps the configured bot owner record.
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

// EnsureOwner upserts the configured owner user_id with role=owner and demotes
// any previous owners to admin.
func (r *Registrar) EnsureOwner(ctx context.Context, ownerID int64) error {
	if r == nil || r.users == nil {
		return errors.New("owner registrar is not initialized")
	}
	if ctx == nil {
		return errors.New("context is required")
	}
	if ownerID == 0 {
		return errors.New("owner id is required")
	}

	now := time.Now().UTC()

	demoteResult, err := r.users.UpdateMany(ctx,
		bson.M{"role": domain.RoleOwner, "user_id": bson.M{"$ne": ownerID}},
		bson.M{"$set": bson.M{
			"role":       domain.RoleAdmin,
			"updated_at": now,
		}},
	)
	if err != nil {
		return fmt.Errorf("demote previous owners: %w", err)
	}

	upsertResult, err := r.users.UpdateOne(ctx,
		bson.M{"user_id": ownerID},
		bson.M{
			"$set": bson.M{
				"user_id":    ownerID,
				"role":       domain.RoleOwner,
				"updated_at": now,
			},
			"$setOnInsert": bson.M{
				"created_at": now,
			},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("ensure owner: %w", err)
	}

	r.logger.WithFields(logging.Fields{
		"event":          "owner_bootstrap",
		"owner_id":       ownerID,
		"demoted_owners": modifiedCount(demoteResult),
		"matched_owner":  matchedCount(upsertResult),
		"upserted_owner": upsertedCount(upsertResult),
	}).Info("ensured bot owner")

	return nil
}

func modifiedCount(result *mongo.UpdateResult) int64 {
	if result == nil {
		return 0
	}
	return result.ModifiedCount
}

func matchedCount(result *mongo.UpdateResult) int64 {
	if result == nil {
		return 0
	}
	return result.MatchedCount
}

func upsertedCount(result *mongo.UpdateResult) int64 {
	if result == nil {
		return 0
	}
	return result.UpsertedCount
}
