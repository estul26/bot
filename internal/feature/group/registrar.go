// Package group provides helpers for registering and tracking group chats.
package group

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bot/internal/logging"
)

type groupCollection interface {
	UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
}

// Registrar ensures groups are persisted when the bot encounters them and keeps
// their last-seen timestamp updated.
type Registrar struct {
	groups groupCollection
	logger *logrus.Entry
}

// NewRegistrar constructs a Registrar for the provided groups collection.
func NewRegistrar(groups groupCollection, logger *logrus.Entry) *Registrar {
	if logger == nil {
		logger = logging.Logger()
	}

	return &Registrar{
		groups: groups,
		logger: logger,
	}
}

// EnsureGroup upserts the group record with the provided chat ID and updates
// last_seen_at on every call.
func (r *Registrar) EnsureGroup(ctx context.Context, chatID int64, title string) (bool, error) {
	if r == nil || r.groups == nil {
		return false, errors.New("group registrar is not initialized")
	}
	if ctx == nil {
		return false, errors.New("context is required")
	}
	if chatID == 0 {
		return false, errors.New("chat id is required")
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	updateTitle := strings.TrimSpace(title)

	setFields := bson.M{"last_seen_at": now}
	if updateTitle != "" {
		setFields["title"] = updateTitle
	}

	update := bson.M{
		"$set": setFields,
		"$setOnInsert": bson.M{
			"chat_id":   chatID,
			"joined_at": now,
		},
	}

	result, err := r.groups.UpdateOne(ctx,
		bson.M{"chat_id": chatID},
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return false, fmt.Errorf("ensure group: %w", err)
	}

	created := result != nil && result.UpsertedCount > 0
	if created {
		r.logger.WithFields(logging.Fields{
			"event":   "group_registered",
			"chat_id": chatID,
			"title":   updateTitle,
		}).Info("registered new group")
		return true, nil
	}

	r.logger.WithFields(logging.Fields{
		"event":   "group_seen",
		"chat_id": chatID,
		"title":   updateTitle,
	}).Debug("updated group last seen")

	return false, nil
}
