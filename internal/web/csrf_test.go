package web

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/amscotti/groveland-lights/internal/config"
)

// csrfIssue runs IssueCSRFToken for a GET bound to s and returns the recorder
// plus the issued token.
func csrfIssue(t *testing.T, s *Server) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec, token := issueOn(t, s, nil)
	return rec, token
}

// issueOn runs IssueCSRFToken for a request bound to s; cookies are added to
// the request before the call.
func issueOn(t *testing.T, s *Server, cookies []*http.Cookie) (*httptest.ResponseRecorder, string) {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	r = r.WithContext(context.WithValue(r.Context(), ctxServer, s))
	rec := httptest.NewRecorder()
	token := IssueCSRFToken(rec, r)
	return rec, token
}

// csrfCookieOf extracts the csrf_token cookie the handler set, failing the
// test when it is absent.
func csrfCookieOf(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()
	for _, c := range resp.Cookies() {
		if c.Name == CSRFTokenName {
			return c
		}
	}
	t.Fatalf("response set no %s cookie", CSRFTokenName)
	return nil // unreachable
}

// csrfVerify builds a POST carrying a csrf_token cookie of value cookie and
// runs VerifyCSRF against submitted; cookie "" means no cookie at all.
func csrfVerify(cookie, submitted string) error {
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/probe", nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: CSRFTokenName, Value: cookie})
	}
	return VerifyCSRF(r, submitted)
}

// tamperCSRF flips the first character, preserving base64url shape and
// length (mirrors the acceptance draft's tampering).
func tamperCSRF(tok string) string {
	b := []byte(tok)
	if b[0] == 'A' {
		b[0] = 'B'
	} else {
		b[0] = 'A'
	}
	return string(b)
}

// TestCSRFVerify covers VerifyCSRF: case names are the assertions.
func TestCSRFVerify(t *testing.T) {
	s := newTestServer(t, config.EnvTest)

	t.Run("valid round-trip accepted", func(t *testing.T) {
		_, token := csrfIssue(t, s)
		if token == "" {
			t.Fatal("IssueCSRFToken returned an empty token")
		}
		if err := csrfVerify(token, token); err != nil {
			t.Fatalf("VerifyCSRF(issued, issued) = %v, want nil", err)
		}
	})

	t.Run("missing cookie rejected", func(t *testing.T) {
		_, token := csrfIssue(t, s)
		if err := csrfVerify("", token); !errors.Is(err, ErrCSRF) {
			t.Fatalf("VerifyCSRF(no cookie, token) = %v, want ErrCSRF", err)
		}
	})

	t.Run("empty submission rejected", func(t *testing.T) {
		_, token := csrfIssue(t, s)
		if err := csrfVerify(token, ""); !errors.Is(err, ErrCSRF) {
			t.Fatalf("VerifyCSRF(cookie, empty) = %v, want ErrCSRF", err)
		}
	})

	t.Run("tampered value rejected", func(t *testing.T) {
		_, token := csrfIssue(t, s)
		tampered := tamperCSRF(token)
		if tampered == token {
			t.Fatal("tamper produced the same token")
		}
		if len(tampered) != len(token) {
			t.Fatalf("tamper changed length: %d vs %d", len(tampered), len(token))
		}
		if err := csrfVerify(token, tampered); !errors.Is(err, ErrCSRF) {
			t.Fatalf("VerifyCSRF(cookie, tampered) = %v, want ErrCSRF", err)
		}
	})

	t.Run("garbage submission rejected", func(t *testing.T) {
		_, token := csrfIssue(t, s)
		for _, bad := range []string{
			"not a token",                 // not base64url (spaces)
			"short",                       // well under the entropy floor
			"###",                         // characters outside the alphabet
			strings.Repeat("A", 43) + "=", // padded: not the raw encoding we mint
			strings.Repeat("A", 300),      // over the length bound
		} {
			if err := csrfVerify(token, bad); !errors.Is(err, ErrCSRF) {
				t.Fatalf("VerifyCSRF(cookie, %q) = %v, want ErrCSRF", bad, err)
			}
		}
	})

	t.Run("malformed cookie rejected", func(t *testing.T) {
		// Well-formedness holds on both sides: a cookie that decodes but
		// carries too little entropy must not pass, even when the
		// submission matches it byte for byte.
		bad := strings.Repeat("A", 10)
		if err := csrfVerify(bad, bad); !errors.Is(err, ErrCSRF) {
			t.Fatalf("VerifyCSRF(short cookie, same) = %v, want ErrCSRF", err)
		}
	})
}

// TestCSRFIssue covers IssueCSRFToken: reuse, entropy, malformed-cookie
// reissue. Case names are the assertions.
func TestCSRFIssue(t *testing.T) {
	s := newTestServer(t, config.EnvTest)

	t.Run("issue is idempotent per cookie", func(t *testing.T) {
		rec1, first := csrfIssue(t, s)
		if first == "" {
			t.Fatal("first issue returned an empty token")
		}
		if c := csrfCookieOf(t, rec1); c.Value != first {
			t.Fatalf("cookie value %q != issued token %q", c.Value, first)
		}
		// A second request carrying the first cookie must reuse the value,
		// not mint a fresh one, and needs no redundant Set-Cookie.
		rec2, second := issueOn(t, s, []*http.Cookie{{Name: CSRFTokenName, Value: first}})
		if second != first {
			t.Fatalf("second issue = %q, want reused %q", second, first)
		}
		if n := len(rec2.Result().Cookies()); n != 0 {
			t.Errorf("reuse path set %d cookie(s), want 0", n)
		}
	})

	t.Run("issued token meets entropy floor", func(t *testing.T) {
		_, a := csrfIssue(t, s)
		_, b := csrfIssue(t, s)
		raw, err := base64.RawURLEncoding.DecodeString(a)
		if err != nil {
			t.Fatalf("issued token not base64url: %v", err)
		}
		if len(raw) < csrfTokenBytes {
			t.Fatalf("issued token carries %d bytes, want >= %d", len(raw), csrfTokenBytes)
		}
		if a == b {
			t.Fatal("two issues produced identical tokens")
		}
	})

	t.Run("malformed cookie is reissued", func(t *testing.T) {
		bad := "garbage-not-base64!!"
		rec, got := issueOn(t, s, []*http.Cookie{{Name: CSRFTokenName, Value: bad}})
		if got == bad {
			t.Fatal("malformed cookie value reused as the token")
		}
		if !validCSRFToken(got) {
			t.Fatalf("reissued token %q is not well-formed", got)
		}
		if c := csrfCookieOf(t, rec); c.Value != got {
			t.Fatalf("Set-Cookie value %q != reissued token %q", c.Value, got)
		}
	})
}

// TestCSRFCookieFlags: Path=/, HttpOnly and SameSite=Lax always; Secure
// exactly in prod.
func TestCSRFCookieFlags(t *testing.T) {
	tests := []struct {
		name   string
		env    config.Env
		secure bool
	}{
		{"prod cookie is Secure", config.EnvProd, true},
		{"test cookie is not Secure", config.EnvTest, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t, tc.env)
			rec, token := csrfIssue(t, s)
			if token == "" {
				t.Fatal("empty token")
			}
			c := csrfCookieOf(t, rec)
			if c.Secure != tc.secure {
				t.Errorf("Secure = %v, want %v (env %s)", c.Secure, tc.secure, tc.env)
			}
			if !c.HttpOnly {
				t.Error("HttpOnly = false, want true")
			}
			if c.Path != "/" {
				t.Errorf("Path = %q, want /", c.Path)
			}
			if c.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, want Lax", c.SameSite)
			}
		})
	}
}
