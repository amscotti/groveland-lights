package store

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

// clockBase is the fixed instant the injected-clock tests walk around.
var clockBase = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

// setClock swaps the auth slice's clock for a fixed one and restores the
// real clock on cleanup, so no test can leave a frozen clock behind.
func setClock(t *testing.T, at time.Time) {
	t.Helper()
	prev := Now
	Now = func() time.Time { return at }
	t.Cleanup(func() { Now = prev })
}

// TestCreateSessionRoundTrip covers the session lifecycle as a table of
// flows; case names are the assertions.
func TestCreateSessionRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"created session validates with its user and role", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			id, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			gotUID, gotRole, ok, err := db.SessionValid(ctx, id)
			if err != nil {
				t.Fatalf("SessionValid: %v", err)
			}
			if !ok {
				t.Fatal("SessionValid = false for a freshly created session")
			}
			if gotUID != uid {
				t.Fatalf("SessionValid user = %d, want %d", gotUID, uid)
			}
			if gotRole != "admin" {
				t.Fatalf("SessionValid role = %q, want admin", gotRole)
			}
		}},
		{"session id is 32 random bytes base64url and unique per create", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			a, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			b, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession #2: %v", err)
			}
			if a == b {
				t.Fatal("two creates returned the same session id")
			}
			raw, err := base64.RawURLEncoding.DecodeString(a)
			if err != nil {
				t.Fatalf("session id not base64url: %v", err)
			}
			if len(raw) != sessionIDBytes {
				t.Fatalf("session id carries %d bytes, want %d", len(raw), sessionIDBytes)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestCreateSessionTimestamps pins the stored stamps against the injected
// clock: created_at now, expires_at now+24h (the frozen session lifetime).
func TestCreateSessionTimestamps(t *testing.T) {
	db, _ := openFresh(t)
	setClock(t, clockBase)
	ctx := context.Background()
	uid, err := db.EnsureAdminUser(ctx)
	if err != nil {
		t.Fatalf("EnsureAdminUser: %v", err)
	}
	id, err := db.CreateSession(ctx, uid, "admin")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	var createdAt, expiresAt string
	if err := db.sql.QueryRowContext(ctx,
		`SELECT created_at, expires_at FROM sessions WHERE id = ?`, id).
		Scan(&createdAt, &expiresAt); err != nil {
		t.Fatalf("select session row: %v", err)
	}
	if createdAt != clockBase.Format(time.RFC3339) {
		t.Fatalf("created_at = %q, want %q", createdAt, clockBase.Format(time.RFC3339))
	}
	if expiresAt != clockBase.Add(sessionTTL).Format(time.RFC3339) {
		t.Fatalf("expires_at = %q, want %q", expiresAt, clockBase.Add(sessionTTL).Format(time.RFC3339))
	}
}

// TestSessionRejectionPaths covers the not-valid paths and the delete
// lifecycle; case names are the assertions.
func TestSessionRejectionPaths(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"unknown id is not valid and carries no error", func(t *testing.T) {
			db, _ := openFresh(t)
			_, _, ok, err := db.SessionValid(context.Background(), "no-such-session")
			if err != nil {
				t.Fatalf("SessionValid: %v", err)
			}
			if ok {
				t.Fatal("SessionValid = true for an unknown id")
			}
		}},
		{"empty id is not valid and carries no error", func(t *testing.T) {
			db, _ := openFresh(t)
			_, _, ok, err := db.SessionValid(context.Background(), "")
			if err != nil {
				t.Fatalf("SessionValid: %v", err)
			}
			if ok {
				t.Fatal("SessionValid = true for an empty id")
			}
		}},
		{"delete removes the session and is idempotent", func(t *testing.T) {
			db, _ := openFresh(t)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			id, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			if err := db.DeleteSession(ctx, id); err != nil {
				t.Fatalf("DeleteSession: %v", err)
			}
			if _, _, ok, _ := db.SessionValid(ctx, id); ok {
				t.Fatal("SessionValid = true after DeleteSession")
			}
			if err := db.DeleteSession(ctx, id); err != nil {
				t.Fatalf("DeleteSession (again): %v", err)
			}
		}},
		{"foreign key refuses a session for an unknown user", func(t *testing.T) {
			db, _ := openFresh(t)
			_, err := db.CreateSession(context.Background(), UserID(9999), "admin")
			if err == nil {
				t.Fatal("CreateSession for unknown user succeeded, want an FK error")
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}

// TestSessionExpiryBoundary walks the frozen 24h expiry with the injected
// clock: valid up to expires_at-1s, rejected from expires_at onwards.
func TestSessionExpiryBoundary(t *testing.T) {
	tests := []struct {
		name    string
		offset  time.Duration
		wantOK  bool
		wantErr error // non-nil when the rejection must carry this sentinel
	}{
		{"valid just before expiry", sessionTTL - time.Second, true, nil},
		{"invalid exactly at expiry", sessionTTL, false, ErrBadCredentials},
		{"invalid after expiry", sessionTTL + time.Hour, false, ErrBadCredentials},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, _ := openFresh(t)
			setClock(t, clockBase)
			ctx := context.Background()
			uid, err := db.EnsureAdminUser(ctx)
			if err != nil {
				t.Fatalf("EnsureAdminUser: %v", err)
			}
			id, err := db.CreateSession(ctx, uid, "admin")
			if err != nil {
				t.Fatalf("CreateSession: %v", err)
			}
			setClock(t, clockBase.Add(tc.offset))
			_, _, ok, err := db.SessionValid(ctx, id)
			if ok != tc.wantOK {
				t.Fatalf("SessionValid ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("SessionValid err = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("SessionValid err = %v, want it to wrap %v", err, tc.wantErr)
			}
		})
	}
}
