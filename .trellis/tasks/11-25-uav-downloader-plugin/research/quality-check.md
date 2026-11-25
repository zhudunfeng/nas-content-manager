# UAV independent quality review

- Reviewer: child reviewer `/root/uav_quality`.
- Date: 2026-11-25.
- Scope: shared contract, app source/search/preview/task flow, worker bridge/download/process/network boundaries, Vue fixed adapter flow, pinned optional deployment dependencies, and all four supported sites (`jable`, `missav`, `hanime1`, `huangguo`).
- Method: loaded current task PRD/check specs, Trellis check/finish skills, collection/JMComic/cross-platform/frontend contracts; CodeGraph CLI located actual symbols before bounded source reads. All local commands ran through `python -B scripts/project_env.py -- ...`.

## Findings (fixed)

### Long Chinese UAV title rejected by application preview

- File: `internal/app/collection_preview.go`.
- Issue: UAV search accepts up to 1200 UTF-8 bytes, bridge limits source text, and worker `safeTitle` limits to 240 Unicode characters. A valid 200-character Chinese title is 600 bytes, but the app's inherited unconditional 512-byte preview limit rejected it with HTTP 502.
- Evidence: a temporary package-level regression test exercised the real `collectionCreatePreview` handler, real isolated SQLite migrations, real shared-root nonce verification and a controlled worker HTTP protocol. The 200-character title failed before the fix; four other boundary cases already behaved correctly.
- Fix: use the same bounded 1200-byte title budget for `uav` previews only. Every other source retains its 512-byte limit.
- Result: five handler subcases passed after the change: legal long Chinese title returns 200 unchanged; UAV 1201-byte title returns 502; existing web 600-byte title remains 502; web rejects UAV selectors with 400; UAV rejects JMComic album selector with 400.

## Directed temporary tests

Nine `TestQuality*` functions passed across `internal/app`, `internal/collectioncontract` and `internal/collectionworker`:

1. Legacy `OperationRequest` marshaling and SHA-256 are byte-for-byte equal to the pre-UAV field shape for web/local/remote/JMComic cases; new empty selectors are omitted.
2. Jable/MissAV/Hanime ID valid/invalid, traversal, URL, query, newline, full-width digits and maximum length cases.
3. Search Unicode character counts, trim/default page, unknown site, control characters, invalid UTF-8, lower/upper page bounds.
4. Search response source/query/page binding; unique IDs; count, title and duration bounds; invalid UTF-8/control characters.
5. Worker UAV selectors rejected for every other kind; UAV rejects arbitrary URL/source/path, album selector, audio/comic profile, invalid site/ID and excessive budgets.
6. `requestPreview` copies UAV selectors; site and video ID affect operation/preview hashes; short-lived preview ID does not affect operation hash; historical expected hash remains exact.
7. Fixed native downloader/config/plugin/JS/remote-component/playlist/fragment guards and bounded retries; original generic web formats remain HTTP/DASH only.
8. Media URL protocol, credentials, unsafe port, fragment and multiline rejection; legal HTTP(S) signed query accepted for actual proxy enforcement.
9. The five app handler title/isolation cases described above.

All three generated `uav_quality_tmp_test.go` files were deleted after successful execution as required by `AGENTS.md`. These tests were local contract/protocol evidence, not live-site or container acceptance.

## Verification executed by reviewer

- `go vet ./internal/collectioncontract ./internal/collectionworker ./internal/app`: passed in initial review.
- `npm --prefix web run typecheck`: passed in initial review and again after the title fix.
- `go test ./internal/app ./internal/collectioncontract ./internal/collectionworker -run TestQuality -v`: passed all nine temporary test functions, including four legacy-kind subcases and five real app-handler subcases.
- `go vet ./...`: passed after the title fix.
- `go test ./...`: passed after the title fix with temporary tests still present.
- Final `go test ./internal/collectionworker -run 'TestQualityHuangguo|TestQualityUAV' -v`: passed all 13 final temporary test functions, including 22 distinct actual downloader scenarios and the real complete/truncated TS comparison. The final run completed in 54.390 seconds; repeated earlier runs are not additional scenarios or assertions.
- Final actual pinned Python `scripts/verify_uav_bridge.py`: passed four-site search/resolve/empty, 24 bad-input/unsupported cases, two transport redirect cases, plus actual native HLS/AES acquisition and fatal missing/unsupported media cases.
- Final `go vet ./...` and `npm --prefix web run typecheck`: passed against the final production fixes.
- After removing the final generated test source, `go test ./...`: passed. The repository intentionally has no remaining Go test files after the user-required cleanup; this command is not substituted for the directed tests above.

## Review observations and evidence boundaries

