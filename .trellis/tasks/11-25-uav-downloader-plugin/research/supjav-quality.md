# SupJav independent quality review — 2026-11-25

## Scope and outcome

The independent reviewer checked the SupJav worker acquisition and local TS normalization path, the terminal-versus-candidate failure boundary, private source selectors, and the existing HLS relay regression suite. The implementation owner applied coordinated worker fixes; the reviewer independently reproduced the TS defect and reran the final production paths. The parent owns application/browser verification and the final repository-wide Go gate.

The final directed command passed with **18 top-level tests and 68 passing subtests**. This includes **22 SupJav production-media scenarios** and the **22 existing native-downloader relay scenarios**. These are scenario counts, not a count of assertions or unique production functions.

```powershell
python -B scripts/project_env.py -- .tools/go/bin/go.exe test ./internal/collectionworker -run '^(TestSupJavQuality|TestQuality)' -count=1 -v
```

Evidence: `.runtime/verify/supjav-quality-20261125-1/`. The final command exited 0 in 117.461 seconds. The earlier 22-scenario media-only run is retained for diagnosis but is not added again to the final counts.

## Findings (fixed)

### Inconsistent TS adaptation/payload declarations were accepted

- File: `internal/collectionworker/uav_supjav.go`.
- Issue: `validSupJavTSPacket` initially accepted adaptation-only packets with a declared adaptation length of 0 or 182, and packets declaring both adaptation and payload with adaptation length 183. The first two leave undeclared bytes; the latter leaves no payload. All three were independently reproduced as failing rejection tests in `ts-adaptation-before.log`.
- Fix: adaptation-only packets require length 183; adaptation-plus-payload permits at most 182. The implementation owner changed the production parser after the reviewer supplied the concrete counterexamples. Seven positive/negative length boundaries now pass, followed by the real-media matrix. The parent synchronized this constraint into the UAV contract.

### Terminal process and local I/O failures must survive fallback classification

- Files: `internal/collectionworker/tools.go`, `internal/collectionworker/uav.go`, `internal/collectionworker/uav_supjav.go`.
- Issue: wrapping every tool failure as a candidate error could initiate another source after the owned tool tree had failed to exit, or hide a local storage failure. Ignoring a normalizer close error would similarly obscure local I/O failure.
- Fix: `runTool` returns a fixed `errUAVProcess` for process cleanup failure and `errUAVStorage` for recognized fixed-tool local I/O failures. Download, offline remux, and full decode retain both terminal classes. `fetchUAV` advances only for `errUAVCandidate`; cancellation, storage/process errors, staging/budget failures, changed identity and unexpected staging entries stop. The normalizer propagates write, sync, and close failures as storage errors.
- Verification: six controlled tool subprocess scenarios exercise storage-versus-candidate propagation across download/remux/decode. These subprocesses deliberately implement only the error protocol, and are not counted as real-media evidence. Process cleanup failure and sync/close failure were traced statically through the concrete branches; no orphan-process simulation, cleanup-failure injection or new production injection API was introduced.

## SupJav actual production-media matrix

Each scenario invokes production `fetchUAVMedia`, the production finite-resource relay and fixed downloader arguments/environment, the installed locked `yt-dlp 2026.10.26`, production TS normalization, and actual local FFmpeg remux/full decode/duration checks. The fixture is an actual generated eight-second MPEG-TS video, split at complete packet boundaries. Keys, ciphertext, HTTP responses and manifests are controlled test data. Successful cases leave exactly one regular `media.mp4`; all outcomes revoke their relay registration.

The controlled TLS fixture replaces only dialing to an owned local server and explicitly trusts that fixture certificate for its expected name. A separate existing test checks rejection of an untrusted TLS certificate, and private redirect tests use the production public dialer. This is offline evidence and does not establish public-provider reachability.

| Scenario group | Expected and observed |
| --- | --- |
| Ordinary TS | Success |
| Recognized PNG wrapper on both fragments | Success |
| Recognized wrapper at exactly 8000 bytes | Success |
| Ordinary AES-128, explicit IV | Success |
| Wrapped AES-128, explicit IV | Success |
| Wrapped AES-128, default IV from sequence 11/12 | Success |
| Wrapped AES-128 with key rotation | Success |
| Changed nonfinal padding byte, identical resulting TS | Success; explicit limitation below |
| Unknown prefix / wrapper over 8000 bytes | Rejected in each case |
| Internal bad sync / invalid TS control field | Rejected in each case |
| Partial final TS packet | Rejected |
| PNG-only body / wrapper followed by four packets | Rejected in each case |
| Wrong AES key / short AES key | Rejected in each case |
| Unpadded AES tail / truncated ciphertext | Rejected in each case |
| Short HTTP transfer / extra TS junk / missing fragment | Rejected in each case |

