package store

// DB-backed admin sessions (A-201): a session is a 32-byte random id stored
// in the sessions table; the browser carries an HMAC-signed cookie over that
// id (internal/web/session.go). The auth slice's clock is the package-level
// Now var, injectable so the expiry boundaries are testable from this
// package's unit tests and from the web package's session tests.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Now is the auth slice's clock: every created_at/expires_at stamp and every
// expiry check in the session and token code reads it. It always returns
// UTC. Production never touches the var; tests swap it (with a cleanup
// restore) to walk the expiry boundary deterministically.
var Now = func() time.Time { return time.Now().UTC() }

// sessionTTL is both the sessions row lifetime and the gl_session cookie
// Max-Age the contract freezes (24 h, Max-Age=86400).
const sessionTTL = 24 * time.Hour

// sessionIDBytes is the entropy of a session id: 32 crypto/rand bytes,
// base64url-encoded without padding (43 characters).
const sessionIDBytes = 32

// ErrBadCredentials reports presented-but-stale credentials: a session whose
// row exists yet has passed its expires_at. A session id with no row at all
// is the everyday anonymous case and is reported through the ok flag only.
// Callers treat any ok=false as not authenticated; the sentinel exists so
// the stale-credential case stays distinguishable in logs and tests.
var ErrBadCredentials = errors.New("store: bad credentials")

// CreateSession mints a session for userID: 32 crypto/rand bytes as
// base64url id, stored with created_at now and expires_at now+24h
// (RFC3339). The id is returned for cookie signing. An unknown userID fails
// on the schema's foreign key.
func (db *DB) CreateSession(ctx context.Context, userID UserID, role string) (string, error) {
	raw := make([]byte, sessionIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("store: create session: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	stamp := Now()
	if _, err := db.sql.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, role, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		id, int64(userID), role,
		stamp.Format(time.RFC3339),
		stamp.Add(sessionTTL).Format(time.RFC3339)); err != nil {
		return "", fmt.Errorf("store: create session: %w", err)
	}
	return id, nil
}

// SessionValid reports whether id names an existing, unexpired session and
// returns its user and role. A missing (or empty) id is (0, "", false, nil) —
// the anonymous caller. An expired session is (0, "", false, err) with
// ErrBadCredentials wrapped, so callers that only branch on ok stay correct
// while logs and tests can still tell staleness from absence. Any other
// error is a database failure, wrapped with %w.
func (db *DB) SessionValid(ctx context.Context, id string) (UserID, string, bool, error) {
	if id == "" {
		return 0, "", false, nil
	}
	var (
		userID    int64
		role      string
		expiresAt string
	)
	err := db.sql.QueryRowContext(ctx,
		`SELECT user_id, role, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&userID, &role, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("store: session valid: %w", err)
	}
	exp, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return 0, "", false, fmt.Errorf("store: session valid: expires_at %q: %w", expiresAt, err)
	}
	if !Now().Before(exp) {
		return 0, "", false, fmt.Errorf("store: session valid: %w", ErrBadCredentials)
	}
	return UserID(userID), role, true, nil
}

// DeleteSession removes a session row. Deleting an unknown id is a no-op, so
// cleanup paths are idempotent.
func (db *DB) DeleteSession(ctx context.Context, id string) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}
