package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"

	"bot/internal/config"
)

func TestNewManagerConnectsAndExposesCollections(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	cfg := config.Config{
		MongoURI: "mongodb://stub-host:27017",
		MongoDB:  "telegram_bot_test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	manager, err := NewManager(ctx, cfg)
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if manager.Database().Name() != cfg.MongoDB {
		t.Fatalf("expected database %s, got %s", cfg.MongoDB, manager.Database().Name())
	}

	if len(fake.databaseRequests) != 1 || fake.databaseRequests[0] != cfg.MongoDB {
		t.Fatalf("expected database request for %s, got %v", cfg.MongoDB, fake.databaseRequests)
	}

	if manager.Users().Name() != CollectionUsers {
		t.Fatalf("expected users collection name %s, got %s", CollectionUsers, manager.Users().Name())
	}

	if manager.Groups().Name() != CollectionGroups {
		t.Fatalf("expected groups collection name %s, got %s", CollectionGroups, manager.Groups().Name())
	}

	if err := manager.Close(ctx); err != nil {
		t.Fatalf("expected clean disconnect, got %v", err)
	}

	if !fake.disconnectCalled {
		t.Fatalf("expected disconnect to be called")
	}
}

func TestNewManagerFailsOnPingAndCleansUp(t *testing.T) {
	fake := newFakeMongoClient(t)
	fake.pingErr = errors.New("ping failed")

	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewManager(ctx, config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err == nil {
		t.Fatalf("expected ping error")
	}

	if !fake.disconnectCalled {
		t.Fatalf("expected disconnect after ping failure")
	}
}

func TestNewManagerPropagatesConnectError(t *testing.T) {
	restore := stubConnect(nil, errors.New("connect failed"))
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := NewManager(ctx, config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err == nil {
		t.Fatalf("expected connection error")
	}
}

func TestNewManagerValidatesContext(t *testing.T) {
	_, err := NewManager(nil, config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManagerCloseRequiresContext(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if err := manager.Close(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestManagerPingChecksConnectivity(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := manager.Ping(ctx); err != nil {
		t.Fatalf("expected ping to succeed, got error: %v", err)
	}

	if fake.pingCalls < 2 {
		t.Fatalf("expected ping to be invoked at least twice (init + explicit), got %d", fake.pingCalls)
	}
	if fake.lastReadPref != "primary" {
		t.Fatalf("expected ping to use primary read preference, got %q", fake.lastReadPref)
	}
}

func TestManagerPingPropagatesErrors(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	errPing := errors.New("ping failed")
	fake.pingErr = errPing

	if err := manager.Ping(context.Background()); err == nil {
		t.Fatalf("expected ping to fail")
	} else if !errors.Is(err, errPing) {
		t.Fatalf("expected ping error to wrap ping failed, got %v", err)
	}
}

func TestManagerPingValidatesContext(t *testing.T) {
	fake := newFakeMongoClient(t)
	restore := stubConnect(fake, nil)
	t.Cleanup(restore)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if err := manager.Ping(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

func TestEnsureBaseIndexesCreatesUniqueIndexes(t *testing.T) {
	fake := newFakeMongoClient(t)
	restoreConnect := stubConnect(fake, nil)
	t.Cleanup(restoreConnect)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	recorder := newIndexRecorder(t, "")
	restoreIndexes := recorder.stub()
	t.Cleanup(restoreIndexes)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := manager.EnsureBaseIndexes(ctx); err != nil {
		t.Fatalf("expected indexes to be created, got error: %v", err)
	}

	if len(recorder.calls) != 2 {
		t.Fatalf("expected 2 index creation calls, got %d", len(recorder.calls))
	}

	userCall := recorder.calls[0]
	if userCall.collection != CollectionUsers {
		t.Fatalf("expected first collection %s, got %s", CollectionUsers, userCall.collection)
	}
	assertUniqueIndex(t, userCall.models, "user_id", "user_id_unique")

	groupCall := recorder.calls[1]
	if groupCall.collection != CollectionGroups {
		t.Fatalf("expected second collection %s, got %s", CollectionGroups, groupCall.collection)
	}
	assertUniqueIndex(t, groupCall.models, "chat_id", "chat_id_unique")
}

func TestEnsureBaseIndexesFailsFastOnErrors(t *testing.T) {
	fake := newFakeMongoClient(t)
	restoreConnect := stubConnect(fake, nil)
	t.Cleanup(restoreConnect)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	recorder := newIndexRecorder(t, CollectionUsers)
	restoreIndexes := recorder.stub()
	t.Cleanup(restoreIndexes)

	err = manager.EnsureBaseIndexes(context.Background())
	if err == nil {
		t.Fatalf("expected error from index creation")
	}
	if len(recorder.calls) != 1 {
		t.Fatalf("expected to stop after first failure, got %d calls", len(recorder.calls))
	}
	if !errors.Is(err, errIndexFailure) {
		t.Fatalf("expected error to wrap index failure, got %v", err)
	}
}

func TestEnsureBaseIndexesValidatesContext(t *testing.T) {
	fake := newFakeMongoClient(t)
	restoreConnect := stubConnect(fake, nil)
	t.Cleanup(restoreConnect)

	manager, err := NewManager(context.Background(), config.Config{MongoURI: "mongodb://stub", MongoDB: "telegram_bot_test"})
	if err != nil {
		t.Fatalf("expected manager to initialize, got error: %v", err)
	}

	if err := manager.EnsureBaseIndexes(nil); err == nil {
		t.Fatalf("expected error for nil context")
	}
}

type fakeMongoClient struct {
	client           *mongo.Client
	pingErr          error
	disconnectErr    error
	disconnectCalled bool
	databaseRequests []string
	pingCalls        int
	lastReadPref     string
}

func newFakeMongoClient(t *testing.T) *fakeMongoClient {
	t.Helper()

	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://example.com:27017"))
	if err != nil {
		t.Fatalf("failed to build fake client: %v", err)
	}

	return &fakeMongoClient{client: client}
}

func (f *fakeMongoClient) Ping(_ context.Context, rp *readpref.ReadPref) error {
	f.pingCalls++
	if rp != nil {
		f.lastReadPref = rp.String()
	}
	return f.pingErr
}

func (f *fakeMongoClient) Database(name string, opts ...options.Lister[options.DatabaseOptions]) *mongo.Database {
	f.databaseRequests = append(f.databaseRequests, name)
	return f.client.Database(name, opts...)
}

func (f *fakeMongoClient) Disconnect(context.Context) error {
	f.disconnectCalled = true
	return f.disconnectErr
}

func stubConnect(fake mongoClient, err error) func() {
	prev := connectMongo
	connectMongo = func(context.Context, *options.ClientOptions) (mongoClient, error) {
		return fake, err
	}

	return func() {
		connectMongo = prev
	}
}

var errIndexFailure = errors.New("index failure")

type indexCall struct {
	collection string
	models     []mongo.IndexModel
}

type indexRecorder struct {
	t               *testing.T
	calls           []indexCall
	errorCollection string
}

func newIndexRecorder(t *testing.T, errorCollection string) *indexRecorder {
	t.Helper()
	return &indexRecorder{t: t, errorCollection: errorCollection}
}

func (r *indexRecorder) stub() func() {
	prev := createIndexes
	createIndexes = func(ctx context.Context, coll *mongo.Collection, models []mongo.IndexModel) ([]string, error) {
		r.calls = append(r.calls, indexCall{collection: coll.Name(), models: models})
		if r.errorCollection == coll.Name() {
			return nil, errIndexFailure
		}
		return []string{coll.Name() + "_idx"}, nil
	}

	return func() {
		createIndexes = prev
	}
}

func assertUniqueIndex(t *testing.T, models []mongo.IndexModel, key, name string) {
	t.Helper()
	assertIndexModel(t, models, name, bson.D{{Key: key, Value: 1}}, true)
}

func assertIndexModel(t *testing.T, models []mongo.IndexModel, name string, expectedKeys bson.D, unique bool) {
	t.Helper()

	for _, model := range models {
		indexOptions := resolveIndexOptions(t, model.Options)
		if indexOptions.Name == nil || *indexOptions.Name != name {
			continue
		}

		keysDoc, ok := model.Keys.(bson.D)
		if !ok {
			t.Fatalf("expected bson.D keys for index %s, got %T", name, model.Keys)
		}
		if len(keysDoc) != len(expectedKeys) {
			t.Fatalf("expected %d keys for index %s, got %d", len(expectedKeys), name, len(keysDoc))
		}
		for i := range expectedKeys {
			if keysDoc[i].Key != expectedKeys[i].Key || keysDoc[i].Value != expectedKeys[i].Value {
				t.Fatalf("unexpected keys for index %s: got %v, want %v", name, keysDoc, expectedKeys)
			}
		}

		if unique {
			if indexOptions.Unique == nil || !*indexOptions.Unique {
				t.Fatalf("expected unique=true for index %s", name)
			}
		} else if indexOptions.Unique != nil && *indexOptions.Unique {
			t.Fatalf("expected unique=false for index %s", name)
		}

		return
	}

	t.Fatalf("expected index %s to be present", name)
}

func resolveIndexOptions(t *testing.T, builder options.Lister[options.IndexOptions]) options.IndexOptions {
	t.Helper()
	if builder == nil {
		return options.IndexOptions{}
	}

	resolved := options.IndexOptions{}
	for _, setter := range builder.List() {
		if err := setter(&resolved); err != nil {
			t.Fatalf("failed to resolve index options: %v", err)
		}
	}
	return resolved
}
