# syntax=docker/dockerfile:1

FROM golang:1.26.4-alpine3.24 AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/bot ./cmd/bot

FROM gcr.io/distroless/static-debian13
WORKDIR /app

COPY --from=builder /bin/bot /app/bot

USER nonroot:nonroot

ENTRYPOINT ["/app/bot"]
