package telegram

import (
	"context"
	"errors"
	"fmt"
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
	getMeUser   *models.User
	getMeErr    error
	getMeCalls  int
}

func (f *fakeBot) Start(ctx context.Context) {
	f.startedWith = ctx
}

func (f *fakeBot) GetMe(ctx context.Context) (*models.User, error) {
	f.getMeCalls++
	return f.getMeUser, f.getMeErr
}

func TestNewClientCreatesBot(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	var gotToken string
	var gotOptions []bot.Option
	runner := &fakeBot{getMeUser: &models.User{Username: "template_bot"}}

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
	if runner.getMeCalls != 1 {
		t.Fatalf("expected bot username lookup to run once, got %d", runner.getMeCalls)
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

func TestNewClientContinuesWhenBotUsernameLookupFails(t *testing.T) {
	origCreateBot := createBot
	defer func() { createBot = origCreateBot }()

	expected := errors.New("get me failed")
	runner := &fakeBot{getMeErr: expected}
	createBot = func(string, ...bot.Option) (botRunner, error) {
		return runner, nil
	}

	hookLogger, hook := logtest.NewNullLogger()
	client, err := NewClient(config.Config{TelegramToken: "token"}, logrus.NewEntry(hookLogger))
	if err != nil {
		t.Fatalf("expected NewClient to continue after username lookup failure, got %v", err)
	}
	if client == nil {
		t.Fatalf("expected client")
	}
	if runner.getMeCalls != 1 {
		t.Fatalf("expected one username lookup, got %d", runner.getMeCalls)
	}

	entry := findLogEvent(hook.AllEntries(), "telegram_identity_lookup_failed")
	if entry == nil {
		t.Fatalf("expected username lookup failure warning")
	}
	if entry.Level != logrus.WarnLevel {
		t.Fatalf("expected warning level, got %s", entry.Level)
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

func TestCacheBotUsernameStoresSuccessfulLookup(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	identity := &botIdentity{}
	runner := &fakeBot{getMeUser: &models.User{Username: "@Template_Bot"}}

	cacheBotUsername(logrus.NewEntry(hookLogger), runner, identity)

	if runner.getMeCalls != 1 {
		t.Fatalf("expected one username lookup, got %d", runner.getMeCalls)
	}
	if identity.Username() != "template_bot" {
		t.Fatalf("expected normalized username, got %q", identity.Username())
	}
	if findLogEvent(hook.AllEntries(), "telegram_identity_cached") == nil {
		t.Fatalf("expected identity cached log")
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

func TestDefaultHandlerRedactsMessageTextFromLogs(t *testing.T) {
	hookLogger, hook := logtest.NewNullLogger()
	handler := defaultHandler(logrus.NewEntry(hookLogger), nil, nil, 1, commandDiagnostics{})

	secret := "please keep this private"
	fullText := "/missing " + secret
	handler(context.Background(), nil, privateTextUpdate(42, 42, fullText))

	entries := hook.AllEntries()
	if len(entries) == 0 {
		t.Fatalf("expected log entries")
	}

	sawTextMetadata := false
	for _, entry := range entries {
		if _, ok := entry.Data["text"]; ok {
			t.Fatalf("expected no raw text field in log entry %q: %v", entry.Message, entry.Data)
		}
		if containsLogValue(entry, secret) || containsLogValue(entry, fullText) {
			t.Fatalf("expected log entry to redact message text, got %q: %v", entry.Message, entry.Data)
		}
		if entry.Data["has_text"] == true && entry.Data["text_length"] == len([]rune(fullText)) {
			sawTextMetadata = true
		}
	}

	if !sawTextMetadata {
		t.Fatalf("expected derived text metadata in at least one log entry")
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
		{name: "whoami", text: "/whoami", want: "command_whoami"},
		{name: "users", text: "/users", want: "command_users"},
		{name: "chats", text: "/chats", want: "command_chats"},
		{name: "setrole", text: "/setrole 7 admin", want: "command_setrole"},
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

func TestRouterFiltersGroupCommandMentions(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	router := newMessageRouter(logrus.NewEntry(logger), 1, commandDiagnostics{botUsername: "template_bot"})

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "unmentioned command", text: "/help", want: "command_help"},
		{name: "own mention", text: "/help@Template_Bot", want: "command_help"},
		{name: "foreign mention", text: "/help@OtherBot", want: "command_ignored_foreign_mention"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			update := groupTextUpdate(42, -100, tt.text)
			got := router.route(context.Background(), nil, update, extractUpdateMeta(update))
			if got != tt.want {
				t.Fatalf("expected route %s, got %s", tt.want, got)
			}
		})
	}
}

func TestRouterHandlesUnmentionedGroupCommandsWithoutCachedUsername(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	router := newMessageRouter(logrus.NewEntry(logger), 1, commandDiagnostics{})

	unmentioned := groupTextUpdate(42, -100, "/help")
	if got := router.route(context.Background(), nil, unmentioned, extractUpdateMeta(unmentioned)); got != "command_help" {
		t.Fatalf("expected unmentioned command to route without cached username, got %s", got)
	}

	mentioned := groupTextUpdate(42, -100, "/help@template_bot")
	if got := router.route(context.Background(), nil, mentioned, extractUpdateMeta(mentioned)); got != "command_ignored_foreign_mention" {
		t.Fatalf("expected mentioned command to be ignored without cached username, got %s", got)
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

func TestWhoamiCommandRepliesWithRole(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	whoamiCommandHandler(logrus.NewEntry(logger), commandDiagnostics{
		userFetcher: &stubUserFetcher{user: domain.User{UserID: 42, Role: domain.RoleAdmin}},
	})(context.Background(), newTestBot(t), privateTextUpdate(42, 42, "/whoami"))

	if len(*sent) != 1 {
		t.Fatalf("expected one whoami message, got %d", len(*sent))
	}
	text := (*sent)[0].Text
	for _, expected := range []string{"whoami:", "user_id: 42", "chat_id: 42", "chat_type: private", "role: admin"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected %q in whoami message %q", expected, text)
		}
	}
}

func TestListCommandsAllowAdminAndOwner(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	base := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	userLister := &stubUserLister{
		users: []domain.User{
			{UserID: 100, Role: domain.RoleUser, LastSeenAt: base},
			{UserID: 101, Role: domain.RoleAdmin, LastSeenAt: base.Add(-time.Hour)},
		},
	}
	usersCommandHandler(logrus.NewEntry(logger), commandDiagnostics{
		userFetcher: &stubUserFetcher{user: domain.User{UserID: 42, Role: domain.RoleAdmin}},
		userLister:  userLister,
	})(context.Background(), newTestBot(t), privateTextUpdate(42, 42, "/users 2"))

	groupLister := &stubGroupLister{
		groups: []domain.Group{
			{ChatID: -100, Title: "Team Chat", LastSeenAt: base},
		},
	}
	chatsCommandHandler(logrus.NewEntry(logger), commandDiagnostics{
		userFetcher: &stubUserFetcher{user: domain.User{UserID: 42, Role: domain.RoleOwner}},
		groupLister: groupLister,
	})(context.Background(), newTestBot(t), privateTextUpdate(42, 42, "/chats"))

	if userLister.limit != 2 {
		t.Fatalf("expected user list limit 2, got %d", userLister.limit)
	}
	if groupLister.limit != defaultCommandListLimit {
		t.Fatalf("expected default chat list limit %d, got %d", defaultCommandListLimit, groupLister.limit)
	}
	if len(*sent) != 2 {
		t.Fatalf("expected two list messages, got %d", len(*sent))
	}
	for _, expected := range []string{"users:", "user_id: 100", "role: user"} {
		if !strings.Contains((*sent)[0].Text, expected) {
			t.Fatalf("expected %q in users message %q", expected, (*sent)[0].Text)
		}
	}
	for _, expected := range []string{"chats:", "chat_id: -100", "title: Team Chat"} {
		if !strings.Contains((*sent)[1].Text, expected) {
			t.Fatalf("expected %q in chats message %q", expected, (*sent)[1].Text)
		}
	}
}

func TestListCommandsDenyNormalUser(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	userLister := &stubUserLister{}
	groupLister := &stubGroupLister{}
	diag := commandDiagnostics{
		userFetcher: &stubUserFetcher{user: domain.User{UserID: 7, Role: domain.RoleUser}},
		userLister:  userLister,
		groupLister: groupLister,
	}

	usersCommandHandler(logrus.NewEntry(logger), diag)(context.Background(), newTestBot(t), privateTextUpdate(7, 7, "/users"))
	chatsCommandHandler(logrus.NewEntry(logger), diag)(context.Background(), newTestBot(t), privateTextUpdate(7, 7, "/chats"))

	if len(*sent) != 2 {
		t.Fatalf("expected two denial messages, got %d", len(*sent))
	}
	if (*sent)[0].Text != "permission denied" || (*sent)[1].Text != "permission denied" {
		t.Fatalf("expected permission denials, got %q and %q", (*sent)[0].Text, (*sent)[1].Text)
	}
	if userLister.limit != 0 || groupLister.limit != 0 {
		t.Fatalf("expected listers not to be called, got user limit %d group limit %d", userLister.limit, groupLister.limit)
	}
}

func TestSetRoleCommandSuccess(t *testing.T) {
	sent, restore := stubSendMessage(t)
	defer restore()

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	setter := &stubUserRoleSetter{}

	setRoleCommandHandler(logrus.NewEntry(logger), 42, commandDiagnostics{
		userFetcher:    &stubUserFetcher{user: domain.User{UserID: 42, Role: domain.RoleOwner}},
		userRoleSetter: setter,
	})(context.Background(), newTestBot(t), privateTextUpdate(42, 42, "/setrole 7 admin"))

	if setter.calls != 1 || setter.userID != 7 || setter.role != domain.RoleAdmin {
		t.Fatalf("unexpected setrole call: calls=%d user_id=%d role=%s", setter.calls, setter.userID, setter.role)
	}
	if len(*sent) != 1 {
		t.Fatalf("expected one role update message, got %d", len(*sent))
	}
	for _, expected := range []string{"role updated", "user_id: 7", "role: admin"} {
		if !strings.Contains((*sent)[0].Text, expected) {
			t.Fatalf("expected %q in setrole message %q", expected, (*sent)[0].Text)
		}
	}
}

func TestSetRoleCommandErrors(t *testing.T) {
	tests := []struct {
		name       string
		fromUserID int64
		userRole   string
		text       string
		setterErr  error
		wantText   string
		wantCalls  int
	}{
		{
			name:       "invalid args",
			fromUserID: 42,
			userRole:   domain.RoleOwner,
			text:       "/setrole",
			wantText:   "usage: /setrole <user_id> <admin|user>",
		},
		{
			name:       "invalid role",
			fromUserID: 42,
			userRole:   domain.RoleOwner,
			text:       "/setrole 7 owner",
			wantText:   "invalid role: must be admin or user",
		},
		{
			name:       "missing user",
			fromUserID: 42,
			userRole:   domain.RoleOwner,
			text:       "/setrole 7 admin",
			setterErr:  domain.ErrUserNotFound,
			wantText:   "user not found",
			wantCalls:  1,
		},
		{
			name:       "non owner",
			fromUserID: 7,
			userRole:   domain.RoleAdmin,
			text:       "/setrole 8 admin",
			wantText:   "permission denied",
		},
		{
			name:       "configured owner target",
			fromUserID: 42,
			userRole:   domain.RoleOwner,
			text:       "/setrole 42 user",
			wantText:   "cannot change configured owner role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sent, restore := stubSendMessage(t)
			defer restore()

			logger := logrus.New()
			logger.SetOutput(io.Discard)
			setter := &stubUserRoleSetter{err: tt.setterErr}

			setRoleCommandHandler(logrus.NewEntry(logger), 42, commandDiagnostics{
				userFetcher:    &stubUserFetcher{user: domain.User{UserID: tt.fromUserID, Role: tt.userRole}},
				userRoleSetter: setter,
			})(context.Background(), newTestBot(t), privateTextUpdate(tt.fromUserID, tt.fromUserID, tt.text))

			if len(*sent) != 1 {
				t.Fatalf("expected one response, got %d", len(*sent))
			}
			if (*sent)[0].Text != tt.wantText {
				t.Fatalf("expected response %q, got %q", tt.wantText, (*sent)[0].Text)
			}
			if setter.calls != tt.wantCalls {
				t.Fatalf("expected setter calls %d, got %d", tt.wantCalls, setter.calls)
			}
		})
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

func groupTextUpdate(userID, chatID int64, text string) *models.Update {
	return &models.Update{
		Message: &models.Message{
			From: &models.User{ID: userID},
			Chat: models.Chat{ID: chatID, Type: models.ChatTypeSupergroup, Title: "Team"},
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

type stubUserLister struct {
	users []domain.User
	limit int
	err   error
}

func (s *stubUserLister) List(_ context.Context, limit int) ([]domain.User, error) {
	s.limit = limit
	return s.users, s.err
}

type stubGroupLister struct {
	groups []domain.Group
	limit  int
	err    error
}

func (s *stubGroupLister) List(_ context.Context, limit int) ([]domain.Group, error) {
	s.limit = limit
	return s.groups, s.err
}

type stubUserRoleSetter struct {
	userID int64
	role   string
	calls  int
	err    error
}

func (s *stubUserRoleSetter) SetRole(_ context.Context, userID int64, role string) error {
	s.calls++
	s.userID = userID
	s.role = role
	return s.err
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

func containsLogValue(entry *logrus.Entry, needle string) bool {
	for _, value := range entry.Data {
		if strings.Contains(fmt.Sprint(value), needle) {
			return true
		}
	}
	return strings.Contains(entry.Message, needle)
}

func findLogEvent(entries []*logrus.Entry, event string) *logrus.Entry {
	for _, entry := range entries {
		if entry.Data["event"] == event {
			return entry
		}
	}
	return nil
}
