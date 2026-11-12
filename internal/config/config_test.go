package config

import (
	"testing"
)

func withEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func baseEnv() map[string]string {
	return map[string]string{
		"PORT":        "8080",
		"APP_ENV":     "dev",
		"BASE_URL":    "http://localhost:8080",
		"DB_PATH":     "./dev.db",
		"PHOTO_DIR":   "./dev-photos",
		"SESSION_KEY": "c2Vzc2lvbi1rZXktMzItYnl0ZXMtYWFiYmNjZGQ=",
		"IP_HASH_KEY": "aXAtaGFzaC1rZXktMzItYnl0ZXMtZWVmZmdoaGk=",
		"ADMIN_TOKEN": "YWRtaW4tdG9rZW4tMzItYnl0ZXMtamprbGxtbQ==",
		"TOWN_BBOX":   "42.70,-71.09,42.82,-70.96",
		"TZ":          "America/New_York",
	}
}

func TestLoadOK(t *testing.T) {
	withEnv(t, baseEnv())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppEnv != EnvDev {
		t.Errorf("AppEnv = %q", cfg.AppEnv)
	}
	if !cfg.TownBBox.Contains(42.76, -71.03) {
		t.Errorf("bbox should contain Groveland centre: %+v", cfg.TownBBox)
	}
	if cfg.TownBBox.Contains(42.9, -71.03) {
		t.Error("bbox should not contain points north of town")
	}
}

func TestLoadListsAllMissing(t *testing.T) {
	withEnv(t, baseEnv())
	t.Setenv("PORT", "")
	t.Setenv("DB_PATH", "")
	t.Setenv("TOWN_BBOX", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error listing missing vars")
	}
	for _, want := range []string{"PORT", "DB_PATH", "TOWN_BBOX"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	withEnv(t, baseEnv())
	t.Setenv("APP_ENV", "staging")
	if _, err := Load(); err == nil {
		t.Fatal("unknown APP_ENV accepted")
	}
}

func TestParseBBox(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		note string
	}{
		{"42.70,-71.09,42.82,-70.96", true, "valid"},
		{"42.70,-71.09", false, "too few parts"},
		{"a,b,c,d", false, "not numbers"},
		{"42.82,-70.96,42.70,-71.09", false, "min above max"},
	}
	for _, c := range cases {
		_, err := ParseBBox(c.in)
		if (err == nil) != c.ok {
			t.Errorf("ParseBBox(%q) err=%v, want ok=%v (%s)", c.in, err, c.ok, c.note)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