These are eight accepted and fourteen rejected production-media scenarios. The normalizer also has nine exact-byte fixtures covering plain data, multiple wrappers across the 64 KiB buffer, in-place compaction, empty data, unknown corruption, oversized wrappers, partial tails, too few packets after a wrapper, and a truncated packet immediately before a valid wrapper. A pre-canceled invocation preserves its input unchanged. Cleanup checks preserve unexpected files/directories and remove only the expected owned regular files.

The actual pinned downloader removes padding using the terminal length byte without validating every PKCS#7 padding byte. Changing a nonfinal padding byte while preserving the resulting TS plaintext is therefore accepted. This review explicitly records that successful structural validation and decoding do **not** prove strict AES padding validity, authenticity, semantic completeness or losses within the existing duration tolerance. The contract now states this boundary. An unpadded input which loses actual TS tail bytes is rejected by the strict packet grid rather than repaired by generic resynchronization.

## Existing behavior and cross-layer boundaries

The restored 13-test legacy suite retains the original 22 real downloader cases: normal/AES/key lengths, redirected key resource and header scope, exact/missing/ignored/mismatched/truncated/chunked byte ranges, invalid HTTP statuses, missing fragments, TLS/public-address rejection, cancellation and resource cleanup. It also checks whole versus gracefully truncated decoded duration. All passed in the same final command.

Static review confirms `sourceOffset` and `sourceIndex` remain private to the embedded bridge and worker DTOs. They do not enter public search/preview/task structs, canonical identity or durable request metadata. Worker resolution checks range, monotonic progress and HLS-versus-ST slot compatibility; SupJav rejects init maps and byte ranges. After a candidate failure, the same context and operation continue through staging budget checks, restricted cleanup and disk reserve before the next source. Candidate-to-ST task execution, stable submission/restart and browser import/player behavior are covered by the parent's separate integration evidence, not by this review's direct media method calls.

## Evidence and cleanup

- `result.json`: exact final command, counts, source SHA-256 hashes, scope/limits, cleanup and parent frontend evidence reference.
- `final-review-tests.log`: complete final 18-test/68-subtest run.
- `production-media-tests.log`: earlier successful production-media-only run.
- `ts-adaptation-before.log`: preserved three-case failure before the coordinated structural fix.
- `supjav_quality_test.go.txt` and `supjav_legacy_quality_test.go.txt`: noncompiled exact reproduction sources. Temporarily copy them into `internal/collectionworker` under their original filenames to rerun the recorded command, then remove them again.
- `tool-helper/main.go` and `tool-helper.exe`: local controlled subprocess for the six error classification cases; rebuild through the project wrapper if needed.
- `compile-after-cleanup.log`: post-cleanup package compilation passed with no tests selected; it is compilation evidence only.

The reviewer removed only its generated `supjav_quality_test.go` and `supjav_legacy_quality_test.go` after saving the `.txt` copies. The implementation owner separately owns removal of `uav_supjav_temporary_test.go`. No commit, deployment or task archival was performed.

## Findings (not fixed)

No confirmed in-scope code defect remains from this review. Live SupJav/provider search or video transfer, Docker execution, and target NAS behavior were not executed and remain separate acceptance boundaries. No claim of live provider status is made.

## Verification

- Directed tests and real downloader/media scenarios: pass.
- Post-cleanup worker package compilation: pass.
- TypeCheck and frontend build: pass in the parent's same-tree run, recorded at `.runtime/verify/supjav-frontend-20261125-1/frontend-build.log` and `result.json`; the reviewer inspected this evidence rather than rerunning it.
- Lint (`go vet ./...`): pass in the parent's final same-tree run; `git diff --check` also passed in the reviewer's run.
- Repository-wide `go test ./...` after cleanup: pass as package compilation with no retained test sources. Behavioral evidence remains the directed suites above and the parent's integration suites.
- Final Windows native and Linux amd64/arm64 cross-builds for the application and worker: pass in the parent's run. The reviewer inspected `.runtime/verify/supjav-build-20261125-1/result.json`, `go-vet.log`, and `go-compile.log`; no target Linux execution is inferred from cross-compilation.
