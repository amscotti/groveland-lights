# T-206 — independent review (reviewer-alt): photo HTTP side

Basis: the attached diff (`harness/reviews/T-206.diff`), packet
`harness/tasks/T-206.md`, `AGENTS.md`, the inert A-206/A-207 Go draft
(`acceptance/drafts/admin_lights_go_test.go.txt`), `acceptance/contract.json`
(A-206/A-207), the on-disk `internal/img/img.go` and `internal/store/photos.go`
APIs, and `harness/policy.json`.

Verification limit: the diff is **not applied** in this checkout (there is no
`internal/web/upload.go` on disk) and shell execution is denied in this session.
This is therefore a static review of the diffed bytes; the handoff's executed
results (`check-fast`, red-check, coverage, `make check`) are taken on trust.
Line numbers below are post-patch, derived from the diff.

## Findings

### 1. [Medium — coverage theatre] The oversize test cannot fail for `LimitBody`

`TestPhotoOversizeBodyYieldsErrTooLargeAndLeavesNoFile`
(`internal/web/upload_test.go:314-341`) sends a 16 MB **photo part** through
`uploadRequest`, which calls `LimitBody` (`internal/web/upload_test.go:76-99`).
But `persistPhoto` also passes `MaxBytes: MaxPhotoBytes` to the img pipeline
(`internal/web/upload.go:168-175`), and `img.Process` enforces that cap before
any decode. Therefore, if `LimitBody` (`internal/web/upload.go:72-74`) were
deleted, made a no-op, or stopped wrapping the body, `ReadForm(32<<20)` would
happily read the 16 MB body, `img.Process` would return `img.ErrTooLarge`, and
the test would still see `ErrPhotoTooLarge`, 413, and no file. The same is true
of the `oversize` row in `TestPhotoDirCountsUnchangedOnEveryError`
(`internal/web/upload_test.go:421-459`).

The handoff's red-check only removes the `*http.MaxBytesError` mapping
(`harness/handoffs/T-206.md:164`), not `LimitBody`; it therefore proves the
mapping, not the body cap. A regression that accepts an arbitrarily large
request body with a small valid photo (the resource-exhaustion case the cap
exists for) would pass the whole suite and the A-206/A-207 draft, whose
oversize row also uses a >15 MB photo. Add a row that posts a small valid JPEG
inside a >15 MB body (for example a 16 MB `label`/padding field) and asserts
413/no file, or a direct `LimitBody` test with a body that is not itself over
the img cap.

### 2. [Medium — untested no-stray branch and a false handoff claim]

The “no stray files on any error” contract is not exercised for the two error
paths that matter most after the pipeline succeeds:

- failed `CreatePhoto` cleanup (`internal/web/upload.go:188-192`) — the file is
  written at `internal/web/upload.go:238`, then removed only if the store
  insert fails; no test makes that insert fail;
- nil `Server.Store` (`internal/web/upload.go:165-167`) and empty
  `PHOTO_DIR` (`internal/web/upload.go:223-225`) — never reached by the tests,
  because `newUploadServer` always wires a real store and photo dir.

`TestPhotoDirCountsUnchangedOnEveryError` (`internal/web/upload_test.go:421-459`)
only covers failures inside `img.Process`; `TestPhotoNonMultipartIsAWiringErrorNotASentinel`
(`internal/web/upload_test.go:655-672`) fails in `photoForm` before
`persistPhoto`. Neither reaches the cleanup. Yet the handoff says the opposite:
“upload paths are covered including the failed-insert cleanup's error branch via
the non-multipart and wiring tests” (`harness/handoffs/T-206.md:208`). That is
not true of the diff, and it matters because the same paragraph is the
handoff's coverage evidence for the A-207 no-stray rule.

A test can reach the cleanup by closing the real `*store.DB` before calling
`Photo` (the file is written, `CreatePhoto` fails, `os.Remove` runs), and a
nil-store test is a one-line setup. Until then the cleanup branch is dead code
as far as the suite is concerned.

### 3. [Low-Medium — permission assertion weaker than the frozen contract]

The packet freezes files `0644` and directories `0755`
(`harness/tasks/T-206.md:80-83`), and the production code uses exactly those
modes (`internal/web/upload.go:234,238`). The test, however, asserts only
`st.Mode().Perm()&0o022 == 0` for the file and the month directory
(`internal/web/upload_test.go:196-212`). That passes for `0600`, `0640`,
`0700`, `0750`, and even `0755` files: it catches group/world-writable modes
but does not pin the frozen `0644`/`0755`. The handoff calls this “0755/0644
(umask-safe assertion)” (`harness/handoffs/T-206.md:142`), which overstates the
check. If the exact modes matter, temporarily clear the umask for the
assertion (restoring it with `t.Cleanup`) and compare `perm == 0o644`/`0o755`,
or at least assert the read/execute bits the contract promises.

