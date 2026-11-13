# T-206 reviewer — Photo HTTP side: upload parsing, storage, serving

Basis: the attached diff (`harness/reviews/T-206.diff`), packet
`harness/tasks/T-206.md`, the oracle draft
`acceptance/drafts/admin_lights_go_test.go.txt` (readable, unedited — verified
byte-for-byte untouched by this diff), `acceptance/contract.json` (A-206 at
:70-74, A-207 at :76-80), the pre-change sources on disk
(`internal/web/{web,context,csrf,web_test}.go`, `internal/img/img.go`,
`internal/store/photos.go`), the T-202/T-208 handoffs (the APIs this task
compiles against), and `docs/spec/architecture.md`.

Method note: shell execution was denied this session, so the handoff's
exit-code table was not re-run and the worker's worktree (`../wt/T-206`) is
not present for direct inspection. Every load-bearing claim was verified
statically against the diff content and the merged sources on disk; line
numbers below are the new files' own line numbers (diff hunks are `+1,370`
and `+1,675`, so they map 1:1).

## Packet conformance — frozen API implemented exactly

- `MaxPhotoBytes = 15 << 20` (upload.go:32), sentinel messages
  character-for-character the packet's frozen strings (upload.go:54-58),
  `LimitBody(r)` one-arg signature (upload.go:72-74), `UploadedPhoto` with
  `store.PhotoID` (upload.go:79-83), `(*Server).Photo` (upload.go:109),
  `PhotoProblem` (upload.go:255-272) — all match the packet :35-73 block.
- Frozen pipeline options `{MaxBytes: MaxPhotoBytes, MaxPixels: 40_000_000,
  MaxEdge: 1600}` (upload.go:168-172 vs packet :80-81); storage layout
  `PHOTO_DIR/YYYY/MM/<16-hex>.jpg` with UTC year/month, dirs 0755 / file 0644,
  name from `crypto/rand` (upload.go:222-242); sha256 over the RE-ENCODED
  bytes (upload.go:186, and the row/file cross-check at upload_test.go via
  `uploadAssertRowMatchesFile`). All as frozen.
- Serving route `GET /photos/{path...}` registered via `init()` +
  `register()` — exactly the feature-file convention of `web.go:26` /
  architecture.md ownership map. `Content-Type: image/jpeg` exact and
  `Cache-Control: public, max-age=31536000, immutable` (upload.go:333-334),
  404 on miss (upload_test.go:492-497). No other feature registers `/photos`
  (grep of `internal/web` confirms), so no route conflict.
- Error mapping `img.ErrTooLarge→ErrPhotoTooLarge`,
  `img.ErrPixelBomb→ErrPhotoTooManyPx`, `img.ErrUnsupported→
  ErrPhotoUnsupported` (upload.go:202-216) matches packet :60-63 and the
  T-202 handoff's status table. The independent HEIC re-sniff is the approach
  T-202's handoff explicitly prescribed ("my `isHEIC` is package-private on
  purpose — T-206 must do its own sniff"), so the duplication is sanctioned,
  not drift by choice.
- (nil, nil) for absent field / no-filename part / 0-byte part
  (upload.go:114-120, 151-154) — "photo is optional" honored.
- No-stray rule: rejects fail in `photoForm` or `img.Process` before any
  write; the one post-write failure path (`CreatePhoto` error) removes the
  just-written file (upload.go:189-192). `CreatePhoto`'s UNIQUE path makes
  that cleanup the correct pairing with the store contract
  (`internal/store/photos.go:55-58`).
- Every test name the packet lists (:87-94) exists and asserts the named
  behaviour, plus three extra parse-order/wiring tests. Fixtures all exist in
  `testdata/photos/`; the >15 MB case is built in memory (JPEG + pad after
  EOI) exactly as packet :95-96 requires.

## Security

- **Path traversal — sound, double-guarded.** `photoAbsPath`
  (upload.go:351-370) roots the wildcard (`path.Clean("/"+v)` drops every
  `..` above root), NUL-rejects, then re-proves containment with
  `filepath.Rel` — which also covers the Windows-separator case (`..\..`
  survives `path.Clean` as literal bytes but fails the `Rel` prefix check).
  The percent-encoded `%2e%2e` vector that does bypass the mux's literal-dot
  redirect is exactly the case tested through the real handler chain
  (upload_test.go:510-522) plus a white-box table on the resolver
  (:524-536), and the literal `..` case through a real HTTP server asserting
  a final 404 (:538-553). The handoff's red-check (guards disabled → the
  planted outside-secret actually served, 6 failures) is the right evidence
  that these tests bite.
- **Upload limits.** Body capped at 15 MB by `LimitBody` before any read
  (upload.go:72-74); `ReadForm(32<<20)` keeps every compliant upload in RAM
  (body ≤ 15 MB < 32 MB, so no temp-file spill); the img pipeline re-checks
  bytes, refuses >40 MP from the header alone before decode (T-202-proven
  order), and re-encodes so stored bytes are always pipeline-clean. Filenames
  are server-minted; the client controls nothing about the stored name.
- **CSRF — no exposure introduced, one forward risk (see F1).** This diff
  ships no POST route; `Photo` is a helper for T-205. The handoff honestly
  discloses that its "Order A" persists the photo before the CSRF check and
  pins the CSRF-safe "Order B" (parse → VerifyCSRF → Photo) with the
  `*http.MaxBytesError` mapping both orders need. The packet made the parse
  order this task's documentation duty (:102-104) — done.
