package telegram

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"

	"bot/internal/config"
	"bot/internal/domain"
)

type fakeBot struct {
	startedWith context.Context
}

func (f *fakeBot) Start(ctx context.Context) {
	f.startedWith = ctx
}

func TestNewClientCreatesBot(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	var gotToken string
	var gotOptions []bot.Option
	runner := &fakeBot{}

	createBot = func(token string, options ...bot.Option) (botRunner, error) {
		gotToken = token
		gotOptions = options
		return runner, nil
	}

	cfg := config.Config{TelegramToken: "token-123"}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client, err := NewClient(cfg, logrus.NewEntry(logger))
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client == nil || client.bot == nil {
		t.Fatalf("expected client and bot to be initialized")
	}
	if gotToken != cfg.TelegramToken {
		t.Fatalf("expected token %q, got %q", cfg.TelegramToken, gotToken)
	}
	if len(gotOptions) != 3 {
		t.Fatalf("expected 3 bot options, got %d", len(gotOptions))
	}
}

func TestNewClientRequiresToken(t *testing.T) {
	_, err := NewClient(config.Config{}, nil)
	if err == nil {
		t.Fatalf("expected missing token to error")
	}
}

func TestNewClientPropagatesBotError(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	expected := errors.New("boom")
	createBot = func(string, ...bot.Option) (botRunner, error) {
		return nil, expected
	}

	_, err := NewClient(config.Config{TelegramToken: "token"}, nil)
	if !errors.Is(err, expected) {
		t.Fatalf("expected error %v, got %v", expected, err)
	}
}

func TestClientStartLogsAndUsesContext(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	client := &Client{
		bot:    &fakeBot{},
		logger: logrus.NewEntry(hookLogger),
	}

	ctx := context.Background()
	client.Start(ctx)

	if fb, ok := client.bot.(*fakeBot); ok && fb.startedWith != ctx {
		t.Fatalf("expected bot to start with provided context")
	}

	entries := hook.AllEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	if entries[0].Data["event"] != "telegram_listen" {
		t.Fatalf("expected start log event, got %v", entries[0].Data["event"])
	}
	if entries[1].Data["event"] != "telegram_stopped" {
		t.Fatalf("expected stop log event, got %v", entries[1].Data["event"])
	}
}

