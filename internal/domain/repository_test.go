package domain

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestUserRepositoryCreateAndGet(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewUserRepository(coll)

	ctx := context.Background()
	input := User{
		UserID: 12345,
		Role:   RoleAdmin,
	}

	created, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if created.Role != RoleAdmin {
		t.Fatalf("expected role %s, got %s", RoleAdmin, created.Role)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() || created.LastSeenAt.IsZero() {
		t.Fatalf("expected timestamps to be set, got created_at=%v updated_at=%v last_seen_at=%v", created.CreatedAt, created.UpdatedAt, created.LastSeenAt)
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) || !created.CreatedAt.Equal(created.LastSeenAt) {
		t.Fatalf("expected timestamps to match on insert, got created_at=%v updated_at=%v last_seen_at=%v", created.CreatedAt, created.UpdatedAt, created.LastSeenAt)
	}

	doc := coll.docFor(t, "user_id", input.UserID)
	assertIntField(t, doc, "user_id", input.UserID)
	assertStringField(t, doc, "role", RoleAdmin)
	assertTimeFieldSet(t, doc, "created_at")
	assertTimeFieldSet(t, doc, "updated_at")
	assertTimeFieldSet(t, doc, "last_seen_at")

	found, err := repo.GetByID(ctx, input.UserID)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}

	if found.UserID != input.UserID {
		t.Fatalf("expected user_id %d, got %d", input.UserID, found.UserID)
	}
	if found.Role != RoleAdmin {
		t.Fatalf("expected role %s, got %s", RoleAdmin, found.Role)
	}
	if !found.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("expected created_at %v, got %v", created.CreatedAt, found.CreatedAt)
	}
	if !found.UpdatedAt.Equal(created.UpdatedAt) {
		t.Fatalf("expected updated_at %v, got %v", created.UpdatedAt, found.UpdatedAt)
	}
	if !found.LastSeenAt.Equal(created.LastSeenAt) {
		t.Fatalf("expected last_seen_at %v, got %v", created.LastSeenAt, found.LastSeenAt)
	}
}

func TestGroupRepositoryCreateAndGet(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewGroupRepository(coll)

	ctx := context.Background()
	input := Group{
		ChatID: -100200300,
		Title:  "Example Group",
	}

	created, err := repo.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if created.Title != input.Title {
		t.Fatalf("expected title %s, got %s", input.Title, created.Title)
	}
	if created.JoinedAt.IsZero() || created.LastSeenAt.IsZero() {
		t.Fatalf("expected timestamps to be set, got joined_at=%v last_seen_at=%v", created.JoinedAt, created.LastSeenAt)
	}
	if !created.JoinedAt.Equal(created.LastSeenAt) {
		t.Fatalf("expected joined_at and last_seen_at to match on insert, got %v and %v", created.JoinedAt, created.LastSeenAt)
	}

	doc := coll.docFor(t, "chat_id", input.ChatID)
	assertIntField(t, doc, "chat_id", input.ChatID)
	assertStringField(t, doc, "title", input.Title)
	assertTimeFieldSet(t, doc, "joined_at")
	assertTimeFieldSet(t, doc, "last_seen_at")

	found, err := repo.GetByChatID(ctx, input.ChatID)
	if err != nil {
		t.Fatalf("GetByChatID returned error: %v", err)
	}

	if found.ChatID != input.ChatID {
		t.Fatalf("expected chat_id %d, got %d", input.ChatID, found.ChatID)
	}
	if found.Title != input.Title {
		t.Fatalf("expected title %s, got %s", input.Title, found.Title)
	}
	if !found.JoinedAt.Equal(created.JoinedAt) {
		t.Fatalf("expected joined_at %v, got %v", created.JoinedAt, found.JoinedAt)
	}
	if !found.LastSeenAt.Equal(created.LastSeenAt) {
		t.Fatalf("expected last_seen_at %v, got %v", created.LastSeenAt, found.LastSeenAt)
	}
}

