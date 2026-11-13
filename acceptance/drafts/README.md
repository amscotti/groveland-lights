# M2 acceptance drafts (frozen contract)

Inert drafts for milestone M2 (admin inventory, "walk the town"), written by
T-200 BEFORE any feature task branched. The `.go.txt` / `.spec.ts.txt`
extensions are compiled/loaded by nothing, so the merge gate stays green while
the contract is already enforceable: git history proves these tests predate
the implementations. **T-207 (also test-author) activates them by pure
rename — no content edits. A gofmt pass on the activated Go files is the only
sanctioned mechanical step.** If an assertion turns out to be unsatisfiable
or ambiguous against the merged feature, T-207 writes `BLOCKED: <reason>` and
routes the dispute to the planner; the oracle is never edited to fit the
implementation (the T-105 lesson).

## Activation (T-207)

    git mv acceptance/drafts/admin_auth_go_test.go.txt    acceptance/go/admin_auth_test.go
    git mv acceptance/drafts/admin_lights_go_test.go.txt  acceptance/go/admin_lights_test.go
    git mv acceptance/drafts/admin-walk.spec.ts.txt       acceptance/e2e/admin-walk.spec.ts
    # then remove this directory (the frozen bytes stay auditable in git
    # history; preserve the table below in acceptance/README.md)

**T-207 must preserve the contract table below** (copy it into
`acceptance/README.md` verbatim): workers and reviewers resolve every UI
dispute against it.

## Contract reconciliations (read before building, T-205)

These drafts and the worker packets froze three details differently at
first draft; the FINAL frozen resolution (aligned with the worker packets
T-203/T-205, which the server will be built to) is:

1. **Create success redirect**: `POST /admin/lights` succeeds →
   role=status `Saved` fragment for HTMX requests, **303 `Location: /admin`
   without HX-Request** (T-205). The drafts discover a new light's detail
   URL by reading the `/admin` list rows, never from the Location header.
2. **Duplicate panel buttons**: the NUMBERED case (same asset_no) shows
   `Merge` + a cancel link — `Save anyway` appears only in the UNNUMBERED
   case, and `dup_choice=save` on a numbered duplicate answers 422 (T-205;
   the Go draft asserts the 422). The packet's test-6 wording ("with Merge
   and Save anyway") is satisfied by the unnumbered half of the e2e test.
3. **kind enum**: the create form offers `park|building` only (schema
   allows `street` too — reserved for the later street-light import; no M2
   test posts it).

## Frozen UI contract (workers build to this EXACTLY)

| Element | Contract |
|---|---|
| `/admin/login` | heading `Admin login`; label `Token`; button `Sign in` |
| `/admin/walk` `#walk-pos` | text matching `\d+\.\d+, -?\d+\.\d+` (rendered at 6 decimal places) |
| `#walk-accuracy` | text matching `/accuracy \d+ m/` |
| `#pin` (draggable pin) | `aria-label="position pin"`, keyboard-movable |
| `#pin-pos` | current pin coords, same format — rendered at exactly 6 decimal places, so its text equals `#light-pos` for the same position |
| buttons | `Add light here` (exact), `Save anyway (low accuracy)`, `Confirm pin`, `Merge`, `Save anyway` (duplicate panel: `Merge` always; `Save anyway` unnumbered-only, see reconciliation 2) |
| form labels | `Asset number`, `Label`, `Kind` (select: `Park`, `Building`), `Photo` |
| success | role=status containing `Saved` (HTMX fragment for create; edit/delete answer 303 redirects) |
| bbox rejection | role=alert containing `outside Groveland town bounds` |
| duplicate panel | `#dup-panel` with heading `Possible duplicate`; nothing is saved until a button is chosen |
| list `/admin` | heading `Lights`; row per light with thumbnail `img[alt=<label>]`; link or button `Older` |

Additional selectors frozen by the drafts (needed by specific tests):

| Element | Contract |
|---|---|
| `#manual-lat`, `#manual-lon` | number inputs, visible when geolocation is denied |
| `#photo-preview` | in-form preview `img`; renders EXIF orientation applied |
| `#light-pos` | admin detail coordinates as `lat, lon` at 6 decimal places |
| login markup | `<h1>Admin login</h1>`, `<label ..>Token</label>` + `<input name="token">`, `<button>Sign in</button>` |
| list row link | the label text links to `/admin/lights/{id}` (accessible name contains the label) |
| thumbnail alt format | `alt="<asset_no> <label>"` for numbered lights, `alt="<label>"` for unnumbered (this reconciles the table's `img[alt=<label>]` with the packet test 7's `img[alt^="GP-017"]`; asset numbers may not appear anywhere else twice) |
| walk form | `/admin/walk` server-renders the add-light form with a hidden `csrf_token` input (value attribute, order free); the form submits fine with a label alone (Kind defaults) |