- The four-site implementation correctly uses additive `omitempty` fields; private operation snapshots and `requestPreview` preserve site/video identity; task import remains fixed to video.
- App search rechecks source revision/enabled state after network I/O. UI ignores stale searches, locks selection during preview and uncertain submission, preserves pending UUID/body, and disables expired preview confirmation.
- The bridge's `EXT-X-GAP`, partial segment and nonpositive-duration rejection and explicit per-hop requests/curl redirect loops address the concrete completeness/response-body questions raised during the initial review.
- Parent reports separate actual app/worker/FFmpeg protocol-fixture suite and browser evidence; those are not counted as reviewer-executed commands here.
- No independent live public-site search/media transfer, Docker image build/runtime or target NAS throughput acceptance is claimed by this review.

## Huangguo and HLS findings (fixed)

The reviewer reproduced and reported the following issues; the implementation owner fixed the worker files, and the reviewer independently reran the final paths. This avoids concurrent edits to shared worker code.

### Incomplete upstream response could finish normally downstream

- File: `internal/collectionworker/uav_relay.go`.
- Issue: ignoring `io.Copy` errors could turn an aborted chunked upstream transfer into a normally completed downstream response. Unknown-length over-budget bodies also needed an observable failure.
- Fix: abort the downstream connection on upstream read/transfer failure or actual-byte overflow. Actual pinned yt-dlp rejects an aborted segment, a declared-length short response and missing fragments.

### AES-128 key length was more permissive than the protocol

- Files: `internal/collectionworker/uav_relay.go`, `internal/collectionworker/uav.go`.
- Issue: the pinned yt-dlp native HLS path accepts 24/32-byte AES keys for `METHOD=AES-128`; accepting its success alone does not enforce the required 16 bytes.
- Fix: every HLS download uses the finite resource relay. AES-128 key references are marked by the playlist tag rather than filename, read within 17 bytes and returned only for a complete 16-byte response. Actual 16-byte key downloads decode; 15/17/24/32-byte keys fail. A key without a `.key` suffix and with a cross-origin redirect still succeeds with a valid 16-byte key and no CDN header leakage.

### Delta and low-latency playlists could silently omit content

- File: `internal/collectionworker/uav_bridge.py`.
- Issue: checking ENDLIST/GAP/parts alone accepted `EXT-X-SKIP` and standalone low-latency metadata, which can omit historical fragments.
- Fix: use an explicit reviewed standard tag allowlist; reject delta, LL-HLS, unknown extensions, external audio, unsupported encryption and nonfinite/nonpositive durations. Canonical playlist serialization retains validated URI attributes for relay rewriting. HTTPS and Huangguo ordinary-TS restrictions remain enforced.

### Recognized Huangguo empty listing required nonexistent text

- Files: `internal/collectionworker/uav_bridge.py`, `scripts/verify_uav_bridge.py`.
- Issue: the upstream explicitly recognizes `<main><section class="hg-list-page"></section></main>` as an empty listing, but the extra bridge check demanded an empty-result phrase. The old fixture inserted such a phrase and masked the discrepancy.
- Fix: the Huangguo branch accepts its recognized empty listing with no drama cards. Unknown layout and malformed card cases remain errors; the reusable empty fixture now has no empty-result text. The final four-site bridge script passed after this fix.
- Recovery verification: disk exhaustion briefly interrupted a bridge edit. The implementation owner restored the full bridge from a previously built worker, compared it with the stable embedded source, and applied only the empty-layout fix. The reviewer independently ran the real bridge script after restoration. Final bridge SHA-256 is recorded in the evidence JSON; there is no claim that the failed write itself was successful.

### Ignored or mismatched byte ranges could duplicate fragments

- File: `internal/collectionworker/uav_relay.go`.
- Issue: with valid `EXT-X-BYTERANGE`, an upstream that ignored Range and returned HTTP 200 caused actual yt-dlp to exit successfully with a complete, repeated resource. Wrong offsets/lengths and unsolicited 206 responses also needed rejection.
- Fix: accept only a single explicit bounded `bytes=start-end`; require 206, matching Content-Range start/end and valid total, matching declared length when supplied, and exactly the requested number of actual bytes. No-Range requests reject unsolicited 206. Unsupported/malformed ranges fail before an outbound request.
- Additional concrete failure: for an overlong chunked 206, sending the full downstream Content-Length before noticing overflow still let actual yt-dlp finish successfully. The relay now withholds the last byte until upstream EOF/length validation. Correct chunked ranges succeed; short and long chunked ranges fail. Correct range output is compared byte-for-byte with the selected TS segment.

### Normal decoding did not detect a cleanly truncated TS tail

