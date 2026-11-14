package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/amscotti/groveland-lights/internal/store/gen"
)

// LightID identifies one row of the lights table (typed so a report's light
// reference can never be confused with a photo or user id at compile time).
type LightID int64

// PhotoID identifies one row of the photos table.
type PhotoID int64

// Kind is the fixture class of a light, pinned by the schema CHECK to the
// values below (street is reserved for the later street-light import).
type Kind string

// The three schema-valid kinds.
const (
	KindPark     Kind = "park"
	KindBuilding Kind = "building"
	KindStreet   Kind = "street"
)

// errInvalidKind is wrapped when ParseKind rejects a value.
var errInvalidKind = errors.New("store: invalid kind")

// ParseKind validates s as one of the schema's kind values and returns it as
// the typed Kind. Callers parse untrusted input at the boundary with this
// function; only the three exact lowercase values are accepted.
func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case KindPark, KindBuilding, KindStreet:
		return Kind(s), nil
	default:
		return "", fmt.Errorf("%w: %q (want park, building or street)", errInvalidKind, s)
	}
}

// ErrDuplicateAssetNo is wrapped (detectable via errors.Is) when a create or
// edit stores an asset number that another live (non-deleted) light already
// holds: the partial unique index lights_asset_no refused the write.
var ErrDuplicateAssetNo = errors.New("store: duplicate asset_no")

// NewLight is the parsed create-light input. Pointer fields are optional
// columns: nil stores NULL (asset_no, accuracy_m, photo_id, gml_pole_no,
// created_by). created_at/updated_at are stamped by the store.
type NewLight struct {
	AssetNo   *string
	Kind      Kind
	Label     string
	Lat       float64
	Lon       float64
	AccuracyM *float64
	PhotoID   *PhotoID
	GmlPoleNo *string
	CreatedBy *int64
}

// Light is one fixture row as read back from the database. Timestamps are
// parsed from the RFC3339 TEXT columns into UTC times; NULL columns become
// nil pointers. Status and its provenance fields are derived through
// status.Derive only (other tasks write them); the store reads them verbatim.
type Light struct {
	ID                LightID
	AssetNo           *string
	Kind              Kind
	Label             string
	Lat               float64
	Lon               float64
	AccuracyM         *float64
	PhotoID           *PhotoID
	Status            string
	StatusSource      string
	StatusAt          *time.Time
	StatusConfirmedAt *time.Time
	GmlPoleNo         *string
	CreatedBy         *int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	DeletedAt         *time.Time
}

// rfc3339 parses one timestamp column, naming the column in the error so a
// malformed stored value is diagnosable.
func rfc3339(column, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("store: %s %q: %w", column, value, err)
	}
	return t, nil
}

