# SupJav implementation evidence — 2026-11-25

## Scope and ownership

This report covers the shared site/ID gate, private Python bridge, worker acquisition path and reusable bridge verification. The main agent owns application/UI propagation, documentation, protocol/browser harnesses and the final repository checks. Independent media/relay review is recorded separately by the quality agent.

The existing UAV 3.8.4 source commit `d40f09457fbd38dd7dc94580637242afafaf3939`, archive hash, deployment dependencies, public task API and database schema remain unchanged.

## Production files changed

- `internal/collectioncontract/uav.go`: add the fifth fixed site and enforce positive decimal IDs with at most 12 digits, without leading zeros.
- `internal/collectionworker/uav_bridge.py`: import the pinned SupJav browser/detail parser; accept only canonical/search-language numeric video links; recognize actual empty-page markers and `/page/N/?s=...` pagination; independently reject old query-only pagination and fragments. Select EVS/FST/VAS/LUC/VOE/TV finite ordinary-TS HLS in pinned provider order, then ST progressive MP4 with its own Referer. Add bounded private `sourceOffset` input and `sourceIndex` output, without public or persisted selectors.
- `internal/collectionworker/uav.go`: independently validate selected identity/slot/format; acquire one source, then retry the next fixed slot only after a candidate content/transport failure, released relay scope, budget check, owned staging cleanup and disk reserve. Never join candidates. Download, remux and decode preserve cancellation, storage failures and `errUAVProcess` without fallback. SupJav HLS passes through local TS normalization before existing offline remux/full decode/frame/duration gates.
- `internal/collectionworker/uav_supjav.go`: bounded in-place decrypted-TS normalization; only a known PNG signature at the expected packet boundary permits at most 8000 prefix bytes followed by five valid TS packets. Reject arbitrary resynchronization, TEI, invalid adaptation control/length, partial tails and unknown damage. AFC=2 must occupy exactly 183 adaptation bytes; AFC=3 permits at most 182 to retain payload. Read/write/truncate/sync/close failures are fatal local storage errors. Candidate cleanup validates the entire staging entry list before deleting only known regular downloader/remux files.
- `internal/collectionworker/tools.go`: retain the process-tree cleanup error as a sentinel; classify bounded fixed-tool local I/O failures without exposing raw stderr.
- `scripts/verify_uav_bridge.py`: extend the reusable offline actual-parser/native-downloader harness to SupJav; this is retained production verification infrastructure, not a generated unit-test source.

## Actual checks

All commands inherited project-owned cache/temp settings through `python -B scripts/project_env.py -- ...`.

### Shared/worker Go checks

Evidence directory: `.runtime/verify/supjav-implementation-20261125-1/`.

- `.tools/go/bin/gofmt.exe -w internal/collectionworker/uav_supjav.go`: completed successfully after the final AFC/close-error changes. The other edited Go files had already been formatted.
- `.tools/go/bin/go.exe test ./internal/collectionworker -run SupJavTemporary -v`: exit 0. Five test functions cover four byte-preserving positive normalization cases, eleven rejected malformed cases, cancellation/local file errors, known-file cleanup and tool storage-error classification. These are structural TS fixtures, not decodable-video evidence. Full log: `temporary.log`.
- `.tools/go/bin/go.exe vet ./internal/collectioncontract ./internal/collectionworker`: exit 0; full log: `vet.log`.
- `.tools/go/bin/go.exe test ./internal/collectioncontract ./internal/collectionworker`: exit 0 in 117.113 seconds, with the then-present independent quality tests included; full log: `packages.log`.
- Exact commands, durations and exit codes: `go-results.json`.

The temporary unit source was copied to `uav_supjav_temporary_test.go.txt` inside the evidence directory and then deleted from `internal/collectionworker/`, as required by AGENTS.md. The quality agent separately owns preservation/removal of its temporary tests.

### Real pinned Python parser and native HLS checks

Command: `.runtime/research/uav/venv/Scripts/python.exe -B scripts/verify_uav_bridge.py`; exit 0 in 14.939 seconds.

Full command log: `.runtime/verify/supjav-implementation-20261125-1/bridge.log`; command metadata: `bridge-results.json`. Native downloader media/logs and machine-readable result: `.runtime/verify/uav-real-bridge-45135c6b7b1148f68b971a8fb29e2e8f/result.json`.

Observed parser counters:

| Category | Passed |
| --- | ---: |
| Five-site search, detail resolve, explicit empty result | 5 each |
| Rejected bad markup/manifest/identity/format cases | 30 |
| Request/curl redirect handling | 2 |
| Real SupJav static provider parsers | 7 |
| Manifest fallback/AES acceptance/private offset selection | 10 |
| Wrong-query, legacy-query and fragment pagination | 3 |
| Private offset type/bounds, cross-site selectors and numeric ID bounds | 16 |

Provider fixtures exercise EVS/LUC packed extraction, FST declared-HLS `.txt`, VAS actual POST payload, VOE static encoded config, TV extraction and ST robotlink expressions. They replace transport and deny unhandled real sockets; they do not replace the upstream parser classes. Each selected provider's Referer is checked, especially ST versus HLS.

The pinned native downloader separately succeeded for ordinary HLS and AES-128, failed for missing segments and unsupported encryption, retained inline finite-playlist behavior and did not invoke network FFmpeg fallback. Successful outputs contained positive decoded frames. The independent quality agent supplies the production relay plus wrapped/encrypted TS media matrix; the small bridge-native check alone is not its substitute.

## Findings retained

- An early bridge harness run incorrectly expected only `ValueError` for broken pages; the real upstream SupJav browser raises `RuntimeError`. The harness catch was corrected to both expected parser failure types, then rerun successfully; no production parser was mocked or changed to hide the failure.
- Mainline review identified that the generic query-page detector could incorrectly mark SupJav as having a next page. SupJav now exclusively uses its actual path-page detector, with same query and no fragment; three negative regressions pass.
- Independent review exposed three adaptation-field/payload length counterexamples. The production packet gate was corrected, and the final package tests passed with those quality regressions present.
- Process reclamation failure is fatal in all three transfer/remux/decode branches. Retrying another source cannot proceed while an owned process tree may remain alive.

## Boundaries and follow-up

- Supports finite ordinary MPEG-TS SupJav HLS (including ordinary native AES decryption) and the final ST progressive-MP4 source. Init maps, fMP4, BYTERANGE, live/LL-HLS, external audio and unsupported encryption remain explicit failures.
- The local pass runs after native per-fragment decryption. It proves bounded TS structure and feeds the existing decode/duration checks; it does not authenticate content or validate every AES padding byte. The pinned downloader can discard malformed padding, so no strict padding-authenticity or semantic-completeness claim is made.
- Cancellation, stage byte/file budgets, disk failures, unexpected files, changed identity and incomplete process cleanup terminate rather than resetting task state for another candidate.
- These checks do not establish live SupJav/provider/CDN availability, Docker execution or target NAS operation. Application/browser/whole-repository acceptance is owned by the main agent and must use its own final results.
- No commit, deployment or task archival was performed by this implementer.
