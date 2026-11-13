CREATE TABLE probe (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  n  INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  handle TEXT NOT NULL UNIQUE,
  email TEXT UNIQUE,
  role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
  points INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE photos (
  id INTEGER PRIMARY KEY,
  owner_id INTEGER REFERENCES users(id),
  path TEXT NOT NULL UNIQUE,
  width INTEGER NOT NULL,
  height INTEGER NOT NULL,
  bytes INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE TABLE lights (
  id INTEGER PRIMARY KEY,
  asset_no TEXT,
  kind TEXT NOT NULL CHECK (kind IN ('park','building','street')),
  label TEXT NOT NULL,
  lat REAL NOT NULL,
  lon REAL NOT NULL,
  accuracy_m REAL,
  photo_id INTEGER REFERENCES photos(id),
  status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('on','off','degraded','unknown')),
  status_source TEXT NOT NULL DEFAULT 'none' CHECK (status_source IN ('none','single','confirmed','admin')),
  status_at TEXT,
  status_confirmed_at TEXT,
  gml_pole_no TEXT,
  created_by INTEGER REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  deleted_at TEXT
);
CREATE INDEX lights_geo ON lights(lat, lon) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX lights_asset_no ON lights(asset_no) WHERE asset_no IS NOT NULL AND deleted_at IS NULL;
CREATE TABLE reports (
  id INTEGER PRIMARY KEY,
  light_id INTEGER NOT NULL REFERENCES lights(id),
  user_id INTEGER NOT NULL REFERENCES users(id),
  session_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('on','off','degraded')),
  note TEXT CHECK (length(note) <= 500),
  photo_id INTEGER REFERENCES photos(id),
  lat REAL, lon REAL, accuracy_m REAL,
  ip_hmac TEXT NOT NULL,
  created_at TEXT NOT NULL,
  rounded_at TEXT
);
CREATE INDEX reports_light_time ON reports(light_id, created_at DESC);
CREATE INDEX reports_user_time ON reports(user_id, created_at DESC);
CREATE TABLE comments (
  id INTEGER PRIMARY KEY,
  light_id INTEGER NOT NULL REFERENCES lights(id),
  user_id INTEGER NOT NULL REFERENCES users(id),
  body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
  hidden INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);
CREATE TABLE comment_flags (
  comment_id INTEGER NOT NULL REFERENCES comments(id),
  session_id TEXT NOT NULL,
  reason TEXT,
  created_at TEXT NOT NULL,
  PRIMARY KEY (comment_id, session_id)
);
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  role TEXT NOT NULL CHECK (role IN ('anon','user','admin')),
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL
);
CREATE TABLE login_tokens (
  token_hash TEXT PRIMARY KEY,
  email TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  used_at TEXT
);
