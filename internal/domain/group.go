package domain

import "time"

// Group represents a Telegram chat where the bot participates.
type Group struct {
	ChatID     int64     `bson:"chat_id" json:"chat_id"`
	Title      string    `bson:"title" json:"title"`
	JoinedAt   time.Time `bson:"joined_at" json:"joined_at"`
	LastSeenAt time.Time `bson:"last_seen_at" json:"last_seen_at"`
}
