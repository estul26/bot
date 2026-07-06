# Telegram Bot Template

A reusable Go starter for Telegram bots. It gives you the boring-but-important foundation: Telegram long polling, MongoDB persistence, structured logging, Docker, local development setup, CI, and container release publishing.

Use this repository when you want to start a new Telegram bot without rebuilding configuration, logging, database setup, user/group tracking, owner bootstrap, and basic health commands from scratch.

## Features

- Telegram long polling with `github.com/go-telegram/bot`.
- MongoDB connection management, ping checks, and startup indexes.
- Automatic user and group registration.
- Owner bootstrap from `BOT_OWNER`.
- Owner-only runtime status command.
- Structured JSON logging with service and environment fields.
- `.env` loading in development only.
- `-config-only` mode for safe configuration checks.
- Dockerfile and local Docker Compose stack.
- GitHub Actions CI and GHCR image release workflow.

## Built-In Bot Commands

- `/start` shows a short welcome message and the caller role.
- `/help` lists available commands.
- `/ping` returns app environment, uptime, and MongoDB health.
- `/status` returns user/group counts for the configured owner only.

Any non-command private message receives a simple placeholder response. Replace that handler with your project-specific behavior.

## Requirements

- Go `1.26.4` or newer.
- Docker and Docker Compose for the local MongoDB stack.
- A Telegram bot token from BotFather.
- A MongoDB database, either local or hosted.

## Quick Start

1. Create a new Telegram bot with BotFather and copy the token.
2. Get your Telegram numeric user ID and use it as `BOT_OWNER`.
3. Copy the example environment file:

```sh
cp .env.example .env
```

4. Edit `.env`:

```env
APP_ENV=development
LOG_LEVEL=debug
TELEGRAM_TOKEN=123:replace-me
BOT_OWNER=123456789
MONGO_URI=mongodb://localhost:27017
MONGO_DB=telegram_bot_dev
```

5. Start MongoDB:

```sh
docker compose --env-file .env -f docker-compose.local.yml up -d mongo
```

6. Validate configuration without starting Telegram polling:

```sh
APP_ENV=development go run ./cmd/bot -config-only
```

7. Run the bot:

```sh
APP_ENV=development go run ./cmd/bot
```

8. Open Telegram and send your bot `/start`, `/ping`, and `/status`.

## Run With Docker Compose

To build and run both MongoDB and the bot locally:

```sh
docker compose --env-file .env -f docker-compose.local.yml up --build
```

To stop the stack:

```sh
docker compose --env-file .env -f docker-compose.local.yml down
```

To remove the local MongoDB volume too:

```sh
docker compose --env-file .env -f docker-compose.local.yml down -v
```

The local Compose stack uses MongoDB 8.3.4. If you already have a `mongo_data`
volume created by an older major version such as MongoDB 6.0, either recreate
the volume for disposable development data or follow MongoDB's supported upgrade
path before reusing existing data.

## Configuration

Required environment variables:

- `TELEGRAM_TOKEN`: Telegram bot token issued by BotFather.
- `BOT_OWNER`: Numeric Telegram user ID with owner privileges.
- `MONGO_URI`: MongoDB connection string.
- `MONGO_DB`: MongoDB database name.

Optional environment variables:

- `APP_ENV`: `development` or `production`; default `production`.
- `LOG_LEVEL`: logrus level such as `debug`, `info`, `warn`, or `error`; default `info`.

Recommended database names:

- Development: `telegram_bot_dev`
- Production: `telegram_bot`

The app loads `.env` only when `APP_ENV=development`. Production should provide environment variables through the runtime.

## Project Structure

- `cmd/bot`: application entrypoint, dependency wiring, startup, and graceful shutdown.
- `internal/config`: environment contract, dotenv loading, validation, and redacted config output.
- `internal/logging`: logrus setup and contextual logging helpers.
- `internal/store`: MongoDB manager, base collection helpers, indexes, pings, and counts.
- `internal/domain`: user/group models and role helpers.
- `internal/feature/user`: user registration and last-seen tracking.
- `internal/feature/group`: group registration and last-seen tracking.
- `internal/feature/owner`: owner bootstrap and previous-owner demotion.
- `internal/telegram`: Telegram client, router, commands, and placeholder message handler.

## How To Build Your Bot

1. Rename generic project identifiers if needed:
   - Go module in `go.mod`.
   - Docker image/container names in `docker-compose.local.yml`.
   - Logger `serviceName` in `internal/logging/logging.go`.
2. Add your domain logic under `internal/feature/<name>` or another package under `internal`.
3. Keep external APIs behind interfaces so handlers remain testable.
4. Wire new services in `cmd/bot/main.go`.
5. Register commands or message behavior in `internal/telegram/telegram.go`.
6. Add tests beside the package you changed.
7. Update `.env.example` and this README when adding required config.

The default private-message behavior lives in `genericMessageHandler`. That is the easiest place to start if your bot should respond to normal text messages.

## Database Collections

The template creates and indexes:

- `users`: `user_id`, `role`, `created_at`, `updated_at`, `last_seen_at`.
- `groups`: `chat_id`, `title`, `joined_at`, `last_seen_at`.

Startup ensures unique indexes on `users.user_id` and `groups.chat_id`.

## Checks

Run these before pushing changes:

```sh
go fmt ./...
go mod tidy
go test ./...
go vet ./...
go build ./cmd/bot
```

For extra confidence:

```sh
go test -race ./...
docker compose --env-file .env.example -f docker-compose.local.yml config
```

## GitHub Actions

- `ci.yml` runs formatting, tidy checks, vet, tests, race tests, vulnerability scanning, and build.
- `release.yml` builds and pushes a Docker image to GHCR after CI succeeds.

The release workflow tags images with the commit SHA, plus `main` and `latest` on the `main` branch.
