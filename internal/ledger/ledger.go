package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Ledger struct {
	db    *sql.DB
	locks sync.Map
}

type Entry struct {
	ID                int64
	UserID            string
	ChatID            string
	ModelID           string
	OpenCodeSessionID string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func Open(path string) (*Ledger, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ledger := &Ledger{db: db}
	if err := ledger.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return ledger, nil
}

func (l *Ledger) Close() error { return l.db.Close() }

func (l *Ledger) migrate(ctx context.Context) error {
	_, err := l.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS session_ledger (
id INTEGER PRIMARY KEY AUTOINCREMENT,
user_id TEXT NOT NULL,
chat_id TEXT NOT NULL,
model_id TEXT NOT NULL,
opencode_session_id TEXT NOT NULL,
created_at DATETIME NOT NULL,
updated_at DATETIME NOT NULL,
UNIQUE(user_id, chat_id, model_id)
)`)
	return err
}

func (l *Ledger) ResolveOrCreate(ctx context.Context, userID, chatID, modelID string, create func(context.Context) (string, error)) (Entry, bool, error) {
	mu := l.keyLock(userID, chatID, modelID)
	mu.Lock()
	defer mu.Unlock()

	entry, ok, err := l.Get(ctx, userID, chatID, modelID)
	if err != nil || ok {
		return entry, false, err
	}
	sessionID, err := create(ctx)
	if err != nil {
		return Entry{}, false, err
	}
	now := time.Now().UTC()
	res, err := l.db.ExecContext(ctx, `INSERT INTO session_ledger (user_id, chat_id, model_id, opencode_session_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, userID, chatID, modelID, sessionID, now, now)
	if err != nil {
		return Entry{}, false, fmt.Errorf("commit ledger mapping after OpenCode session creation failed; orphan session %s: %w", sessionID, err)
	}
	id, _ := res.LastInsertId()
	return Entry{ID: id, UserID: userID, ChatID: chatID, ModelID: modelID, OpenCodeSessionID: sessionID, CreatedAt: now, UpdatedAt: now}, true, nil
}

func (l *Ledger) Get(ctx context.Context, userID, chatID, modelID string) (Entry, bool, error) {
	row := l.db.QueryRowContext(ctx, `SELECT id, user_id, chat_id, model_id, opencode_session_id, created_at, updated_at FROM session_ledger WHERE user_id = ? AND chat_id = ? AND model_id = ?`, userID, chatID, modelID)
	var entry Entry
	err := row.Scan(&entry.ID, &entry.UserID, &entry.ChatID, &entry.ModelID, &entry.OpenCodeSessionID, &entry.CreatedAt, &entry.UpdatedAt)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	return entry, err == nil, err
}

func (l *Ledger) Touch(ctx context.Context, id int64) error {
	_, err := l.db.ExecContext(ctx, `UPDATE session_ledger SET updated_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

func (l *Ledger) Count(ctx context.Context) (int, error) {
	row := l.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_ledger`)
	var count int
	return count, row.Scan(&count)
}

func (l *Ledger) keyLock(userID, chatID, modelID string) *sync.Mutex {
	key := userID + "\x00" + chatID + "\x00" + modelID
	value, _ := l.locks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}
