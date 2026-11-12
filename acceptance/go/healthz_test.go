//go:build acceptance

// Black-box acceptance tests: the server is built and started by the Makefile
// target (test-acceptance-go); BASE_URL points at it.
package acceptance

import (
	"io"
	"net/http"
	"os"
	"testing"
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func TestHealthz(t *testing.T) {
	base := envOr("BASE_URL", "http://localhost:8099")
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options")
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Error("missing Content-Security-Policy")
	}
}

func TestHomeLoads(t *testing.T) {
	base := envOr("BASE_URL", "http://localhost:8099")
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
