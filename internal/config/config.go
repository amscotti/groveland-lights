// Package config parses the environment into a typed Config, failing fast with
// every missing variable listed at once (plan §8.2).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Env string

const (
	EnvDev  Env = "dev"
	EnvTest Env = "test"
	EnvProd Env = "prod"
)

// BBox is a lat/lon bounding box: MinLat, MinLon, MaxLat, MaxLon.
type BBox struct {
	MinLat, MinLon, MaxLat, MaxLon float64
}

func (b BBox) Contains(lat, lon float64) bool {
	return lat >= b.MinLat && lat <= b.MaxLat && lon >= b.MinLon && lon <= b.MaxLon
}

type Config struct {
	Port       string
	AppEnv     Env
	BaseURL    string
	DBPath     string
	PhotoDir   string
	SessionKey []byte
	IPHashKey  []byte
	AdminToken []byte
	TownBBox   BBox
	Timezone   string
}

// ParseBBox parses "42.70,-71.09,42.82,-70.96" (minLat,minLon,maxLat,maxLon).
func ParseBBox(s string) (BBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return BBox{}, fmt.Errorf("bbox: want 4 comma-separated numbers, got %d parts", len(parts))
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return BBox{}, fmt.Errorf("bbox part %d: %w", i, err)
		}
		v[i] = f
	}
	if v[0] >= v[2] || v[1] >= v[3] {
		return BBox{}, errors.New("bbox: min must be below max")
	}
	return BBox{MinLat: v[0], MinLon: v[1], MaxLat: v[2], MaxLon: v[3]}, nil
}

// Load builds a Config from the environment. All missing variables are reported
// together. Secrets accept raw bytes or base64.
func Load() (Config, error) {
	var missing []string
	get := func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	cfg := Config{
		Port:     get("PORT"),
		BaseURL:  get("BASE_URL"),
		DBPath:   get("DB_PATH"),
		PhotoDir: get("PHOTO_DIR"),
	}
	env := os.Getenv("APP_ENV")
	switch Env(env) {
	case EnvDev, EnvTest, EnvProd:
		cfg.AppEnv = Env(env)
	case "":
		missing = append(missing, "APP_ENV")
	default:
		return cfg, fmt.Errorf("APP_ENV: unknown value %q", env)
	}
	var err error
	cfg.SessionKey, err = secret("SESSION_KEY")
	if err != nil {
		return cfg, err
	}
	cfg.IPHashKey, err = secret("IP_HASH_KEY")
	if err != nil {
		return cfg, err
	}
	cfg.AdminToken, err = secret("ADMIN_TOKEN")
	if err != nil {
		return cfg, err
	}
	bboxStr := get("TOWN_BBOX")
	cfg.TownBBox, err = ParseBBox(bboxStr)
	if err != nil && len(missing) == 0 {
		return cfg, err
	}
	cfg.Timezone = os.Getenv("TZ")
	if cfg.Timezone == "" {
		cfg.Timezone = "America/New_York"
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func secret(name string) ([]byte, error) {
	v := os.Getenv(name)
	if v == "" {
		return nil, nil // reported by get() when required
	}
	if b, err := base64.StdEncoding.DecodeString(v); err == nil && len(b) >= 16 {
		return b, nil
	}
	return []byte(v), nil
}

func (c Config) IsProd() bool { return c.AppEnv == EnvProd }
func (c Config) IsTest() bool { return c.AppEnv == EnvTest }