func TestExtractUpdateMeta(t *testing.T) {
	tests := []struct {
		name   string
		update *models.Update
		want   updateMeta
	}{
		{
			name: "message",
			update: &models.Update{
				Message: &models.Message{
					From: &models.User{ID: 10},
					Chat: models.Chat{ID: 20, Type: models.ChatTypePrivate},
					Date: 1700000000,
					Text: " hello ",
				},
			},
			want: updateMeta{
				userID:     10,
				chatID:     20,
				text:       "hello",
				updateType: "message",
				chatType:   string(models.ChatTypePrivate),
				timestamp:  time.Unix(1700000000, 0).UTC(),
			},
		},
		{
			name: "edited message",
			update: &models.Update{
				EditedMessage: &models.Message{
					From:     &models.User{ID: 11},
					Chat:     models.Chat{ID: 21, Type: models.ChatTypeSupergroup, Title: "Super Chat"},
					Text:     "updated",
					Date:     1700000001,
					EditDate: 1700000020,
				},
			},
			want: updateMeta{
				userID:     11,
				chatID:     21,
				text:       "updated",
				updateType: "edited_message",
				chatType:   string(models.ChatTypeSupergroup),
				chatTitle:  "Super Chat",
				timestamp:  time.Unix(1700000020, 0).UTC(),
			},
		},
		{
			name: "my chat member",
			update: &models.Update{
				MyChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 13},
					Chat: models.Chat{ID: 23, Type: models.ChatTypeGroup, Title: "My Chat Group"},
					Date: 1700000040,
				},
			},
			want: updateMeta{
				userID:     13,
				chatID:     23,
				updateType: "my_chat_member",
				chatType:   string(models.ChatTypeGroup),
				chatTitle:  "My Chat Group",
				timestamp:  time.Unix(1700000040, 0).UTC(),
			},
		},
		{
			name: "chat member",
			update: &models.Update{
				ChatMember: &models.ChatMemberUpdated{
					From: models.User{ID: 14},
					Chat: models.Chat{ID: 24, Type: models.ChatTypeGroup, Title: "Chat Member Group"},
					Date: 1700000050,
				},
			},
			want: updateMeta{
				userID:     14,
				chatID:     24,
				updateType: "chat_member",
				chatType:   string(models.ChatTypeGroup),
				chatTitle:  "Chat Member Group",
				timestamp:  time.Unix(1700000050, 0).UTC(),
			},
		},
		{
			name:   "unknown",
			update: &models.Update{},
			want: updateMeta{
				updateType: "unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractUpdateMeta(tt.update)
			if got != tt.want {
				t.Fatalf("unexpected meta: got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDefaultHandlerRegistersUserAndGroup(t *testing.T) {
	userRegistrar := &stubUserRegistrar{}
	groupRegistrar := &stubGroupRegistrar{}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	handler := defaultHandler(logrus.NewEntry(logger), userRegistrar, groupRegistrar, 1, commandDiagnostics{})
	handler(context.Background(), nil, &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 42},
			Chat: models.Chat{ID: -100, Type: models.ChatTypeSupergroup, Title: "Team"},
			Text: "hello",
		},
	})

	if len(userRegistrar.calls) != 1 || userRegistrar.calls[0] != 42 {
		t.Fatalf("expected user registration for 42, got %v", userRegistrar.calls)
	}
	if len(groupRegistrar.calls) != 1 {
		t.Fatalf("expected one group registration, got %v", groupRegistrar.calls)
	}
	if groupRegistrar.calls[0].chatID != -100 || groupRegistrar.calls[0].title != "Team" {
		t.Fatalf("unexpected group registration call: %+v", groupRegistrar.calls[0])
	}
}

func TestRouterRoutesCommandsAndGenericMessages(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	router := newMessageRouter(logrus.NewEntry(logger), 1, commandDiagnostics{})

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "start", text: "/start", want: "command_start"},
		{name: "help mention", text: "/help@template_bot", want: "command_help"},
		{name: "unknown", text: "/missing", want: "command_unknown"},
		{name: "generic", text: "hello", want: "generic_message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := privateTextUpdate(42, 42, tt.text)
			got := router.route(context.Background(), nil, update, extractUpdateMeta(update))
			if got != tt.want {
				t.Fatalf("expected route %s, got %s", tt.want, got)
			}
		})
	}
}

func TestStartHelpPingAndGenericMessages(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	b := newTestBot(t)

	startCommandHandler(logrus.NewEntry(logger), 42)(context.Background(), b, privateTextUpdate(42, 42, "/start"))
	helpCommandHandler(logrus.NewEntry(logger))(context.Background(), b, privateTextUpdate(42, 42, "/help"))
	pingCommandHandler(logrus.NewEntry(logger), commandDiagnostics{
		appEnv:       config.EnvDevelopment,
		processStart: time.Now().Add(-2 * time.Minute),
		mongoChecker: &stubMongoChecker{},
	})(context.Background(), b, privateTextUpdate(42, 42, "/ping"))
	genericMessageHandler(logrus.NewEntry(logger))(context.Background(), b, privateTextUpdate(42, 42, "hello"))

	if len(*sent) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(*sent))
	}
	if !strings.Contains((*sent)[0].Text, "Telegram Bot Template") || !strings.Contains((*sent)[0].Text, "owner") {
		t.Fatalf("unexpected start message: %q", (*sent)[0].Text)
	}
	if !strings.Contains((*sent)[1].Text, "/status") {
		t.Fatalf("unexpected help message: %q", (*sent)[1].Text)
	}
	if !strings.Contains((*sent)[2].Text, "mongo: ok") {
		t.Fatalf("unexpected ping message: %q", (*sent)[2].Text)
	}
	if !strings.Contains((*sent)[3].Text, "Message received") {
		t.Fatalf("unexpected generic message: %q", (*sent)[3].Text)
	}
}

