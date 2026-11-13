package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

// TestEnsureAdminUser covers the idempotent admin-account creation; case
// names are the assertions.
func TestEnsureAdminUser(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"first call creates the admin row", func(t *testing.T) {
			db, _ := openFresh(t)
			id, err := db.EnsureAdminUser(context.Background())
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			if id <= 0 {
				t.Fatalf("EnsureAdminUser id = %d, want > 0", id)
			}
			var handle, email, role string
			if err := db.sql.QueryRowContext(context.Background(),
				`SELECT handle, email, role FROM users WHERE id = ?`, int64(id)).
				Scan(&handle, &email, &role); err != nil {
				t.Fatalf("select admin row: %v", err)
			}
			if handle != adminHandle || email != adminEmail || role != "admin" {
				t.Fatalf("admin row = (%q, %q, %q), want (%q, %q, admin)",
					handle, email, role, adminHandle, adminEmail)
			}
		}},
		{"second call is idempotent and returns the same id", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			first, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser #1: %v", err)
			}
			second, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser #2: %v", err)
			}
			if first != second {
				t.Fatalf("EnsureAdminUser ids differ: %d then %d", first, second)
			}
			var n int
			if err := db.sql.QueryRowContext(ctx,
				`SELECT count(*) FROM users WHERE handle = ?`, adminHandle).Scan(&n); err != nil {
				t.Fatalf("count users: %v", err)
			}
			if n != 1 {
				t.Fatalf("users with handle %q = %d, want exactly 1", adminHandle, n)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestConsumeAdminToken covers the one-time token exchange; case names are
// the assertions.
func TestConsumeAdminToken(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"first use succeeds", func(t *testing.T) {
			db, _ := openFresh(t)
			if err := db.ConsumeAdminToken(context.Background(), []byte("the-admin-token")); err != nil {
				t.Fatalf("ConsumeAdminToken: %v", err)
			}
		}},
		{"second use wraps ErrTokenReused", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			token := []byte("the-admin-token")
			if err := db.ConsumeAdminToken(ctx, token); err != nil {
				t.Fatalf("first ConsumeAdminToken: %v", err)
			}
			err := db.ConsumeAdminToken(ctx, token)
			if !errors.Is(err, ErrTokenReused) {
				t.Fatalf("second ConsumeAdminToken err = %v, want it to wrap ErrTokenReused", err)
			}
		}},
		{"stored row records email, 24h expiry and the use stamp", func(t *testing.T) {
			db, _ := openFresh(t)
			setClock(t, clockBase)
			token := []byte("the-admin-token")
			if err := db.ConsumeAdminToken(context.Background(), token); err != nil {
				t.Fatalf("ConsumeAdminToken: %v", err)
			}
			var email, expiresAt, usedAt string
			if err := db.sql.QueryRowContext(context.Background(),
				`SELECT email, expires_at, used_at FROM login_tokens WHERE token_hash = ?`,
				tokenHashOf(token)).Scan(&email, &expiresAt, &usedAt); err != nil {
				t.Fatalf("select login_tokens row: %v", err)
			}
			if email != adminEmail {
				t.Fatalf("email = %q, want %q", email, adminEmail)
			}
			if expiresAt != clockBase.Add(loginTokenTTL).Format(time.RFC3339) {
				t.Fatalf("expires_at = %q, want %q", expiresAt, clockBase.Add(loginTokenTTL).Format(time.RFC3339))
			}
			if usedAt != clockBase.Format(time.RFC3339) {
				t.Fatalf("used_at = %q, want %q", usedAt, clockBase.Format(time.RFC3339))
			}
		}},
		{"a different token value is independent", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			if err := db.ConsumeAdminToken(ctx, []byte("token-a")); err != nil {
				t.Fatalf("ConsumeAdminToken A: %v", err)
			}
			if err := db.ConsumeAdminToken(ctx, []byte("token-b")); err != nil {
				t.Fatalf("ConsumeAdminToken B: %v", err)
			}
		}},
		{"database errors wrap the driver error", func(t *testing.T) {
			db, _ := openFresh(t)
			if err := db.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			err := db.ConsumeAdminToken(context.Background(), []byte("the-admin-token"))
			if err == nil {
				t.Fatal("ConsumeAdminToken on a closed DB succeeded, want an error")
			}
			if errors.Is(err, ErrTokenReused) {
				t.Fatalf("database error misreported as a replay: %v", err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// tokenHashOf mirrors ConsumeAdminToken's hash derivation for test queries.
func tokenHashOf(token []byte) string {
	sum := sha256.Sum256(token)
	return hex.EncodeToString(sum[:])
}
