package database

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	ID        string
	UserID    int64
	CSRFToken string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (db *DB) CreateUser(username, passwordHash string) error {
	_, err := db.Exec(
		"INSERT INTO User (username, password_hash) VALUES (?, ?)",
		username, passwordHash,
	)
	if err != nil {
		return fmt.Errorf("cannot create user %q: %w", username, err)
	}
	return nil
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.QueryRow(
		"SELECT id, username, password_hash, created_at, updated_at FROM User WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot query user %q: %w", username, err)
	}
	return u, nil
}

func (db *DB) UserCount() (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM User").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("cannot count users: %w", err)
	}
	return count, nil
}

func (db *DB) CreateSession(s *Session) error {
	_, err := db.Exec(
		"INSERT INTO Session (id, user_id, csrf_token, expires_at) VALUES (?, ?, ?, ?)",
		s.ID, s.UserID, s.CSRFToken, s.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("cannot create session: %w", err)
	}
	return nil
}

func (db *DB) GetSession(token string) (*Session, error) {
	s := &Session{}
	err := db.QueryRow(
		"SELECT id, user_id, csrf_token, created_at, expires_at FROM Session WHERE id = ?",
		token,
	).Scan(&s.ID, &s.UserID, &s.CSRFToken, &s.CreatedAt, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot query session: %w", err)
	}
	return s, nil
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.Exec("DELETE FROM Session WHERE id = ?", token)
	if err != nil {
		return fmt.Errorf("cannot delete session: %w", err)
	}
	return nil
}

func (db *DB) DeleteExpiredSessions() (int64, error) {
	result, err := db.Exec("DELETE FROM Session WHERE expires_at < ?", time.Now())
	if err != nil {
		return 0, fmt.Errorf("cannot delete expired sessions: %w", err)
	}
	return result.RowsAffected()
}
