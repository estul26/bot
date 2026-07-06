package owner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"bot/internal/domain"
)

func TestEnsureOwnerDemotesPreviousAndUpsertsConfiguredOwner(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	fake := &fakeUsers{
		updateManyResult: &mongo.UpdateResult{ModifiedCount: 2},
		updateOneResult:  &mongo.UpdateResult{MatchedCount: 0, UpsertedCount: 1},
	}

	registrar := NewRegistrar(fake, logrus.NewEntry(hookLogger))

	ctx := context.Background()
	ownerID := int64(999)
	if err := registrar.EnsureOwner(ctx, ownerID); err != nil {
		t.Fatalf("EnsureOwner returned error: %v", err)
	}

	if len(fake.updateManyCalls) != 1 {
		t.Fatalf("expected one demotion call, got %d", len(fake.updateManyCalls))
	}

	demoteCall := fake.updateManyCalls[0]
	filter, ok := demoteCall.filter.(bson.M)
	if !ok {
		t.Fatalf("expected demote filter bson.M, got %T", demoteCall.filter)
	}
	if filter["role"] != domain.RoleOwner {
		t.Fatalf("expected demote filter role %s, got %v", domain.RoleOwner, filter["role"])
	}
	userFilter, ok := filter["user_id"].(bson.M)
	if !ok || userFilter["$ne"] != ownerID {
		t.Fatalf("expected demote filter user_id $ne %d, got %v", ownerID, filter["user_id"])
	}

	update, ok := demoteCall.update.(bson.M)
	if !ok {
		t.Fatalf("expected demote update bson.M, got %T", demoteCall.update)
	}
	setFields, ok := update["$set"].(bson.M)
	if !ok {
		t.Fatalf("expected demote update $set, got %v", update)
	}
	if setFields["role"] != domain.RoleAdmin {
		t.Fatalf("expected demoted role %s, got %v", domain.RoleAdmin, setFields["role"])
	}
	if _, ok := setFields["updated_at"].(time.Time); !ok {
		t.Fatalf("expected updated_at timestamp in demote update, got %v", setFields["updated_at"])
	}

	if len(fake.updateOneCalls) != 1 {
		t.Fatalf("expected one upsert call, got %d", len(fake.updateOneCalls))
	}
	upsertCall := fake.updateOneCalls[0]

	upsertFilter, ok := upsertCall.filter.(bson.M)
	if !ok {
		t.Fatalf("expected upsert filter bson.M, got %T", upsertCall.filter)
	}
	if upsertFilter["user_id"] != ownerID {
		t.Fatalf("expected upsert filter user_id %d, got %v", ownerID, upsertFilter["user_id"])
	}

	upsertUpdate, ok := upsertCall.update.(bson.M)
	if !ok {
		t.Fatalf("expected upsert update bson.M, got %T", upsertCall.update)
	}
	setClause, ok := upsertUpdate["$set"].(bson.M)
	if !ok {
		t.Fatalf("expected $set clause in upsert update, got %v", upsertUpdate)
	}
	if setClause["role"] != domain.RoleOwner {
		t.Fatalf("expected upsert role %s, got %v", domain.RoleOwner, setClause["role"])
	}
	if setClause["user_id"] != ownerID {
		t.Fatalf("expected upsert user_id %d, got %v", ownerID, setClause["user_id"])
	}
	if _, ok := setClause["updated_at"].(time.Time); !ok {
		t.Fatalf("expected upsert updated_at timestamp, got %v", setClause["updated_at"])
	}

	setOnInsert, ok := upsertUpdate["$setOnInsert"].(bson.M)
	if !ok {
		t.Fatalf("expected $setOnInsert clause, got %v", upsertUpdate)
	}
	if _, ok := setOnInsert["created_at"].(time.Time); !ok {
		t.Fatalf("expected created_at timestamp on insert, got %v", setOnInsert["created_at"])
	}

	if !updateOneUpsert(t, upsertCall.opts) {
		t.Fatalf("expected upsert option to be enabled, got %v", upsertCall.opts)
	}

	entry := findLogEvent(hook.AllEntries(), "owner_bootstrap")
	if entry == nil {
		t.Fatalf("expected owner_bootstrap log entry")
	}
	if entry.Data["owner_id"] != ownerID {
		t.Fatalf("expected log owner_id %d, got %v", ownerID, entry.Data["owner_id"])
	}
	if entry.Data["demoted_owners"] != int64(2) {
		t.Fatalf("expected demoted_owners=2, got %v", entry.Data["demoted_owners"])
	}
	if entry.Data["upserted_owner"] != int64(1) {
		t.Fatalf("expected upserted_owner=1, got %v", entry.Data["upserted_owner"])
	}
}

