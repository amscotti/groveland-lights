package store

// Admin identity and the one-time ADMIN_TOKEN exchange (A-201). The login
// flow (internal/web/auth.go) first compares the presented token against the
// configured ADMIN_TOKEN, then consumes it here: the login_tokens row is the
// global, server-side one-time record, so a replay from any client is
// refused even with a fresh cookie jar.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// UserID identifies one row of the users table (typed so a session or photo
// reference can never be confused with another id at compile time).
type UserID int64

const (
	// adminHandle is the single admin account's handle. The town has one
	// admin; the row is created lazily on first token exchange.
	adminHandle = "admin"

	// adminEmail is the contact recorded on the admin user row and on every
	// consumed login token row (login_tokens.email is NOT NULL).
	adminEmail = "admin"
)

// loginTokenTTL is how long a consumed token row stays recorded (the row is
// the replay record, so expiry only bounds housekeeping, not the one-time
// rule: a consumed token never validates again regardless of age).
const loginTokenTTL = 24 * time.Hour

// ErrTokenReused reports a replayed one-time login token: the token_hash row
// already exists with used_at set. Callers answer 403 without revealing
// whether the presented token was otherwise correct.
var ErrTokenReused = errors.New("store: login token already used")

// EnsureAdminUser idempotently ensures the admin account exists and returns
// its id: INSERT ... ON CONFLICT(handle) DO NOTHING followed by the SELECT,
// so the first exchange creates the row and every later call returns the
// same id.
func (db *DB) EnsureAdminUser(ctx context.Context) (UserID, error) {
	if _, err := db.sql.ExecContext(ctx, `
		INSERT INTO users (handle, email, role, created_at)
		VALUES (?, ?, 'admin', ?)
		ON CONFLICT(handle) DO NOTHING`,
		adminHandle, adminEmail, Now().Format(time.RFC3339)); err != nil {
		return 0, fmt.Errorf("store: ensure admin user: %w", err)
	}
	var id int64
	if err := db.sql.QueryRowContext(ctx,
		`SELECT id FROM users WHERE handle = ?`, adminHandle).Scan(&id); err != nil {
		return 0, fmt.Errorf("store: ensure admin user: select: %w", err)
	}
	return UserID(id), nil
}

// ConsumeAdminToken records the one-time use of the admin login token. The
// first call inserts the login_tokens row (token_hash = hex(sha256(token)),
// email admin, expires_at now+24h, used_at now) and succeeds; a second call
// for the same token finds used_at set and wraps ErrTokenReused. Any other
// database failure is wrapped with %w. Consumption is global and permanent:
// it does not depend on which client presents the token and does not lapse
// when the row's expires_at passes.
func (db *DB) ConsumeAdminToken(ctx context.Context, token []byte) error {
	sum := sha256.Sum256(token)
	tokenHash := hex.EncodeToString(sum[:])
	stamp := Now()
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO login_tokens (token_hash, email, expires_at, used_at)
		VALUES (?, ?, ?, ?)`,
		tokenHash, adminEmail,
		stamp.Add(loginTokenTTL).Format(time.RFC3339),
		stamp.Format(time.RFC3339))
	if err == nil {
		return nil
	}
	// The INSERT can only conflict on the token_hash primary key: the token
	// was consumed before. Confirm through the stored row so a driver-level
	// failure is never misread as a replay (and vice versa).
	var usedAt sql.NullString
	qerr := db.sql.QueryRowContext(ctx,
		`SELECT used_at FROM login_tokens WHERE token_hash = ?`, tokenHash).Scan(&usedAt)
	switch {
	case qerr == nil && usedAt.Valid:
		return fmt.Errorf("store: consume admin token: %w", ErrTokenReused)
	case qerr == nil:
		// A row exists but was never marked used — unreachable through this
		// API (used_at is stamped at insert time). Consume it now so the
		// first use still wins exactly once.
		if _, uerr := db.sql.ExecContext(ctx,
			`UPDATE login_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL`,
			stamp.Format(time.RFC3339), tokenHash); uerr != nil {
			return fmt.Errorf("store: consume admin token: %w", uerr)
		}
		return nil
	case errors.Is(qerr, sql.ErrNoRows):
		// The INSERT failed without storing anything: a real database error.
		return fmt.Errorf("store: consume admin token: %w", err)
	default:
		return fmt.Errorf("store: consume admin token: %w", qerr)
	}
}
