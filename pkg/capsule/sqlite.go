package capsule

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

// SQLiteStore — реализация Store на SQLite
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore открывает/создаёт БД и таблицу
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS capsules (
		id TEXT PRIMARY KEY,
		sender_id INTEGER,
		content TEXT,
		unlock_at DATETIME,
		is_hacked BOOLEAN DEFAULT FALSE
	)`)
	if err != nil {
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Insert(c *Capsule) error {
	_, err := s.db.Exec(
		"INSERT INTO capsules (id, sender_id, content, unlock_at, is_hacked) VALUES (?, ?, ?, ?, ?)",
		c.ID, c.SenderID, c.Content, c.UnlockAt, c.IsHacked,
	)
	return err
}

func (s *SQLiteStore) GetByID(id string) (*Capsule, error) {
	c := &Capsule{}
	err := s.db.QueryRow(
		"SELECT id, sender_id, content, unlock_at, is_hacked FROM capsules WHERE id = ?", id,
	).Scan(&c.ID, &c.SenderID, &c.Content, &c.UnlockAt, &c.IsHacked)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (s *SQLiteStore) SetHacked(id string) (*Capsule, error) {
	_, err := s.db.Exec("UPDATE capsules SET is_hacked = true WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return s.GetByID(id)
}

// Ensure interface satisfaction
var _ Store = (*SQLiteStore)(nil)
