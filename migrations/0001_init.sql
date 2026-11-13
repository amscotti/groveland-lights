-- Persistent probe counter: proves SQLite writes survive restarts/deploys.
CREATE TABLE probe (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  n  INTEGER NOT NULL DEFAULT 0
);
INSERT INTO probe (id, n) VALUES (1, 0);
