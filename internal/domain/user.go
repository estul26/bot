package domain

import "time"

// User represents a Telegram user registered with the bot.
type User struct {
	UserID     int64     `bson:"user_id" json:"user_id"`
	Role       string    `bson:"role" json:"role"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
	LastSeenAt time.Time `bson:"last_seen_at" json:"last_seen_at"`
}
