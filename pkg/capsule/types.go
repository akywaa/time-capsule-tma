package capsule

import "time"

// Store — общий интерфейс для хранения капсул (SQLite / MongoDB)
type Store interface {
	Insert(c *Capsule) error
	GetByID(id string) (*Capsule, error)
	SetHacked(id string) (*Capsule, error)
}

// Capsule — структура капсулы времени
type Capsule struct {
	ID       string    `json:"id" bson:"_id"`
	SenderID int64     `json:"sender_id" bson:"sender_id"`
	Content  string    `json:"content" bson:"content"`
	UnlockAt time.Time `json:"unlock_at" bson:"unlock_at"`
	IsHacked bool      `json:"is_hacked" bson:"is_hacked"`
}
