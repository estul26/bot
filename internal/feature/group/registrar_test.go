package group

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestEnsureGroupCreatesNewRecord(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	coll := newFakeGroupCollection(t)
	registrar := NewRegistrar(coll, logrus.NewEntry(hookLogger))

	ctx := context.Background()
	created, err := registrar.EnsureGroup(ctx, -100200, " Test Group ")
	if err != nil {
		t.Fatalf("EnsureGroup returned error: %v", err)
	}
	if !created {
		t.Fatalf("expected created to be true for new group")
	}

	doc := coll.docFor(t, -100200)

	assertFieldEquals(t, doc, "chat_id", int64(-100200))
	assertFieldEquals(t, doc, "title", "Test Group")

	joinedAt := assertTimeField(t, doc, "joined_at")
	lastSeen := assertTimeField(t, doc, "last_seen_at")

	if !joinedAt.Equal(lastSeen) {
		t.Fatalf("expected joined_at and last_seen_at to match on insert, got %v and %v", joinedAt, lastSeen)
	}
}

func TestEnsureGroupUpdatesLastSeenAndTitle(t *testing.T) {
	hookLogger, _ := logtest.NewNullLogger()
	coll := newFakeGroupCollection(t)

	joinedAt := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	initialLastSeen := joinedAt.Add(time.Hour)

	coll.seed(t, bson.M{
		"chat_id":      int64(-200300),
		"title":        "Old Title",
		"joined_at":    joinedAt,
		"last_seen_at": initialLastSeen,
	})

	registrar := NewRegistrar(coll, logrus.NewEntry(hookLogger))

	ctx := context.Background()
	created, err := registrar.EnsureGroup(ctx, -200300, "Updated Title")
	if err != nil {
		t.Fatalf("EnsureGroup returned error: %v", err)
	}
	if created {
		t.Fatalf("expected created=false for existing group")
	}

	doc := coll.docFor(t, -200300)

	assertFieldEquals(t, doc, "chat_id", int64(-200300))
	assertFieldEquals(t, doc, "title", "Updated Title")
	assertFieldEquals(t, doc, "joined_at", joinedAt)

	lastSeen := assertTimeField(t, doc, "last_seen_at")
	if !lastSeen.After(initialLastSeen) {
		t.Fatalf("expected last_seen_at to advance beyond %v, got %v", initialLastSeen, lastSeen)
	}
}

type fakeGroupCollection struct {
	t    *testing.T
	docs map[int64]bson.M
}

func newFakeGroupCollection(t *testing.T) *fakeGroupCollection {
	t.Helper()
	return &fakeGroupCollection{
		t:    t,
		docs: make(map[int64]bson.M),
	}
}

func (f *fakeGroupCollection) UpdateOne(_ context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	filterDoc, ok := filter.(bson.M)
	if !ok {
		return nil, f.Errorf("unexpected filter type %T", filter)
	}

	chatID := readInt64(f.t, filterDoc["chat_id"])

	updateDoc, ok := update.(bson.M)
	if !ok {
		return nil, f.Errorf("unexpected update type %T", update)
	}

	setDoc, _ := updateDoc["$set"].(bson.M)
	setOnInsertDoc, _ := updateDoc["$setOnInsert"].(bson.M)

	upsert := len(opts) > 0 && opts[0] != nil && opts[0].Upsert != nil && *opts[0].Upsert

	doc, found := f.docs[chatID]
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
	f.docs[chatID] = doc

	result := &mongo.UpdateResult{
		MatchedCount:  1,
		ModifiedCount: 1,
	}

	if !found && upsert {
		result.MatchedCount = 0
		result.UpsertedCount = 1
		result.UpsertedID = chatID
	}

	return result, nil
}

func (f *fakeGroupCollection) docFor(t *testing.T, chatID int64) bson.M {
	t.Helper()

	doc, ok := f.docs[chatID]
	if !ok {
		t.Fatalf("no document stored for chat_id=%d", chatID)
	}

	return doc
}

func (f *fakeGroupCollection) seed(t *testing.T, doc bson.M) {
	t.Helper()

	idVal, ok := doc["chat_id"]
	if !ok {
		t.Fatalf("seed document missing chat_id: %v", doc)
	}

	chatID := readInt64(t, idVal)
	f.docs[chatID] = doc
}

func (f *fakeGroupCollection) Errorf(format string, args ...interface{}) error {
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
