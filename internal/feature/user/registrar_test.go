package user

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"bot/internal/domain"
)

func TestEnsureUserCreatesNewRecord(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	coll := newFakeUserCollection(t)
	registrar := NewRegistrar(coll, logrus.NewEntry(hookLogger))

	ctx := context.Background()
	created, err := registrar.EnsureUser(ctx, 123)
	if err != nil {
		t.Fatalf("EnsureUser returned error: %v", err)
	}
	if !created {
		t.Fatalf("expected created to be true for new user")
	}

	doc := coll.docFor(t, 123)

	assertFieldEquals(t, doc, "user_id", int64(123))
	assertFieldEquals(t, doc, "role", domain.RoleUser)

	createdAt := assertTimeField(t, doc, "created_at")
	updatedAt := assertTimeField(t, doc, "updated_at")
	lastSeen := assertTimeField(t, doc, "last_seen_at")

	if !createdAt.Equal(updatedAt) || !createdAt.Equal(lastSeen) {
		t.Fatalf("expected timestamps to match on insert, got created_at=%v updated_at=%v last_seen_at=%v", createdAt, updatedAt, lastSeen)
	}
}

func TestEnsureUserUpdatesLastSeenOnly(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	coll := newFakeUserCollection(t)

	createdAt := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
	initialLastSeen := createdAt.Add(time.Hour)

	coll.seed(t, bson.M{
		"user_id":      int64(777),
		"role":         domain.RoleOwner,
		"created_at":   createdAt,
		"updated_at":   createdAt,
		"last_seen_at": initialLastSeen,
	})

	registrar := NewRegistrar(coll, logrus.NewEntry(hookLogger))

	ctx := context.Background()
	created, err := registrar.EnsureUser(ctx, 777)
	if err != nil {
		t.Fatalf("EnsureUser returned error: %v", err)
	}
	if created {
		t.Fatalf("expected created=false for existing user")
	}

	doc := coll.docFor(t, 777)

	assertFieldEquals(t, doc, "role", domain.RoleOwner)
	assertFieldEquals(t, doc, "created_at", createdAt)

	updatedAt := assertTimeField(t, doc, "updated_at")
	lastSeen := assertTimeField(t, doc, "last_seen_at")

	if !updatedAt.Equal(lastSeen) {
		t.Fatalf("expected updated_at and last_seen_at to match, got %v and %v", updatedAt, lastSeen)
	}
	if !updatedAt.After(initialLastSeen) {
		t.Fatalf("expected last_seen_at to advance beyond %v, got %v", initialLastSeen, lastSeen)
	}
}

type fakeUserCollection struct {
	t    *testing.T
	docs map[int64]bson.M
}

func newFakeUserCollection(t *testing.T) *fakeUserCollection {
	t.Helper()
	return &fakeUserCollection{
		t:    t,
		docs: make(map[int64]bson.M),
	}
}

func (f *fakeUserCollection) UpdateOne(_ context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	filterDoc, ok := filter.(bson.M)
	if !ok {
		return nil, f.Errorf("unexpected filter type %T", filter)
	}

	userID := readInt64(f.t, filterDoc["user_id"])

	updateDoc, ok := update.(bson.M)
	if !ok {
		return nil, f.Errorf("unexpected update type %T", update)
	}

	setDoc, _ := updateDoc["$set"].(bson.M)
	setOnInsertDoc, _ := updateDoc["$setOnInsert"].(bson.M)

	upsert := updateOneUpsert(f.t, opts)

	doc, found := f.docs[userID]
	if !found && !upsert {
		return &mongo.UpdateResult{
			MatchedCount:  0,
			ModifiedCount: 0,
		}, nil
	}
	if !found {
		doc = bson.M{}
		merge(doc, setOnInsertDoc)
	}

	merge(doc, setDoc)
	f.docs[userID] = doc

	result := &mongo.UpdateResult{
		MatchedCount:  1,
		ModifiedCount: 1,
	}

	if !found && upsert {
		result.MatchedCount = 0
		result.UpsertedCount = 1
		result.UpsertedID = userID
	}

	return result, nil
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

func (f *fakeUserCollection) docFor(t *testing.T, userID int64) bson.M {
	t.Helper()

	doc, ok := f.docs[userID]
	if !ok {
		t.Fatalf("no document stored for user_id=%d", userID)
	}

	return doc
}

func (f *fakeUserCollection) seed(t *testing.T, doc bson.M) {
	t.Helper()
	idVal, ok := doc["user_id"]
	if !ok {
		t.Fatalf("seed document missing user_id: %v", doc)
	}

	userID := readInt64(t, idVal)
	f.docs[userID] = doc
}

func (f *fakeUserCollection) Errorf(format string, args ...interface{}) error {
	f.t.Helper()
	f.t.Fatalf(format, args...)
	return nil
}

func merge(dst bson.M, updates bson.M) {
	for k, v := range updates {
		dst[k] = v
	}
}

func readInt64(t *testing.T, value interface{}) int64 {
	t.Helper()

	switch v := value.(type) {
	case int64:
		return v
	case int32:
		return int64(v)
	default:
		t.Fatalf("expected int64-compatible value, got %T", value)
		return 0
	}
}

func assertFieldEquals(t *testing.T, doc bson.M, field string, expected interface{}) {
	t.Helper()

	val, ok := doc[field]
	if !ok {
		t.Fatalf("expected field %s to be set", field)
	}

	if val != expected {
		t.Fatalf("expected %s=%v, got %v", field, expected, val)
	}
}

func assertTimeField(t *testing.T, doc bson.M, field string) time.Time {
	t.Helper()

	val, ok := doc[field]
	if !ok {
		t.Fatalf("expected field %s to be set", field)
	}

	ts, ok := val.(time.Time)
	if !ok {
		t.Fatalf("expected field %s to be time.Time, got %T", field, val)
	}

	if ts.IsZero() {
		t.Fatalf("expected field %s to be non-zero", field)
	}

	return ts
}
