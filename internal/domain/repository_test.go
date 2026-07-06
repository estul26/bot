package domain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

func TestUserRepositoryListSortsByLastSeenAndLimits(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewUserRepository(coll)
	ctx := context.Background()

	base := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	for _, user := range []User{
		{UserID: 1, Role: RoleUser, LastSeenAt: base.Add(-2 * time.Hour)},
		{UserID: 2, Role: RoleAdmin, LastSeenAt: base},
		{UserID: 3, Role: RoleUser, LastSeenAt: base.Add(-time.Hour)},
	} {
		if _, err := repo.Create(ctx, user); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	users, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].UserID != 2 || users[1].UserID != 3 {
		t.Fatalf("expected users sorted by last seen desc, got %+v", users)
	}
	if coll.lastFindLimit != 2 {
		t.Fatalf("expected find limit 2, got %d", coll.lastFindLimit)
	}
}

func TestUserRepositoryListCapsLimit(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewUserRepository(coll)

	if _, err := repo.List(context.Background(), 500); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if coll.lastFindLimit != maxListLimit {
		t.Fatalf("expected capped limit %d, got %d", maxListLimit, coll.lastFindLimit)
	}
}

func TestGroupRepositoryListSortsByLastSeenAndLimits(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewGroupRepository(coll)
	ctx := context.Background()

	base := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	for _, group := range []Group{
		{ChatID: -1, Title: "Old", LastSeenAt: base.Add(-2 * time.Hour)},
		{ChatID: -2, Title: "Newest", LastSeenAt: base},
		{ChatID: -3, Title: "Middle", LastSeenAt: base.Add(-time.Hour)},
	} {
		if _, err := repo.Create(ctx, group); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	groups, err := repo.List(ctx, 2)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].ChatID != -2 || groups[1].ChatID != -3 {
		t.Fatalf("expected groups sorted by last seen desc, got %+v", groups)
	}
	if coll.lastFindLimit != 2 {
		t.Fatalf("expected find limit 2, got %d", coll.lastFindLimit)
	}
}

func TestUserRepositorySetRole(t *testing.T) {
	coll := newFakeInsertFindCollection(t)
	repo := NewUserRepository(coll)
	ctx := context.Background()

	if _, err := repo.Create(ctx, User{UserID: 42, Role: RoleUser}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := repo.SetRole(ctx, 42, RoleAdmin); err != nil {
		t.Fatalf("SetRole returned error: %v", err)
	}

	found, err := repo.GetByID(ctx, 42)
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if found.Role != RoleAdmin {
		t.Fatalf("expected role %s, got %s", RoleAdmin, found.Role)
	}

	doc := coll.docFor(t, "user_id", int64(42))
	assertTimeFieldSet(t, doc, "updated_at")
}

func TestUserRepositorySetRoleRejectsInvalidRole(t *testing.T) {
	repo := NewUserRepository(newFakeInsertFindCollection(t))

	if err := repo.SetRole(context.Background(), 42, RoleOwner); err == nil {
		t.Fatalf("expected owner role to be rejected")
	}
}

func TestUserRepositorySetRoleReturnsNotFound(t *testing.T) {
	repo := NewUserRepository(newFakeInsertFindCollection(t))

	err := repo.SetRole(context.Background(), 42, RoleAdmin)
	if !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
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
	t             *testing.T
	docs          map[string]bson.M
	lastFindLimit int64
}

func newFakeInsertFindCollection(t *testing.T) *fakeInsertFindCollection {
	t.Helper()
	return &fakeInsertFindCollection{
		t:    t,
		docs: make(map[string]bson.M),
	}
}

func (f *fakeInsertFindCollection) InsertOne(ctx context.Context, document interface{}, opts ...options.Lister[options.InsertOneOptions]) (*mongo.InsertOneResult, error) {
	doc := marshalDoc(f.t, document)
	keyName, keyVal := idField(doc)
	if keyName == "" {
		return nil, fmt.Errorf("missing id field in %v", doc)
	}

	f.docs[f.key(keyName, keyVal)] = doc
	return &mongo.InsertOneResult{InsertedID: keyVal}, nil
}

func (f *fakeInsertFindCollection) FindOne(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOneOptions]) *mongo.SingleResult {
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

func (f *fakeInsertFindCollection) Find(ctx context.Context, filter interface{}, opts ...options.Lister[options.FindOptions]) (*mongo.Cursor, error) {
	if !isEmptyFilter(filter) {
		return nil, fmt.Errorf("unexpected filter %v", filter)
	}

	findOpts := applyFindOptions(f.t, opts...)
	if findOpts.Limit != nil {
		f.lastFindLimit = *findOpts.Limit
	}

	docs := make([]bson.M, 0, len(f.docs))
	for _, doc := range f.docs {
		docs = append(docs, doc)
	}
	sort.Slice(docs, func(i, j int) bool {
		return docLastSeen(f.t, docs[i]).After(docLastSeen(f.t, docs[j]))
	})

	if findOpts.Limit != nil && *findOpts.Limit >= 0 && int64(len(docs)) > *findOpts.Limit {
		docs = docs[:*findOpts.Limit]
	}

	cursorDocs := make([]any, 0, len(docs))
	for _, doc := range docs {
		cursorDocs = append(cursorDocs, doc)
	}

	return mongo.NewCursorFromDocuments(cursorDocs, nil, nil)
}

func (f *fakeInsertFindCollection) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...options.Lister[options.UpdateOneOptions]) (*mongo.UpdateResult, error) {
	filterDoc, ok := filter.(bson.M)
	if !ok {
		return nil, fmt.Errorf("unexpected filter type %T", filter)
	}
	userID, ok := filterDoc["user_id"]
	if !ok {
		return nil, fmt.Errorf("missing user_id filter in %v", filterDoc)
	}

	key := f.key("user_id", userID)
	doc, found := f.docs[key]
	if !found {
		return &mongo.UpdateResult{}, nil
	}

	updateDoc, ok := update.(bson.M)
	if !ok {
		return nil, fmt.Errorf("unexpected update type %T", update)
	}
	setFields, ok := updateDoc["$set"].(bson.M)
	if !ok {
		return nil, fmt.Errorf("missing $set in update %v", updateDoc)
	}
	for field, value := range setFields {
		doc[field] = value
	}
	f.docs[key] = doc

	return &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}, nil
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

func isEmptyFilter(filter interface{}) bool {
	switch doc := filter.(type) {
	case bson.D:
		return len(doc) == 0
	case bson.M:
		return len(doc) == 0
	default:
		return false
	}
}

func applyFindOptions(t *testing.T, opts ...options.Lister[options.FindOptions]) options.FindOptions {
	t.Helper()

	var out options.FindOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		for _, setter := range opt.List() {
			if err := setter(&out); err != nil {
				t.Fatalf("apply find option: %v", err)
			}
		}
	}
	return out
}

func docLastSeen(t *testing.T, doc bson.M) time.Time {
	t.Helper()

	value, ok := doc["last_seen_at"]
	if !ok {
		return time.Time{}
	}
	return parseTime(t, value)
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
	case bson.DateTime:
		return v.Time()
	case time.Time:
		return v
	default:
		t.Fatalf("expected time value, got %T", value)
		return time.Time{}
	}
}