### 4. [Low-Medium — traversal test accepts any non-200, and the white-box branch is vacuous]

`TestPhotoTraversalAttempt404s` (`internal/web/upload_test.go:503-572`) is the
only test for the packet's “traversal must 404” clause. For the encoded
requests it asserts only `rec.Code == http.StatusOK`
(`internal/web/upload_test.go:527`); a 500 (including a recovered panic), 400,
or a redirect would pass. The literal dot-dot case does require a final 404
(`internal/web/upload_test.go:566`), so the clause is covered there, but the
encoded cases are not. The test's own message says “404 family”, which is
weaker than the packet.

The white-box loop at `internal/web/upload_test.go:537-545` is also weaker than
it looks: it skips every value for which `photoAbsPath` returns `false` (only
the NUL case is expected to), and it never includes the actual one-level
`"../secret.txt"` value. If `photoAbsPath` rejected every traversal outright,
the loop would be vacuous and the test would still pass. Add `"../secret.txt"`
and `"a/../secret.txt"` to the table, require `ok == true` for them, and assert
the resolved path is the guaranteed miss inside `PHOTO_DIR`; separately assert
the handler's status is exactly 404 for the encoded requests (or follow the
mux redirect and assert the final 404).

### 5. [Low — test name overclaims “without decoding”]

`TestPhotoPixelBombYieldsErrPixelBombWithoutDecoding`
(`internal/web/upload_test.go:368-394`) asserts only that the upload maps the
error to `ErrPhotoTooManyPx`; it has no observer for a full decode. The
“without decoding” ordering is genuinely proven by T-202's
`testHookFullDecode` test (`internal/img/img_test.go:313-339`), not by this
test. The upload-level row is still useful for the sentinel mapping, but the
name and the handoff's table row (`harness/handoffs/T-206.md:142`) say more
than the test checks. Rename it (e.g. “maps ErrPixelBomb”) or add the hook
evidence.

### 6. [Low — HEIC sniff drift and a false “identical” claim]

`sniffHEIC`'s `heicCompatibleBrands` (`internal/web/upload.go:284-288`) omits
`mif1`/`msf1`, while `img.heicBrands` (`internal/img/img.go:174-178`) includes
them in the compatible-brand loop. The code comment says the sets are “kept in
step … so the frozen wording and the pipeline's refusal always agree”
(`internal/web/upload.go:274-277`), and the handoff says “brand sets kept
identical (mif1/msf1 major-only)” (`harness/handoffs/T-206.md:195`). For a file
whose major brand is not a HEIC brand but whose compatible-brand list contains
`mif1`/`msf1`, the pipeline refuses it as HEIC (`img.ErrUnsupported`) but
`sniffHEIC` returns false, so `PhotoProblem` answers the plain 415 wording, not
the frozen “please choose JPEG instead of HEIC” wording. The fixture
(`testdata/photos/heic.bin`, major brand `heic`) does not exercise this. No
acceptance impact today, but the duplicate sniff can drift; either mirror the
img set or document the narrower wording and add a compatible-brand fixture.

### 7. [Low — collision edge is untested and destructive]

`writePhotoFile` uses `os.WriteFile` (`internal/web/upload.go:238`), which
truncates an existing file, before `CreatePhoto` checks the `path` UNIQUE
constraint (`internal/web/upload.go:181`). On the (2^-64) collision where a
file and a row already exist at the random name, the new bytes overwrite the
old file, `CreatePhoto` fails, and the cleanup at `internal/web/upload.go:191`
removes the path — destroying the existing photo's file while its row remains.
The store's own contract says a duplicate path “must surface as an error,
never a silent overwrite” (`internal/store/photos.go:55-58`). The test only
asserts that two uploads mint different paths
(`internal/web/upload_test.go:173-190`); there is no collision test. Use
`O_CREATE|O_EXCL` or check/retry before writing. Low probability, but it is an
untested production error path.

### 8. [Info — parse-order doc contradicts T-205's frozen sequence]

The `Photo` doc says “never call ParseMultipartForm / FormValue / ParseForm
BEFORE Photo” (`internal/web/upload.go:94-108`) while also documenting reuse
when the caller parsed first (`internal/web/upload.go:141-142`). T-205's packet
freezes parse → CSRF → bbox → photo (`harness/tasks/T-205.md:56-69`), which is
the handoff's Order B. The handoff flags the CSRF caveat
(`harness/handoffs/T-206.md:187-191`), which is good, but the in-code doc's
first instruction points at Order A, which stores the photo before CSRF/bbox
and leaves a row+file when the create is later rejected. The doc should present
Order B as the required order for T-205 (Order A only if CSRF is checked
first), not the default. This is an integration-contract risk, not a test
failure.

