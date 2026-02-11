package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := New(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateUser_Success(t *testing.T) {
	db := setupTestDB(t)

	err := db.CreateUser("admin", "$2a$10$hashedpassword")
	assert.NoError(t, err)
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)

	err := db.CreateUser("admin", "$2a$10$hash1")
	require.NoError(t, err)

	err = db.CreateUser("admin", "$2a$10$hash2")
	assert.Error(t, err)
}

func TestGetUserByUsername_Exists(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$testhash"))

	user, err := db.GetUserByUsername("admin")
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "admin", user.Username)
	assert.Equal(t, "$2a$10$testhash", user.PasswordHash)
	assert.True(t, user.ID > 0)
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	db := setupTestDB(t)

	user, err := db.GetUserByUsername("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, user)
}

func TestGetUserByID_Exists(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$testhash"))

	userByName, _ := db.GetUserByUsername("admin")
	user, err := db.GetUserByID(userByName.ID)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, "admin", user.Username)
}

func TestGetUserByID_NotFound(t *testing.T) {
	db := setupTestDB(t)

	user, err := db.GetUserByID(9999)
	assert.NoError(t, err)
	assert.Nil(t, user)
}

func TestUserCount_Empty(t *testing.T) {
	db := setupTestDB(t)

	count, err := db.UserCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestUserCount_WithUsers(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$hash"))

	count, err := db.UserCount()
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestCreateSession_Success(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$hash"))

	user, _ := db.GetUserByUsername("admin")
	session := &Session{
		ID:        "token123",
		UserID:    user.ID,
		CSRFToken: "csrf123",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	err := db.CreateSession(session)
	assert.NoError(t, err)
}

func TestGetSession_Exists(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$hash"))

	user, _ := db.GetUserByUsername("admin")
	expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	require.NoError(t, db.CreateSession(&Session{
		ID:        "token123",
		UserID:    user.ID,
		CSRFToken: "csrf456",
		ExpiresAt: expiresAt,
	}))

	session, err := db.GetSession("token123")
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, "token123", session.ID)
	assert.Equal(t, user.ID, session.UserID)
	assert.Equal(t, "csrf456", session.CSRFToken)
}

func TestGetSession_NotFound(t *testing.T) {
	db := setupTestDB(t)

	session, err := db.GetSession("nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, session)
}

func TestDeleteSession_Success(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$hash"))

	user, _ := db.GetUserByUsername("admin")
	require.NoError(t, db.CreateSession(&Session{
		ID:        "token123",
		UserID:    user.ID,
		CSRFToken: "csrf",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	err := db.DeleteSession("token123")
	assert.NoError(t, err)

	session, _ := db.GetSession("token123")
	assert.Nil(t, session)
}

func TestDeleteExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.CreateUser("admin", "$2a$10$hash"))

	user, _ := db.GetUserByUsername("admin")

	// Create expired session
	require.NoError(t, db.CreateSession(&Session{
		ID:        "expired",
		UserID:    user.ID,
		CSRFToken: "csrf1",
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}))

	// Create valid session
	require.NoError(t, db.CreateSession(&Session{
		ID:        "valid",
		UserID:    user.ID,
		CSRFToken: "csrf2",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}))

	deleted, err := db.DeleteExpiredSessions()
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Expired session should be gone
	s1, _ := db.GetSession("expired")
	assert.Nil(t, s1)

	// Valid session should remain
	s2, _ := db.GetSession("valid")
	assert.NotNil(t, s2)
}