## Frozen HTTP contract (Go drafts)

| Route | Contract |
|---|---|
| `GET /admin` (no session) | 302 → `/admin/login`; with `HX-Request: true` and no session: 401 + `HX-Redirect: /admin/login` |
| `GET /admin/login` | 200 form page (see markup above) |
| `GET /admin/login?token=X` | valid `ADMIN_TOKEN` → 302 → `/admin` + `Set-Cookie: gl_session=...` with `HttpOnly`, `SameSite=Lax`, `Path=/` (one-time, GLOBAL: replay from any client → 403); wrong/empty token → 403 |
| `POST /admin/lights` | multipart `csrf_token, label, kind(park\|building), lat, lon, asset_no? (^[A-Za-z0-9-]{1,32}$), photo?, accuracy_m?, dup_choice?("" \| merge \| save)`; success → role=status `Saved` for HX-Request, else 303 `Location: /admin`; duplicate → 409 body contains `Possible duplicate` (numbered+save → 422); outside `TOWN_BBOX` → 422 body contains `outside Groveland town bounds`; >15 MB → 413; wrong type / HEIC / >40 MP → 415 (HEIC body contains `please choose JPEG`) |
| `GET /admin/lights/{id}` | 200 detail; photo referenced as `src="/photos/..."`; 404 after soft delete |
| `POST /admin/lights/{id}` | edit (multipart `csrf_token` + changed fields; **unspecified fields keep values**) → 303 back to the detail |
| `POST /admin/lights/{id}/delete` | soft delete (multipart `csrf_token`) → 303 `/admin` |
| `GET /admin` | list: heading `Lights`, the 50 most recent lights, one `img alt` per row (alt format above; placeholder thumbnails for photo-less lights are fine) |
| `GET /admin/rows?page=N` | HTMX rows fragment (root element id `rows`); without `HX-Request` → 303 → `/admin?page=N` |
| `GET /photos/...` | served photo: `Content-Type: image/jpeg`, long edge ≤ 1600, NO APP1 segment (so no EXIF, no GPS), EXIF orientation applied (a 600×800 Orientation=6 upload serves wider-than-tall) |

Duplicate radii (T-204): same non-empty asset_no anywhere → numbered
duplicate; unnumbered within 8 m → unnumbered duplicate.

Ordering: the admin list is `ORDER BY created_at DESC, id DESC` (the id
tie-break is REQUIRED — rapid test creates share a timestamp second).

Session cookie name: `gl_session` (24 h). CSRF token field name:
`csrf_token` (double-submit pattern as in the probe contract).

## Rerun safety + the one-time token (frozen test behaviour)

The Playwright webServer shares ONE persistent `./test-run.db` across every
test, every project AND across the e2e → a11y invocations of `make check`;
`ADMIN_TOKEN` is one-time GLOBAL. The frozen drafts therefore:

- import `loginAsAdmin` (util/admin.ts), which exchanges the token AT MOST
  ONCE per test-run.db lifetime and reuses the 24 h session cookie (in-process
  memo + the gitignored `test-run.db.session.json`, which the `.gitignore`
  pattern `test-run.db*` covers so it lives and dies with its DB). The
  A-201 contract itself (exchange, replay 403, cookie flags) is asserted by
  the Go drafts against fresh per-test servers;
- give every created light an asset number unique per run and per project,
  keeping the packet prefixes (`GP-017-<run>-<proj>` — still `^[A-Za-z0-9-]{1,32}$`,
  still matched by `img[alt^="GP-017"]`);
- use per-run jittered coordinates (inside TOWN_BBOX) for every UNNUMBERED
  create and pagination seed, so the 8 m duplicate radius can never fire
  across tests/projects/runs;
- pre- and post-clean by soft-deleting same-labelled rows (soft delete frees
  the asset number for the partial unique index and the coordinates for the
  duplicate checks), so a passing suite leaves the list empty;
- pin `maxRedirects: 0` on every `page.request.post` so the frozen 303s are
  observable (the API request context follows redirects by default).

Operational note: if `test-run.db` outlives the 24 h session TTL with the
token already consumed, the login helper fails with instructions — the fix
is `rm -f test-run.db*` (runtime state, gitignored; safe at any point).

## Fixtures (`testdata/photos/`, generated)

`go run ./testdata/photos/gen` writes AND verifies every fixture below; run
output includes per-file sha256 (deterministic — identical on every run):