func TestStatusCommandRepliesForOwner(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	statusCommandHandler(logrus.NewEntry(logger), 42, commandDiagnostics{
		appEnv:        config.EnvDevelopment,
		userFetcher:   &stubUserFetcher{user: domain.User{UserID: 42, Role: domain.RoleOwner}},
		statsProvider: &stubStatsProvider{users: 12, groups: 3},
	})(context.Background(), newTestBot(t), privateTextUpdate(42, 42, "/status"))

	if len(*sent) != 1 {
		t.Fatalf("expected one status message, got %d", len(*sent))
	}
	text := (*sent)[0].Text
	for _, expected := range []string{"bot_status: running", "env: development", "connected_chats: 3", "registered_users: 12"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in status message %q", expected, text)
		}
	}
}

func TestStatusCommandDeniesNonOwner(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	statusCommandHandler(logrus.NewEntry(logger), 42, commandDiagnostics{
		userFetcher: &stubUserFetcher{user: domain.User{UserID: 7, Role: domain.RoleUser}},
	})(context.Background(), newTestBot(t), privateTextUpdate(7, 7, "/status"))

	if len(*sent) != 1 {
		t.Fatalf("expected one denial message, got %d", len(*sent))
	}
	if (*sent)[0].Text != "permission denied" {
		t.Fatalf("expected permission denied, got %q", (*sent)[0].Text)
	}
}

func TestSplitCommandTextSupportsMentionAndNewlineArgs(t *testing.T) {
	cmd, args := splitCommandText("/status@template_bot\nextra text")
	if cmd != "status" || args != "extra text" {
		t.Fatalf("unexpected split command: cmd=%q args=%q", cmd, args)
	}
}

func privateTextUpdate(userID, chatID int64, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			From: &models.User{ID: userID},
			Chat: models.Chat{ID: chatID, Type: models.ChatTypePrivate},
			Date: 1700000000,
			Text: text,
		},
	}
}

func newTestBot(t *testing.T) *bot.Bot {
	t.Helper()
	return new(bot.Bot)
}

func stubSendMessage(t *testing.T) (*[]*bot.SendMessageParams, func()) {
	t.Helper()
	orig := sendMessage
	sent := make([]*bot.SendMessageParams, 0)
	sendMessage = func(_ context.Context, _ *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
		sent = append(sent, params)
		return &models.Message{ID: len(sent)}, nil
	}
	return &sent, func() {
		sendMessage = orig
	}
}

type stubUserRegistrar struct {
	calls []int64
	err   error
}

func (s *stubUserRegistrar) EnsureUser(_ context.Context, userID int64) (bool, error) {
	s.calls = append(s.calls, userID)
	return true, s.err
}

type groupCall struct {
	chatID int64
	title  string
}

type stubGroupRegistrar struct {
	calls []groupCall
	err   error
}

func (s *stubGroupRegistrar) EnsureGroup(_ context.Context, chatID int64, title string) (bool, error) {
	s.calls = append(s.calls, groupCall{chatID: chatID, title: title})
	return true, s.err
}

type stubMongoChecker struct {
	err error
}

func (s *stubMongoChecker) Ping(context.Context) error {
	return s.err
}

type stubUserFetcher struct {
	user domain.User
	err  error
}

func (s *stubUserFetcher) GetByID(context.Context, int64) (domain.User, error) {
	return s.user, s.err
}

type stubStatsProvider struct {
	users  int64
	groups int64
	err    error
}

func (s *stubStatsProvider) CountUsers(context.Context) (int64, error) {
	return s.users, s.err
}

func (s *stubStatsProvider) CountGroups(context.Context) (int64, error) {
	return s.groups, s.err
}
