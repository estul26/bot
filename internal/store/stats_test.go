package store

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestStatsProviderCountsUsersAndGroups(t *testing.T) {
	users := &stubCountCollection{count: 12}
	groups := &stubCountCollection{count: 5}

	provider := NewStatsProvider(users, groups)

	ctx := context.Background()

	userCount, err := provider.CountUsers(ctx)
	if err != nil {
		t.Fatalf("expected user count to succeed, got error: %v", err)
	}
	if userCount != 12 {
		t.Fatalf("expected 12 users, got %d", userCount)
	}
	if users.calls != 1 {
		t.Fatalf("expected users count to be called once, got %d", users.calls)
	}

	groupCount, err := provider.CountGroups(ctx)
	if err != nil {
		t.Fatalf("expected group count to succeed, got error: %v", err)
	}
	if groupCount != 5 {
		t.Fatalf("expected 5 groups, got %d", groupCount)
	}
	if groups.calls != 1 {
		t.Fatalf("expected groups count to be called once, got %d", groups.calls)
	}
}

func TestStatsProviderRequiresContext(t *testing.T) {
	provider := NewStatsProvider(&stubCountCollection{}, &stubCountCollection{})

	if _, err := provider.CountUsers(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
	if _, err := provider.CountGroups(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestStatsProviderRequiresInitialization(t *testing.T) {
	var provider *StatsProvider

	if _, err := provider.CountUsers(context.Background()); err == nil {
		t.Fatalf("expected error for nil provider")
	}
	if _, err := provider.CountGroups(context.Background()); err == nil {
		t.Fatalf("expected error for nil provider")
	}
}

func TestStatsProviderPropagatesErrors(t *testing.T) {
	expectedErr := errors.New("count failed")
	provider := NewStatsProvider(
		&stubCountCollection{err: expectedErr},
		&stubCountCollection{err: expectedErr},
	)

	if _, err := provider.CountUsers(context.Background()); err == nil {
		t.Fatalf("expected error from user count")
	}
	if _, err := provider.CountGroups(context.Background()); err == nil {
		t.Fatalf("expected error from group count")
	}
}

type stubCountCollection struct {
	count int64
	err   error
	calls int
}

func (s *stubCountCollection) CountDocuments(ctx context.Context, filter interface{}, opts ...*options.CountOptions) (int64, error) {
	s.calls++
	return s.count, s.err
}
