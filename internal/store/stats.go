// Package store encapsulates MongoDB client management and collection helpers.
package store

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type countCollection interface {
	CountDocuments(ctx context.Context, filter interface{}, opts ...*options.CountOptions) (int64, error)
}

// StatsProvider exposes helper methods to retrieve collection counts for basic
// diagnostics without leaking MongoDB internals to callers.
type StatsProvider struct {
	users  countCollection
	groups countCollection
}

// NewStatsProvider constructs a StatsProvider backed by the provided user and
// group collections.
func NewStatsProvider(users, groups countCollection) *StatsProvider {
	return &StatsProvider{
		users:  users,
		groups: groups,
	}
}

// CountUsers returns the number of documents in the users collection.
func (p *StatsProvider) CountUsers(ctx context.Context) (int64, error) {
	if ctx == nil {
		return 0, errors.New("context is required")
	}
	if p == nil || p.users == nil {
		return 0, errors.New("stats provider is not initialized")
	}

	count, err := p.users.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

// CountGroups returns the number of documents in the groups collection.
func (p *StatsProvider) CountGroups(ctx context.Context) (int64, error) {
	if ctx == nil {
		return 0, errors.New("context is required")
	}
	if p == nil || p.groups == nil {
		return 0, errors.New("stats provider is not initialized")
	}

	count, err := p.groups.CountDocuments(ctx, bson.D{})
	if err != nil {
		return 0, fmt.Errorf("count groups: %w", err)
	}

	return count, nil
}