### 9. [Info — handoff precision]

- The handoff says “11 test functions” (`harness/handoffs/T-206.md:26`);
  `upload_test.go` contains 13 `Test...` functions.
- The `TestPhotoDirCountsUnchangedOnEveryError` condition
  (`internal/web/upload_test.go:442`) exempts `empty.jpg` from the error
  assertion, so an error for the empty part would pass that row; the separate
  `TestPhotoAbsentFieldIsNotAnErrorAndStoresNothing` does cover the nil,nil
  requirement.
- `TestPhotoReusesFormTheCallerParsed`
  (`internal/web/upload_test.go:635-652`) asserts only that one file appears;
  it does not check the stored dimensions/row, so a reuse implementation that
  stored the wrong part could still pass that row.
- The handoff's “0755/0644” claims are covered by finding 3; the traversal
  “both 404 in the end” prose is covered by finding 4.

## Checked and found sound

- **No existing test deleted, skipped, narrowed or disabled.** The diff adds
  new files only; there is no `t.Skip`, `test.skip`, `test.only`, `.fixme`,
  `TODO`/`FIXME`, `panic("not implemented")`, or “Phase N” string in the added
  code. The two `//nolint:gosec` lines carry on-line reasons (rule 9).
- **No fixture-value special-casing in production.** `upload.go` contains only
  the frozen limits/messages (`15 << 20`, 40 MP, 1600 px, the 413/415 strings,
  the JPEG content type and cache header); no fixture filename, fixture
  dimension, or acceptance coordinate appears in non-test code.
- **No mocks bypassing the behaviour under test.** The upload tests use a real
  temp SQLite store opened through `store.Open`, a real temp `PHOTO_DIR`, and
  the real `img.Process` pipeline; the serving tests go through the real
  `s.Handler()` chain. `photoAbsPath` is additionally exercised white-box.
- **The happy path is genuinely pinned.** The portrait-orient6 test checks the
  source really carries APP1, then asserts the stored file is a JPEG from SOI,
  has no APP1/“Exif”, decodes as exactly 800×600, matches
  `YYYY/MM/<16-hex>.jpg`, has a different name on a second upload, and that the
  row path/dimensions/byte count/sha256 match the stored bytes (the hash is of
  the re-encoded bytes, not the source).
- **The optional-photo contract is covered.** No part, a zero-byte file part,
  and a plain value field named `photo` all return `(nil, nil)` and store
  nothing.
- **Rejection mapping is covered.** Text-as-JPEG, HEIC, pixel bomb, oversize,
  garbage and empty are exercised; the HEIC pair (HEIC fixture vs
  text-as-JPEG) pins the frozen HEIC wording against the plain 415; the
  `PhotoProblem` table covers every sentinel, wrapped/deep-wrapped errors,
  raw `*http.MaxBytesError`, unknown errors and nil.
- **Serving headers are pinned exactly.** The happy-path GET asserts
  `Content-Type: image/jpeg`, `Cache-Control: public, max-age=31536000,
  immutable`, and byte equality with the stored file; HEAD 200 and missing 404
  are covered.
- **No new dependencies, protected paths or migrations.** The diff adds only
  `internal/web/upload*.go` and the handoff; `go.mod`/`go.sum` are unchanged;
  `acceptance/**`, `Makefile`, `.golangci.yml` and goldens are untouched.
- **The containment resolver is sound in the implementation.** `photoAbsPath`
  roots the value with `path.Clean("/"+urlPath)` and re-proves containment with
  `filepath.Rel` (`internal/web/upload.go:351-370`); the defect is the
  traversal test's assertion strength, not an obvious escape.

## Verdict rationale

The packet's explicit test list is substantially implemented and the core
behaviour is real: real temp store and photo dir, real pipeline, exact output
dimensions/metadata stripping, sha256 of the re-encoded bytes, no files/rows on
the covered rejects, exact serving headers, both parse orders, and the full
`PhotoProblem` table. I found no deleted/skipped/narrowed pre-existing test, no
production branch on fixture values, no mock substituting for the behaviour
under test, and no dead code added to satisfy a linter.

The Medium findings are real coverage holes, not a hidden broken deliverable:
the body cap is not isolated (the current test passes if `LimitBody` is a
no-op), and the failed-insert cleanup / nil-store / empty-PHOTO_DIR branches
are untested while the handoff claims the cleanup is covered. The Low findings
are assertion strength (permissions, traversal status), a test-name overclaim,
the HEIC sniff drift, and a rare destructive collision edge. The correct
disposition is merge-with-follow-up rather than another gate cycle: fix the
two Medium holes (body-cap test and store-failure cleanup/nil-store test) and
correct the handoff's coverage claim while T-205 is still building the
handler-level no-stray behaviour.

VERDICT: PASS