// rfc3339Null parses a nullable timestamp column: NULL becomes nil.
func rfc3339Null(column string, v sql.NullString) (*time.Time, error) {
	if !v.Valid {
		return nil, nil
	}
	t, err := rfc3339(column, v.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// lightFromGen converts a generated row into the domain type.
func lightFromGen(g gen.Light) (Light, error) {
	l := Light{
		ID:           LightID(g.ID),
		AssetNo:      fromNullString(g.AssetNo),
		Kind:         Kind(g.Kind),
		Label:        g.Label,
		Lat:          g.Lat,
		Lon:          g.Lon,
		AccuracyM:    fromNullFloat(g.AccuracyM),
		PhotoID:      photoIDFromNull(g.PhotoID),
		Status:       g.Status,
		StatusSource: g.StatusSource,
		GmlPoleNo:    fromNullString(g.GmlPoleNo),
		CreatedBy:    fromNullInt(g.CreatedBy),
	}
	var err error
	if l.CreatedAt, err = rfc3339("created_at", g.CreatedAt); err != nil {
		return Light{}, err
	}
	if l.UpdatedAt, err = rfc3339("updated_at", g.UpdatedAt); err != nil {
		return Light{}, err
	}
	if l.StatusAt, err = rfc3339Null("status_at", g.StatusAt); err != nil {
		return Light{}, err
	}
	if l.StatusConfirmedAt, err = rfc3339Null("status_confirmed_at", g.StatusConfirmedAt); err != nil {
		return Light{}, err
	}
	if l.DeletedAt, err = rfc3339Null("deleted_at", g.DeletedAt); err != nil {
		return Light{}, err
	}
	return l, nil
}

// isDuplicateAssetNo reports whether err is SQLite's unique violation on the
// partial unique index lights_asset_no. The driver surfaces the constraint
// failure as a plain error whose message carries SQLite's stable
// "UNIQUE constraint failed" text plus the offending index/column name; both
// spellings (index and column form) are accepted so a driver upgrade that
// switches message format cannot silently turn duplicates into 500s.
func isDuplicateAssetNo(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") {
		return false
	}
	return strings.Contains(msg, "lights_asset_no") || strings.Contains(msg, "lights.asset_no")
}

// CreateLight inserts one light row and returns it as read back (including
// the server-side status defaults). created_at and updated_at are stamped
// now (UTC, RFC3339). Storing an asset number that another live light
// already holds wraps ErrDuplicateAssetNo.
func (db *DB) CreateLight(ctx context.Context, n NewLight) (Light, error) {
	if _, err := ParseKind(string(n.Kind)); err != nil {
		return Light{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	row, err := db.q.CreateLight(ctx, gen.CreateLightParams{
		AssetNo:   toNullString(n.AssetNo),
		Kind:      string(n.Kind),
		Label:     n.Label,
		Lat:       n.Lat,
		Lon:       n.Lon,
		AccuracyM: toNullFloat(n.AccuracyM),
		PhotoID:   toNullPhotoID(n.PhotoID),
		GmlPoleNo: toNullString(n.GmlPoleNo),
		CreatedBy: toNullInt(n.CreatedBy),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		if isDuplicateAssetNo(err) {
			return Light{}, fmt.Errorf("store: create light: %w: %w", ErrDuplicateAssetNo, err)
		}
		return Light{}, fmt.Errorf("store: create light: %w", err)
	}
	return lightFromGen(row)
}

// GetLight returns one non-deleted light. A missing or soft-deleted id wraps
// ErrNotFound.
func (db *DB) GetLight(ctx context.Context, id LightID) (Light, error) {
	row, err := db.q.GetLight(ctx, int64(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Light{}, ErrNotFound
		}
		return Light{}, fmt.Errorf("store: get light %d: %w", id, err)
	}
	return lightFromGen(row)
}

// LightByAssetNo returns the live light carrying assetNo. A deleted light's
// asset number is not found (the partial unique index frees it for reuse);
// an unknown asset number wraps ErrNotFound.
func (db *DB) LightByAssetNo(ctx context.Context, assetNo string) (Light, error) {
	row, err := db.q.GetLightByAssetNo(ctx, sql.NullString{String: assetNo, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Light{}, ErrNotFound
		}
		return Light{}, fmt.Errorf("store: light by asset_no %q: %w", assetNo, err)
	}
	return lightFromGen(row)
}

// RecentLights returns up to limit lights, newest first (created_at DESC with
// the id DESC tie-break the admin list contract freezes), excluding deleted
// rows. offset pages through the set.
func (db *DB) RecentLights(ctx context.Context, limit, offset int) ([]Light, error) {
	if limit < 0 {
		return nil, fmt.Errorf("store: recent lights: limit %d must be >= 0", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("store: recent lights: offset %d must be >= 0", offset)
	}
	rows, err := db.q.ListRecentLights(ctx, gen.ListRecentLightsParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("store: recent lights: %w", err)
	}
	out := make([]Light, 0, len(rows))
	for _, g := range rows {
		l, err := lightFromGen(g)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// EditLight updates a live light's label and asset number, stamps updated_at
// and returns the row as read back. A missing or soft-deleted id wraps
// ErrNotFound; reusing another live light's asset number wraps
// ErrDuplicateAssetNo.
func (db *DB) EditLight(ctx context.Context, id LightID, label string, assetNo *string, kind Kind) (Light, error) {
	row, err := db.q.UpdateLight(ctx, gen.UpdateLightParams{
		Label:     label,
		AssetNo:   toNullString(assetNo),
		Kind:      string(kind),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		ID:        int64(id),
	})
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return Light{}, ErrNotFound
		case isDuplicateAssetNo(err):
			return Light{}, fmt.Errorf("store: edit light %d: %w: %w", id, ErrDuplicateAssetNo, err)
		default:
			return Light{}, fmt.Errorf("store: edit light %d: %w", id, err)
		}
	}
	return lightFromGen(row)
}

// SoftDeleteLight stamps deleted_at on a live light, making it invisible to
// every read and edit and freeing its asset number. A missing id, a
// soft-deleted id (the WHERE deleted_at IS NULL guard makes the delete
// idempotent-refusing) wraps ErrNotFound.
func (db *DB) SoftDeleteLight(ctx context.Context, id LightID) error {
	_, err := db.q.SoftDeleteLight(ctx, gen.SoftDeleteLightParams{
		DeletedAt: sql.NullString{String: time.Now().UTC().Format(time.RFC3339), Valid: true},
		ID:        int64(id),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("store: soft delete light %d: %w", id, err)
	}
	return nil
}

// NearbyLights returns every live light inside a bounding box that safely
// contains the radiusM disk around (lat, lon), as a prefilter for the exact
// Haversine distance that internal/geo computes in Go. The box over-covers
// the radius (safety factor below), so a row inside the exact radius can
// never be clipped: callers filter candidates by exact distance. A negative
// radius is refused.
func (db *DB) NearbyLights(ctx context.Context, lat, lon, radiusM float64) ([]Light, error) {
	if radiusM < 0 {
		return nil, fmt.Errorf("store: nearby lights: radius %v must be >= 0", radiusM)
	}
	latDelta, lonDelta := bboxDegrees(lat, radiusM)
	rows, err := db.q.NearbyLights(ctx, gen.NearbyLightsParams{
		MinLat: lat - latDelta,
		MaxLat: lat + latDelta,
		MinLon: lon - lonDelta,
		MaxLon: lon + lonDelta,
	})
	if err != nil {
		return nil, fmt.Errorf("store: nearby lights: %w", err)
	}
	out := make([]Light, 0, len(rows))
	for _, g := range rows {
		l, err := lightFromGen(g)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

// bboxDegrees converts a ground radius in metres into latitude/longitude
// degree deltas at the given latitude. Values are degree-level accurate (the
// mean-Earth figure below) with a safety factor that over-covers the radius,
// matching NearbyLights's superset contract. Towards the poles the longitude
// span grows without bound; there the box caps at a hemisphere, which still
// trivially contains any realistic radius.
const (
	metersPerDegree = 111194.9 // mean-Earth metres per degree of latitude
	nearbySafety    = 1.10     // bbox over-cover factor (>= 1)
)

func bboxDegrees(lat, radiusM float64) (latDelta, lonDelta float64) {
	latDelta = nearbySafety * radiusM / metersPerDegree
	cosLat := math.Abs(math.Cos(lat * math.Pi / 180))
	if cosLat == 0 {
		lonDelta = 180
		return latDelta, lonDelta
	}
	lonDelta = nearbySafety * radiusM / (metersPerDegree * cosLat)
	if lonDelta > 180 {
		lonDelta = 180
	}
	return latDelta, lonDelta
}

// --- nullable-column conversions shared by the typed wrappers ---

// toNullString maps an optional string column value to the driver type.
func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

// fromNullString maps a nullable string column to the optional domain type.
func fromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

// toNullFloat maps an optional float column value to the driver type.
func toNullFloat(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

// fromNullFloat maps a nullable float column to the optional domain type.
func fromNullFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	f := nf.Float64
	return &f
}

// toNullInt maps an optional int column value to the driver type.
func toNullInt(i *int64) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *i, Valid: true}
}

// fromNullInt maps a nullable int column to the optional domain type.
func fromNullInt(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	i := ni.Int64
	return &i
}

// toNullPhotoID maps an optional photo reference to the driver type.
func toNullPhotoID(p *PhotoID) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// photoIDFromNull maps a nullable photo reference to the typed domain id.
func photoIDFromNull(ni sql.NullInt64) *PhotoID {
	if !ni.Valid {
		return nil
	}
	p := PhotoID(ni.Int64)
	return &p
}
