# Acceptance drafts (inert until T-105)

The M1 assertions A-103 (`/probe` counter persisted in SQLite) and A-104
(migration checksum guard) describe features that do **not exist yet** in the
skeleton. Their tests are written **now**, as frozen contracts, and stored
here under extensions no runner picks up:

- `*.spec.ts.txt` — Playwright only loads `*.spec.ts` from `acceptance/e2e/`.
- `*.go.txt` — the Go toolchain only compiles `*.go`.

`scripts/holdout-run.sh` likewise runs only `<holdout>/go/*_test.go` and
`<holdout>/e2e/*.spec.ts` — nothing in subdirectories — so `drafts/` is inert
in the holdout tree too.

Because these files exist in git history **before** the feature tasks
(T-101, T-102, T-103) branch, they are the contract-first oracle: history
proves the tests were not written to match an implementation after the fact.

## Activation (T-105)

T-105 activates a draft with a pure `git mv` and **no content edits**:

```sh
git mv acceptance/drafts/probe.spec.ts.txt      acceptance/e2e/probe.spec.ts
git mv acceptance/drafts/probe_go_test.go.txt   acceptance/go/probe_test.go
git mv acceptance/drafts/migrations_go_test.go.txt acceptance/go/migrations_test.go
```

Exactly one exception exists: if `TestA104RefusesTamperedChecksum` fails
because the FIRST 64-char hex token found in the DB file is not the migration
checksum, T-105 may pin a different token **index** in that one test — see the
header of `migrations_go_test.go.txt`. That is the only sanctioned edit.

Every draft here would currently FAIL against the skeleton (no `/probe`
route: `GET /probe` answers 404; no migrations: `DB_PATH` is never created),
which is exactly why they are kept inert.

## Frozen contract strings (workers build to these, byte-exact)

| Where | String | Meaning |
| --- | --- | --- |
| `GET /` HTML | `<section id="probe">` | probe section, populated by HTMX from `/probe` on page load; server-rendered (with a plain form) when JS is off |
| probe fragment | heading, accessible name exactly `Probe` | section heading |
| probe fragment | `Count: N` (N ≥ 0) | counter readout; `N` also held by `#probe-count` |
| probe fragment | `#probe-count` | element whose text is exactly the digits of N |
| probe fragment | button, accessible name exactly `Increment counter` | submits POST `/probe` |
| probe fragment | `<input ... name="csrf_token" ...>` | hidden double-submit CSRF token in the fragment and the no-JS form |
| `POST /probe` | missing/tampered `csrf_token` → 403 | counter unchanged |
| `POST /probe` | valid token + `HX-Request: true` → 200 | fragment with `Count: N+1` |
| `POST /probe` | valid token, no `HX-Request` → 303 `Location: /` | no-JS form flow (increments, then redirect) |
| persistence | counter survives SIGTERM + respawn with the same `DB_PATH` | SQLite-backed |
| migrations | first migration file is `migrations/0001_init.sql` | applied at startup, checksum recorded in `schema_migrations` as 64-char lowercase hex sha256 |
| tamper | startup with a mutated checksum refuses to boot | exits non-zero; message contains `0001_init.sql` and `checksum` |

## Visual notes (A-103, what a correct screenshot shows)

Desktop and mobile: below the home-page nav, a bordered `section#probe`
containing the heading "Probe", the line "Count: N" (with N rendered inside
`#probe-count`, right-aligned digits), and a full-width "Increment counter"
button. After a click the digit ticks up in place — no full-page reload, no
URL change, no history entry. With JS disabled the same section renders
server-side with a real form; submitting it lands back on `/` with the digit
already incremented. Nothing overflows the 320px iPhone SE viewport.
