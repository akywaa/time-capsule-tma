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
	Contribute(id string, userID int64, amount int) (*Capsule, error)
	GeoCheck(id string, lat, lng float64) (unlocked bool, distance float64, err error)
	FindPendingReminders() ([]*Capsule, error)
	MarkReminderSent(id string) error
	CreateIndexes() error
}

// Capsule — структура капсулы времени
type Capsule struct {
	ID                 string           `json:"id" bson:"_id"`
	SenderID           int64            `json:"sender_id" bson:"sender_id"`
	Content            string           `json:"content" bson:"content"`
	UnlockAt           time.Time        `json:"unlock_at" bson:"unlock_at"`
	IsHacked           bool             `json:"is_hacked" bson:"is_hacked"`
	Passcode           string           `json:"-" bson:"passcode"`
	PasscodeAttempts   int              `json:"passcode_attempts" bson:"passcode_attempts"`
	MediaType          string           `json:"media_type" bson:"media_type"`
	Reactions          map[string]int   `json:"reactions" bson:"reactions"`
	ReactionsUsers     map[int64]string `json:"-" bson:"reactions_users"`
	ReminderSent       bool             `json:"-" bson:"reminder_sent"`
	ViewerID           int64            `json:"-" bson:"viewer_id"`
	HackPrice          int              `json:"hack_price" bson:"hack_price"`
	AllowHack          bool             `json:"allow_hack" bson:"allow_hack"`
	CapsuleType        string           `json:"capsule_type" bson:"capsule_type"`           // "personal", "group", "geo"
	ChatID             int64            `json:"-" bson:"chat_id"`
	GoalStars          int              `json:"goal_stars" bson:"goal_stars"`               // цель для группового сбора
	StarsContributions map[int64]int    `json:"stars_contributions" bson:"stars_contributions"` // userId → сумма
	GeoLat             float64          `json:"geo_lat" bson:"geo_lat"`
	GeoLng             float64          `json:"geo_lng" bson:"geo_lng"`
	GeoRadius          int              `json:"geo_radius" bson:"geo_radius"`               // метров
}