| File | What | sha256 |
|---|---|---|
| `portrait-orient6-gps.jpg` | 600×800 JPEG, EXIF APP1 with Orientation=6 + GPS IFD; processed output must be 800×600 with no APP1 | `52846307a4650b3e8b983792012314d18221c441bcd051b168ae1d059dbad8a9` |
| `portrait-orient8-gps.jpg` | same with Orientation=8 (holdout uses this) | `cf2ae83678ad03262485135b0f34c6d2e06ba21400a742822249e40198e1ce1c` |
| `landscape.jpg` | 320×200 plain JPEG, no EXIF (happy path) | `732bb9fe995f74d0c14c65a2a5193bb5a3d1ef30f75467f773120946522aa02c` |
| `tiny.png` | 1×1 PNG | `1bffb9ddaca4e6f750fb22599dcdd7a83de59d1066f987bb448f373b79b718b0` |
| `pixelbomb.png` | IHDR claims 50000×50000 (>40 MP) in 45 bytes; never a raster | `5f30f81f7864fd8612975cfde74c03a9c08769748559406e91809db1f14679f5` |
| `text-as.jpg` | plain text with a .jpg name | `075b49fd4d5c8220b36351f0d556ba35789a48adc96714d57853f038ba805fe7` |
| `heic.bin` | 16 bytes, `ftyp` box at offset 4, brand `heic` at offset 8 | `30d1619fdf4c18354f114d3de09fc36c645a9b3d51089da1bd830fbb2f8b6ac9` |

Oversized uploads are NEVER committed: the Go drafts synthesise a >15 MB
multipart body at runtime (a valid JPEG plus junk after its EOI marker).

## Schema oracle (`testdata/schema.expected.sql`)

The frozen M2 schema: `migrations/0002_*` must produce a database whose
`sqlite_master` CREATE statements equal this file (probe table first, copied
verbatim from `migrations/0001_init.sql`, then the product tables in file
order). T-201 owns that equality.

## Per-assertion visual notes (what a correct screenshot shows)

The mobile projects are the primary target; every note assumes the
iphone-se-320 viewport (320×640) unless stated.

- **A-201 login**: a single centred card — heading "Admin login", one labelled
  "Token" input (password-style), a full-width "Sign in" button. Nothing else
  competes for attention; tap targets ≥ 44 px tall; no horizontal scroll.
- **A-202 walk page** (320×640): top strip shows `#walk-pos` and
  `#walk-accuracy` ("accuracy 12 m") in readable type; the middle is the map
  canvas; **"Add light here" sits in the bottom third, full-width, thumb-
  reachable**. With accuracy 80 m the button is visibly disabled and "Save
  anyway (low accuracy)" appears beside/below it; choosing it re-enables the
  button. Geolocation denied: `#manual-lat` / `#manual-lon` number inputs
  appear (manual pin-drop fallback), still no horizontal scroll.
- **A-203 pin**: a pin marker with `aria-label="position pin"` centred on the
  map; `#pin-pos` under the map shows the live coordinates at 6 dp; dragging
  (or arrow keys, focus on the pin) moves it and updates `#pin-pos`; after
  "Confirm pin" the admin detail's `#light-pos` shows exactly those
  coordinates, same 6 dp string.
- **A-204 rejection**: a red alert box reading "…outside Groveland town
  bounds…" (role=alert) near the position readout; the form does NOT advance.
- **A-205 duplicate panel**: `#dup-panel` — an amber/bordered panel headed
  "Possible duplicate" showing the existing light (label, asset number,
  distance); numbered case: "Merge" (primary) + a cancel link; unnumbered
  case: "Merge" + "Save anyway". **Nothing is saved until one is chosen.**
- **A-206 photo**: after picking the portrait fixture, `#photo-preview`
  renders **wider than tall** (orientation already applied); the served photo
  on the list is a clean re-encoded JPEG (no EXIF/GPS block, long edge
  ≤ 1600).
- **A-208 detail/edit**: the admin detail shows the photo, `#light-pos` and a
  labelled form ("Label" filled) with "Save changes"; "Delete light" is a
  visually destructive (red) action separated from the save button.
- **A-209 list**: heading "Lights"; one row per light with a small square
  thumbnail (`img alt=<label>`), asset number + label text, status chip;
  "Older" at the bottom swaps in the next 50 via HTMX (no full reload);
  **thumbnails visible and no horizontal scroll at 320 px**.
- **A-201 guard (unauthenticated)**: visiting `/admin` logged out lands on
  the login card from A-201.

## Holdout mirror

`<holdout_dir>/drafts/` (see harness/policy.json) holds inert variants with
different data: coordinates 42.7752,-71.0148, asset `GP-042`, the
Orientation=8 fixture, a keyboard-moved pin, an east-of-bbox rejection, a
clicked Merge, a runtime-built 16 MB upload, a 51-light pagination boundary.
They activate by the same rename procedure, follow the same rerun-safety
rules (their runner always provides a fresh DB; the login helper still
exchanges the token exactly once per run and reuses the session), and are
equally frozen.
