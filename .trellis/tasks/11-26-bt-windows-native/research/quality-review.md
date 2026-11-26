# Windows BT quality review

Date: 2026-11-26. Platform: native Windows amd64.

## Scope

- Reviewed `internal/desktop/bt_config.go`, native launcher integration, Windows example configuration, release copy rules, Windows BT documentation and the engine-dialog guidance.
- Read-only review of the backend path changes, qB publication/operation ownership checks, BOM handling and `scripts/verify_windows_bt.py`; implementation and end-to-end fixture results remain recorded by their owners.
- Used the `trellis-check` workflow. CodeGraph was tried first for call sites; its index did not contain the newly added `sameEnginePath`, so current changed-source verification used scoped `rg` and direct source inspection.

## Findings (fixed)

### Explicit JSON null silently became an omitted field

- File: `internal/desktop/bt_config.go`.
- Issue: `encoding/json` accepts explicit `null` for pointers and scalar fields. For example, `downloads: null` alongside a valid search section, or `root: null` in a disabled download section, could pass as an omitted value.
- Fix: added scoped `decodeNativeBTJSON` validation after the existing strict duplicate/unknown/EOF checks. It rejects explicit null at the top level and in the two flat configuration sections while retaining genuinely omitted optional fields. The shared decoder and unrelated configuration formats remain unchanged.

### Illegal UTF-8 inside a JSON string was replaced silently

- File: `internal/desktop/bt_config.go`.
- Issue: Go JSON decoding replaces invalid string bytes with U+FFFD, so the documented UTF-8-only configuration boundary was not enforced for string values.
- Fix: validate `utf8.Valid(data)` before decoding the BOM-stripped BT document. A real invalid-byte string regression case now fails closed. UTF-8 BOM is still accepted only at the beginning; UTF-16 and repeated BOM are rejected.

## Findings (not fixed)

None within this review scope. Backend ownership comparisons, fixed-path command construction, runtime-only complete-root authorization, environment priority, release examples and the UI guidance showed no additional concrete defect in the inspected changes.

## Temporary behavioral matrix

Created `internal/desktop/bt_windows_review_tmp_test.go`, executed it on native Windows, then deleted only that newly generated test source as required by AGENTS.md.

The final run had **6 test functions and 29 invalid-document subtests**, producing **35 Go PASS entries**. Counting one evaluated contract condition group as one behavior check, and excluding setup/write/error guards and the host precondition, the matrix contained **64 behavior checks**:

| Area | Behavior checks | Coverage |
| --- | ---: | --- |
| Missing file | 3 | Empty overlay, unchanged process environment, no implicit file creation |
| Valid merged profile | 3 | Chinese/space profile, BOM, relative private references, effective environment root as default engine root, explicit overrides, exact-byte input preservation |
| Independent modes and legacy flags | 8 | Search-only, no download injection, old true/false flags, explicit mode priority, disabled without credentials |
| Invalid documents | 29 | Unknown/duplicate/case-folded fields, trailing object, missing/unsupported schema or modules/modes, external without root, relative root, every null field/section, UTF-16/repeated BOM, invalid UTF-8 in a string |
| File and byte limits | 4 | Raw BOM-inclusive 65,536 bytes accepted, 65,537 rejected, empty file and directory rejected |
| Referenced paths | 17 | Empty/control/trim/traversal rejection, stable profile-relative and absolute paths, real Windows drive-relative and root-relative rejection |

This is configuration and safety behavior evidence; it is not evidence of a public indexer search, actual peer transfer or a clean-machine installation.

## Verification

All commands ran from the repository through `python -B scripts/project_env.py -- ...`, with controlled caches and temporary files under `.runtime/`.

- Pass: `.tools/go/bin/gofmt.exe -w internal/desktop/bt_config.go internal/desktop/bt_windows_review_tmp_test.go`.
- Pass: `go test ./internal/desktop -run TestReviewBT -count=1 -v`, including the final null and invalid UTF-8 cases.
- Pass: `go vet ./...`, rerun after the final UTF-8 fix.
- Pass: `npm --prefix web run typecheck` (`vue-tsc --noEmit`).
- Pass: `go test ./...` with the temporary behavioral matrix present before the final UTF-8-only change; the entire affected desktop matrix and `go vet ./...` were then rerun after that change.
- Pass: final `go test ./...` after deleting the temporary matrix. All packages reported no test files; this final run establishes compilation only, separate from the executed temporary behavior evidence above. `Test-Path` confirmed the generated review test source is absent.
- Pass: scoped `gofmt -l` produced no file names; `git diff --check` reported no whitespace errors (only repository line-ending conversion notices).
- The parent agent owns final desktop rebuilding, full release/media/UI verification and spec/PRD synchronization. It was informed that the last UTF-8 validation change requires a fresh desktop build.

## Contract synchronization

The Windows BT spec should retain the executable distinctions verified here: omitted optional fields are allowed, explicit null is rejected, raw byte budget includes BOM, UTF-8 must be valid before Go JSON decoding, private relative references resolve under profile, explicit nonempty environment settings retain priority, and old `*_ENABLED` flags retain their fallback semantics. The parent agent is updating the shared task/spec documentation to avoid concurrent edits.