func TestRolePriority(t *testing.T) {
	tests := []struct {
		role     string
		expected int
	}{
		{RoleOwner, RolePriorityOwner},
		{RoleAdmin, RolePriorityAdmin},
		{RoleUser, RolePriorityUser},
		{"unknown", 0},
	}

	for _, tt := range tests {
		if got := RolePriority(tt.role); got != tt.expected {
			t.Fatalf("RolePriority(%s) = %d, want %d", tt.role, got, tt.expected)
		}
	}
}

type fakeInsertFindCollection struct {
	t    *testing.T
	docs map[string]bson.M
}

func newFakeInsertFindCollection(t *testing.T) *fakeInsertFindCollection {
	t.Helper()
	return &fakeInsertFindCollection{
		t:    t,
		docs: make(map[string]bson.M),
	}
}

func (f *fakeInsertFindCollection) InsertOne(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
	doc := marshalDoc(f.t, document)
	keyName, keyVal := idField(doc)
	if keyName == "" {
		return nil, fmt.Errorf("missing id field in %v", doc)
	}

	f.docs[f.key(keyName, keyVal)] = doc
	return &mongo.InsertOneResult{InsertedID: keyVal}, nil
}

func (f *fakeInsertFindCollection) FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
	filterDoc, ok := filter.(bson.M)
	if !ok {
		return mongo.NewSingleResultFromDocument(nil, fmt.Errorf("unexpected filter type %T", filter), nil)
	}

	for _, idKey := range []string{"user_id", "chat_id"} {
		if val, ok := filterDoc[idKey]; ok {
			doc, found := f.docs[f.key(idKey, val)]
			if !found {
				return mongo.NewSingleResultFromDocument(nil, mongo.ErrNoDocuments, nil)
			}

			return mongo.NewSingleResultFromDocument(doc, nil, nil)
		}
	}

	return mongo.NewSingleResultFromDocument(nil, fmt.Errorf("missing id filter in %v", filterDoc), nil)
}

func (f *fakeInsertFindCollection) key(field string, value interface{}) string {
	return fmt.Sprintf("%s:%v", field, value)
}

func (f *fakeInsertFindCollection) docFor(t *testing.T, field string, id interface{}) bson.M {
	t.Helper()

	doc, ok := f.docs[f.key(field, id)]
	if !ok {
		t.Fatalf("no document stored for %s=%v", field, id)
	}

	return doc
}

func marshalDoc(t *testing.T, document interface{}) bson.M {
	t.Helper()

	switch doc := document.(type) {
	case bson.M:
		return doc
	default:
		raw, err := bson.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		var out bson.M
		if err := bson.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}
		return out
	}
}

func idField(doc bson.M) (string, interface{}) {
	if val, ok := doc["user_id"]; ok {
		return "user_id", val
	}
	if val, ok := doc["chat_id"]; ok {
		return "chat_id", val
	}
	return "", nil
}

func assertStringField(t *testing.T, doc bson.M, field, expected string) {
	t.Helper()
	value, ok := doc[field]
	if !ok {
		t.Fatalf("expected %s field to be set", field)
	}
	if value != expected {
		t.Fatalf("expected %s=%s, got %v", field, expected, value)
	}
}

func assertIntField(t *testing.T, doc bson.M, field string, expected int64) {
	t.Helper()
	value, ok := doc[field]
	if !ok {
		t.Fatalf("expected %s field to be set", field)
	}

	intVal, ok := value.(int64)
	if !ok {
		t.Fatalf("expected %s to be int64, got %T", field, value)
	}

	if intVal != expected {
		t.Fatalf("expected %s=%d, got %d", field, expected, intVal)
	}
}

func assertTimeFieldSet(t *testing.T, doc bson.M, field string) {
	t.Helper()
	value, ok := doc[field]
	if !ok {
		t.Fatalf("expected %s field to be set", field)
	}

	parsed := parseTime(t, value)
	if parsed.IsZero() {
		t.Fatalf("expected %s to be non-zero", field)
	}
}

func parseTime(t *testing.T, value interface{}) time.Time {
	t.Helper()

	switch v := value.(type) {
	case primitive.DateTime:
		return v.Time()
	case time.Time:
		return v
	default:
		t.Fatalf("expected time value, got %T", value)
		return time.Time{}
	}
}