func TestEnsureOwnerValidatesAndPropagatesErrors(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	tests := []struct {
		name      string
		registrar *Registrar
		ctx       context.Context
		ownerID   int64
		expectErr string
	}{
		{
			name:      "nil registrar",
			registrar: nil,
			ctx:       context.Background(),
			ownerID:   1,
			expectErr: "owner registrar",
		},
		{
			name:      "nil collection",
			registrar: NewRegistrar(nil, logrus.NewEntry(hookLogger)),
			ctx:       context.Background(),
			ownerID:   1,
			expectErr: "registrar is not initialized",
		},
		{
			name:      "nil context",
			registrar: NewRegistrar(&fakeUsers{}, logrus.NewEntry(hookLogger)),
			ctx:       nil,
			ownerID:   1,
			expectErr: "context is required",
		},
		{
			name:      "zero owner id",
			registrar: NewRegistrar(&fakeUsers{}, logrus.NewEntry(hookLogger)),
			ctx:       context.Background(),
			ownerID:   0,
			expectErr: "owner id is required",
		},
		{
			name: "demote error",
			registrar: NewRegistrar(&fakeUsers{
				updateManyErr: errors.New("demote fail"),
			}, logrus.NewEntry(hookLogger)),
			ctx:       context.Background(),
			ownerID:   99,
			expectErr: "demote fail",
		},
		{
			name: "upsert error",
			registrar: NewRegistrar(&fakeUsers{
				updateOneErr: errors.New("upsert fail"),
			}, logrus.NewEntry(hookLogger)),
			ctx:       context.Background(),
			ownerID:   99,
			expectErr: "upsert fail",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.registrar.EnsureOwner(tt.ctx, tt.ownerID)
			if err == nil || !strings.Contains(err.Error(), tt.expectErr) {
				t.Fatalf("expected error containing %q, got %v", tt.expectErr, err)
			}
		})
	}
}

type updateManyCall struct {
	filter interface{}
	update interface{}
}

type updateOneCall struct {
	filter interface{}
	update interface{}
	opts   []options.Lister[options.UpdateOneOptions]
}

type fakeUsers struct {
	updateManyCalls  []updateManyCall
	updateOneCalls   []updateOneCall
	updateManyErr    error
	updateOneErr     error
	updateManyResult *mongo.UpdateResult
	updateOneResult  *mongo.UpdateResult
}

func (f *fakeUsers) UpdateMany(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateManyOptions]) (*mongo.UpdateResult, error) {
	f.updateManyCalls = append(f.updateManyCalls, updateManyCall{filter: filter, update: update})
	return f.updateManyResult, f.updateManyErr
}

func (f *fakeUsers) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	f.updateOneCalls = append(f.updateOneCalls, updateOneCall{filter: filter, update: update, opts: opts})
	return f.updateOneResult, f.updateOneErr
}

func updateOneUpsert(t *testing.T, opts []options.Lister[options.UpdateOneOptions]) bool {
	t.Helper()

	resolved := options.UpdateOneOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		for _, setter := range opt.List() {
			if err := setter(&resolved); err != nil {
				t.Fatalf("failed to resolve update options: %v", err)
			}
		}
	}

	return resolved.Upsert != nil && *resolved.Upsert
}

func findLogEvent(entries []*logrus.Entry, event string) *logrus.Entry {
	for _, entry := range entries {
		if entry.Data["event"] == event {
			return entry
		}
	}
	return nil
}
