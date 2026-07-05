# Telegram Bot Template

Reusable Go template for Telegram bots with MongoDB persistence, structured logging, Docker, and GitHub Actions.

## What Is Included

- Telegram long polling with `github.com/go-telegram/bot`.
- Environment loading and validation with optional `.env` support in development.
- Structured JSON logging with standard service/environment fields.
- MongoDB client lifecycle, health checks, and base indexes.
- User, group, and owner records in MongoDB.
- Basic commands:
  - `/start`
  - `/help`
  - `/ping`
  - `/status` owner-only runtime status
- Graceful shutdown on `SIGINT` and `SIGTERM`.
- Local Docker Compose stack for MongoDB and the bot.
- CI plus GHCR image release workflow.

## Configuration

Required environment variables:

- `TELEGRAM_TOKEN`
- `BOT_OWNER`
- `MONGO_URI`
- `MONGO_DB`

Optional environment variables:

- `APP_ENV` default `production`
- `LOG_LEVEL` default `info`

Recommended database names:

- Production: `telegram_bot`
- Development: `telegram_bot_dev`

## Local Development

1. Copy the local template: `cp .env.example .env`
2. Replace placeholder values in `.env`.
3. Validate Compose config: `docker compose --env-file .env -f docker-compose.local.yml config`
4. Start MongoDB: `docker compose --env-file .env -f docker-compose.local.yml up -d mongo`
5. Check config without starting polling: `APP_ENV=development go run ./cmd/bot -config-only`
6. Run the bot locally: `APP_ENV=development go run ./cmd/bot`

To run the bot in Docker too:

```sh
docker compose --env-file .env -f docker-compose.local.yml up --build bot
```

## Project Structure

- `cmd/bot`: process bootstrap, dependency wiring, and graceful shutdown.
- `internal/config`: environment contract, dotenv loading, validation, and redacted config output.
- `internal/logging`: logrus setup and contextual logging helpers.
- `internal/store`: MongoDB manager, base collection helpers, indexes, and counts.
- `internal/domain`: user/group models and role helpers.
- `internal/feature`: reusable user, group, and owner registration helpers.
- `internal/telegram`: Telegram client, routing, basic commands, and placeholder message handling.

## Extending The Template

- Add domain features under `internal/feature/<name>` or a new package under `internal`.
- Wire new services in `cmd/bot/main.go`.
- Register new Telegram commands in `internal/telegram`.
- Keep external dependencies behind small interfaces so handlers remain easy to test.
- Update `.env.example`, README, and tests when adding new required configuration.

## Checks

```sh
go fmt ./...
go mod tidy
go test ./...
go vet ./...
go build ./cmd/bot
```