- File: `internal/collectionworker/uav.go`.
- Issue: an actual generated 8-second TS, reduced to its first quarter on a 188-byte packet boundary, still remuxed and decoded successfully with 49 frames and about 1.96 seconds. `progress=end` and a positive frame count prove the received video is decodable, not that it has the expected full duration.
- Fix: sum finite positive EXTINF values before relay rewriting; after local remux/full decode, require decoded duration to reach the HLS duration minus `max(0.5 seconds, expected * 2%)`. Retain the positive-frame/normal-end gate. The real 8-second fixture passes; its decodable truncated variant fails the duration gate. Exact tolerance thresholds, missing/negative/invalid/overflow time values and invalid manifest durations were checked.
- Limit: the tolerance is a lower-bound guard for obvious missing HLS content, not proof of media authenticity. Direct MP4 has no trusted expected duration in this integration, so its boundary remains transport completion, budgets and decodability. Parent synchronized this limitation and the tolerance into the UAV spec and user documentation.

## Final directed relay verification

The final 13 `TestQuality*` functions cover exact CDN origin/effective-port/case/extension scope, fixed resource registration and random operation isolation, per-hop redirect/header recomputation, incoming secret-header filtering, real proxy authentication, range validation before connection, cancellation/unregistration, truncated transfer and byte limits, actual public-dial private-address rejection, normal TLS certificate verification, decoded-frame gate, four-site canonical ID bounds, actual duration completeness and the downloader matrix below.

TLS fixtures replace only upstream dialing to an owned local TLS server and explicitly trust that fixture certificate with the expected SAN. A separate test with normal certificate verification confirms an untrusted certificate fails. This does not weaken production TLS or the public address policy. Private redirects are exercised with the actual public dialer.

The actual downloader uses the production `webArguments()` and `toolEnvironment(true)`, the real authenticated proxy, rewritten inline playlist, pinned yt-dlp and local FFmpeg. It has no explicit proxy credential on the command line, original CDN values in the downloader input, or network FFmpeg fallback. Successful outputs decode; byte-range outputs also equal the intended segment. Every case checks resource-scope cleanup.

| Distinct actual downloader scenario | Expected and observed |
| --- | --- |
| Normal TS | Success and local decode |
| Missing segment | Failure |
| Chunked segment abort | Failure |
| Declared Content-Length with short body | Failure |
| Unsolicited partial response | Failure |
| HTTP 201 / HTTP 204 | Failure in each case |
| AES-128 with 16-byte key | Success and local decode |
| AES keys of 15 / 17 / 24 / 32 bytes | Failure in each case |
| 16-byte key with no suffix and cross-origin redirect | Success, local decode, no cross-origin CDN headers |
| Correct explicit range | Success and exact segment bytes |
| Range ignored with HTTP 200 | Failure |
| Wrong range start / end / declared length | Failure in each case |
| Range response with short body | Failure |
| Correct chunked range | Success and exact segment bytes |
| Chunked range with short / long body | Failure in each case |

These are 22 distinct downloader scenarios: five successful cases and 17 deliberate failure cases. Their Go subtest pass events are evidence that expected outcomes matched, not a separate assertion count. The initial nine temporary contract/app functions and final 13 functions are separate review phases, totaling 22 directed functions; they were not all executed in one command.

## Evidence and cleanup

Reviewer evidence directory: `.runtime/verify/uav-independent-quality-20261125/`.

- `result.json`: final production source hashes, exact directed command, exit code and scope.
- `relay-tests.log`: complete final 13-function/22-downloader-case run, including whole/truncated TS cases.
- `uav_relay_quality_test.go.txt`: noncompiled copy of the final temporary test source for reproducing the directed checks. To reproduce locally, temporarily copy it to its original package test path, run the recorded command through `project_env.py`, then remove the generated test again.
- `bridge-tests.log`: independently rerun pinned parser/actual HLS script after the bridge restoration and empty-layout fix.
- `go-vet.log`, `frontend-typecheck.log`, `go-tests-after-cleanup.log`: final static checks and post-cleanup Go package verification.

The three initial generated `uav_quality_tmp_test.go` files and final `uav_relay_quality_tmp_test.go` were removed after passing. Only noncompiled evidence text and reusable integration scripts remain. No temporary unit-test source is left in the application packages.

## Findings (not fixed)

No confirmed in-scope code defects remain in this review. Real public-site availability/download, Docker runtime and target NAS performance remain separate acceptance boundaries; this review does not label them as executed tests. Parent owns the final rebuilt application/worker protocol and browser verification.

## Final quality gate

- Lint (`go vet ./...`): pass.
- TypeCheck (`vue-tsc --noEmit`): pass.
- Directed tests, real parser/HLS fixtures and cleanup verification: pass.
- Reviewer release to parent: final code is ready for the parent's one-time final build and protocol/browser checks; no further reviewer code edits are pending.
