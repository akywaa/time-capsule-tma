package capsule

import "time"

// Store — общий интерфейс для хранения капсул (SQLite / MongoDB)
type Store interface {
	Insert(c *Capsule) error
	GetByID(id string) (*Capsule, error)
	SetHacked(id string) (*Capsule, error)
	SetViewer(id string, viewerID int64) error
	ToggleReaction(id string, userID int64, emoji string) (*Capsule, error)
	AttemptPasscode(id string, passcode string) (success bool, remaining int, err error)
	FindPendingReminders() ([]*Capsule, error)
	MarkReminderSent(id string) error
	CreateIndexes() error
}

// Capsule — структура капсулы времени
type Capsule struct {
	ID               string         `json:"id" bson:"_id"`
	SenderID         int64          `json:"sender_id" bson:"sender_id"`
	Content          string         `json:"content" bson:"content"`
	UnlockAt         time.Time      `json:"unlock_at" bson:"unlock_at"`
	IsHacked         bool           `json:"is_hacked" bson:"is_hacked"`
	Passcode         string         `json:"-" bson:"passcode"`                    // 4-значный код, пусто = без кода
	PasscodeAttempts int            `json:"passcode_attempts" bson:"passcode_attempts"` // попыток осталось (старт = 3)
	MediaType        string         `json:"media_type" bson:"media_type"`         // "text", "photo", "voice"
	Reactions        map[string]int  `json:"reactions" bson:"reactions"`            // эмодзи → кол-во
	ReactionsUsers   map[int64]string `json:"-" bson:"reactions_users"`            // userId → эмодзи
	ReminderSent     bool            `json:"-" bson:"reminder_sent"`               // отправлено ли напоминание
	ViewerID         int64          `json:"-" bson:"viewer_id"`                   // ID получателя (первый открывший)
	HackPrice        int            `json:"hack_price" bson:"hack_price"`           // цена взлома в Stars (0 = по умолчанию 50)
	AllowHack        bool           `json:"allow_hack" bson:"allow_hack"`           // разрешён ли взлом за звёзды
}