- **Info leaks.** `PhotoProblem` collapses everything unrecognized to
  500 "internal error" (upload.go:290-291); wrapped errors containing
  filesystem paths never reach the wire through it. Served responses are
  pipeline-produced JPEGs with explicit `Content-Type` — no sniffing
  surprises; `nosniff` and CSP apply via the shared middleware
  (`web.go:99-113`), and `img-src 'self'` admits `/photos`.
- **SQL** — parameterized sqlc queries via `store.CreatePhoto`; nothing new.
  No `template.HTML`, no JS, no secrets in any output.

## Architecture

- **Ownership:** diff touches exactly `internal/web/upload.go`,
  `internal/web/upload_test.go`, and the handoff — all inside `owns`
  (`internal/web/upload*.go`) plus the sanctioned handoff file. No
  `routes.go`/`base.html` edits, no acceptance/harness/golden edits (rule 1).
- **Parse-don't-validate / typed IDs:** multipart parsed once at the
  boundary; `UploadedPhoto` is a typed result; `store.PhotoID` throughout;
  zero `r.FormValue`/`ParseForm` reads in the new code (grep-verified) —
  rule 6 satisfied.
- **HTMX/#map:** no templates, no partials, no hx-anything — the map rule is
  untouched by construction.
- Rules 3/5/9: no new dependencies (stdlib + own packages only), no Phase-N
  strings or TODO/FIXME in code (the only hits are the handoff prose, which
  is allowed), three `//nolint:gosec` each with an on-line reason
  (upload.go:233, 237, 330) for the contract-frozen 0755/0644 and the
  proven-contained open.

## Tests — do they exercise the behaviour?

Yes, at the right level: `newUploadServer` wires a REAL temp `PHOTO_DIR` and
a REAL `store.Open` SQLite DB onto `Server.Cfg.PhotoDir`/`Server.Store`
exactly as production does (same pattern T-208 documented for probe tests),
and `uploadRequest` applies `LimitBody` before `Photo`, i.e. the frozen
production order. Serving tests go through `s.Handler()` (full middleware
chain, real mux) and assert exact header strings, byte-identity with the
stored file, and HEAD via the mux. The A-206/A-207 draft assertions that
belong to this layer (413 message, HEIC wording, no-APP1, 800×600, file/row
counts) are all pinned here; the draft itself remains an inert `.txt` as the
packet predicts (its only `@A-206` Playwright twin also lives in
`acceptance/drafts/admin-walk.spec.ts.txt`, unactivated — grep-verified), so
the e2e no-op claim checks out. Two documented red-checks (traversal guards
removed; MaxBytes mapping removed → suite fails) demonstrate the tests are
not vacuous.

## Findings (none blocking)

1. **[medium, forward risk for T-205] Photo persists before CSRF in the
   documented "Order A"** — upload.go:94-108 presents Photo-first as the
   primary order; a request that later fails CSRF leaves a stored file+row
   (unauthenticated storage/CPU writes up to the caps). Not a vulnerability
   in THIS merge (no POST route calls `Photo` yet), and the handoff's open
   issue 2 discloses it with the CSRF-safe Order B. **Action for
   T-205/planner:** mandate Order B (or an auth/session gate before
   `Photo`); the acceptance draft does not test strays on 403, so only
   review can enforce it.
2. **[low] `sniffHEIC` brand sets diverge from `img.isHEIC`** — upload.go:283-288
   omits `mif1`/`msf1` from the compatible-brand list while
   internal/img/img.go:174-178 checks them in both positions. A file with a
   non-HEIC major brand but a `mif1`/`msf1` compatible brand is refused 415
   by img yet gets the plain (non-HEIC) wording. Status and sentinel are
   identical; only the message wording differs, in a case no fixture
   exercises. The handoff's "brand sets kept identical" is imprecisely
   worded (its parenthetical does disclose "mif1/msf1 major-only").
3. **[low] An empty `YYYY/MM` directory can survive a `WriteFile` failure**
   after `MkdirAll` succeeded (upload.go:233-239). A-207 and the draft count
   files, not directories, so the frozen rule is met; cosmetic only.
4. **[info] Handoff count off**: "11 test functions" — the diff has 13.
5. **[info] `LimitBody` passes a nil ResponseWriter to `MaxBytesReader`**
   (upload.go:74) — required by the packet's frozen one-arg signature; legal
   on this stdlib (the writer is only used to signal connection-close on
   overflow; net/http's own drain/close behaviour bounds the residue).
6. **[info] Multiple `photo` file parts**: only the first is stored, extras
   ignored — documented (handoff open issue 5); no stray writes result.
7. **[info] Rule 7 wording**: `photoServe` is covered by scenario tests plus
   the white-box `photoAbsPath` table rather than a single table-driven
   handler test; `PhotoProblem` has the true table test. Coverage is real
   and red-checked; acceptable for a file-serving handler.

## Verdict

The frozen API is implemented exactly as the packet specifies, the security
posture of both the intake and serving paths is correct and adversarially
tested, ownership and repo rules are respected, and the one genuine design
tension (CSRF vs single-parse) is honestly disclosed with a safe alternative
pinned for T-205 rather than hidden.

VERDICT: PASS
