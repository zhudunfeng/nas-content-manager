# Regression baseline contract

## 1. Scope / Trigger

Applies to `scripts/verify_baseline.py`, regression orchestration, and target NAS acceptance records. The orchestrator composes existing `verify*.py` programs; it does not change business APIs or operate a user instance. Keep the original verifiers independently usable.

## 2. Signatures

Always use the repository environment runner:

```text
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite local --artifacts .runtime/bin --cases core --ffmpeg <absolute-path> --ffprobe <absolute-path>
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --suite target --target-file docs/nas-acceptance.template.json
python -B scripts/project_env.py -- python -B scripts/verify_baseline.py --self-test
```

Local cases are `core,account,downloads,search,engines,external,components,collection,release`; the default is `core`. Optional controls are `--root`, `--port`, `--timeout`, `--startup-timeout`, and `--large-file`. The artifact directory supplies `nas[.exe]` and, for collection, `nas-collection-worker[.exe]`. Core calls `verify.py --base-url ... --root ...`; that verifier has no `--binary` parameter. Account calls `verify_account.py --binary ... --root ... --offline` with historical schema fixtures, actual account requests and the isolated offline-maintenance verifier; the standalone account script without `--offline` remains online-only. Collection also needs `--worker-binary`. Release invokes only the existing release self-test.

## 3. Contracts

- Every run creates a new descendant of repository `.runtime/verify/`. Reject an existing root, `..`, links, junctions/reparse ancestors, and paths outside this root before writing anything. Generated run IDs combine UTC time and randomness. Do not remove prior evidence.
- Core requires built frontend assets and explicitly selected FFmpeg/ffprobe. Give its service fresh data/media/web directories, clear inherited `NAS_*` configuration, and enable only this isolated instance. Do not connect inherited helpers, workers, download engines, or external sources.
- Core owns its application process. Readiness requires that process to remain alive, a successful health response, and an exact randomly named static probe from the run's own frontend directory. An HTTP 200 or free-port probe alone cannot identify a service. An occupied explicit port is blocked; never stop its owner.
- Launch commands as argument arrays without a shell. On Windows, place the suspended child in an owned kill-on-close Job before resuming it; fail closed if ownership cannot be established. On POSIX, use an owned session/process group. Normal completion, timeout, exceptions, and interrupts must reclaim owned descendants and collect process handles/pipes. Do not kill by executable name, arbitrary PID, or global port.
- Bound child runtime, startup wait, and stored output. Independent cases may continue after another fails. A cleanup failure makes the affected case fail; it cannot be hidden behind a successful child exit.
- Persist `run-manifest.json` atomically during execution and at completion. Its v1 fields include run/suite/time/status, source commit/dirty state, platform, tool versions, artifact digests, bounded configuration, cases, and limitations. Case data includes selection/status/reason, assertion count, evidence kind, command/exit/timeout/duration/log, and evidence paths relative to the run root.
- Use only `passed`, `failed`, `blocked`, and `not_run`. A case passes only when its real verifier exits zero and its expected result/positive assertion count is present. A missing result cannot become passed merely because the process returned zero. Never label an incomplete run as passed.
- Reports and logs redact secrets, authentication headers/cookies, private URLs, and absolute paths. Private generated fixture material is local restricted input, not shareable evidence. Do not record password/token values in commands or manifests.
- Redact structured JSON recursively before serialization. Include UNC, POSIX roots, and paths with spaces; never run a text regular expression over serialized JSON. Secondary fixture-log traversal must prune links and Windows reparse directories at every level.
- Error precedence is cleanup failure (`failed`), safe-launch prerequisite failure (`blocked`), then command/result validation. Retain command exit/log evidence when finalization fails. Every cleanup operation, including self-test link removal, must preserve final manifest publication. CLI root validation applies equally to normal and self-test entry points and rejects Windows device/ADS/trailing-dot/space aliases before creation.
- Target v1 validates the explicit acceptance record only. It does not log in, install/stop anything, change production state, or import claimed passing measurements. Unknown hardware and unexecuted M1/M2/M3 gates stay `null`, `blocked`, or `not_run` as appropriate. Template validity is separate from device acceptance.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| All selected cases ran with complete passing assertions | Overall `passed`, exit 0 |
| Verifier failure/nonzero exit/timeout, missing result, or cleanup failure | Case `failed`; overall exit 1 |
| Required artifact/tool/frontend missing, occupied port, unsupported safe process ownership | Case `blocked`; exit 2 unless another case failed |
| Case not selected | `not_run`, no passing credit |
| No target device evidence | Device milestones remain `blocked`/`not_run`; no acceptance claim |
| Root exists, escapes, or traverses a link/reparse point | Exit 2 before writing into that root |
| Target record invalid | Explicit validation failure; do not touch any target |

## 5. Good / Base / Bad Cases

- Good: run core twice against the same build in separate generated directories, then compare assertion counts and artifact SHA-256 values. Each instance owns its distinct data and port.
- Base: choose one affected domain and invoke its existing verifier with the actual required arguments. Store its real assertions and label its evidence accurately.
- Bad: accept a foreign `/healthz` response, overwrite an earlier run, use the ordinary server executable for a desktop-only verifier, or call a missing dependency a skip/pass.
- A 5 GiB sparse Range fixture proves HTTP offsets and lifecycle only. It does not prove real codec decoding, seek latency, throughput, or target NAS memory/CPU usage. Protocol fixtures, actual local application/tool runs, historical records, and real device/product evidence must remain distinct.

## 6. Tests Required

- Retained orchestration self-test: missing artifacts, repeated/outside/link roots, occupied port with its owner preserved, failed child, timeout, inherited-pipe descendant cleanup, independent sentinel survival, secret redaction, and distinct repeated runs. Test actual process ownership rather than mocking a successful kill.
- A filesystem heartbeat written with truncation can be temporarily empty. Prove observer survival with bounded polling for a nonempty valid changed tick and a live owned-process handle; a fixed sleep followed by one read is not a reliable liveness assertion. Keep descendant-stop assertions and process-tree ownership checks intact.
- Execute public CLI entry points for invalid/existing roots; helper-only assertions cannot prove dispatch/error handling. Inject finalization errors and check both the failed status and preserved command/evidence records. Exercise tool-version cleanup failures separately from ordinary command failures.
- Two actual core runs: authenticated five-media access, scanning and personal state, and SQLite snapshot assertions from the original verifier. Record artifact identity and real evidence paths.
- Domain changes additionally select the corresponding existing verifier. Collection requires both executables. Fixture checks do not authorize or demonstrate real Docker/product deployment.
- Account requires `accountMutationAPIs=true`, `trueSchema4Migration=true`, `offlineCLI=true` and `offlineVerification.requested/passed=true`. `onlineAssertions` and `offlineAssertions` are positive integers; their sum must equal the total, and the nested offline count must match. Missing, false or inconsistent coverage fails even if the child exits zero. Complete and incomplete orchestration fixtures must traverse the actual `domain_case` dispatch and JSON read-back; helper-only negative tests cannot detect a dropped gate call. Rejections retain the command but publish no passing count or success evidence. Label every fixture and its saved evidence separately from actual application tests.
- Account uses `local-real-application`, `historical-schema-fixtures` and `local-offline-maintenance` evidence kinds rather than claiming an external protocol service. Actual console and OS coverage comes from the same-run child result, never from the case name. The separate account browser verifier is explicit, not automatically launched by baseline; its actual backend and intercepted presentation counts remain separate.
- Target record: OS/architecture, CPU/memory/disks/filesystems, UID/GID/ACL, mounts, Docker/Compose mode and image digest, real samples and clients, plus owners/inputs for pending M1/M2/M3 gates. Preserve unverified Linux permissions/process behavior separately from cross-compilation.
- Run Go vet/package compilation and frontend typecheck/build for the initial baseline; an empty retained Go unit suite is compilation evidence only. Remove only newly generated temporary unit-test sources, preserving reusable integration scripts. Record actual results in `docs/verification.md`.

## 7. Wrong vs Correct

```text
Wrong: HTTP 200 on a chosen port -> assume this is our service -> run destructive fixture setup.
Correct: own the fresh process/data root -> health + exact random static probe -> run isolated verifier.

Wrong: process.wait() succeeded -> ignore its inherited-pipe descendants; taskkill by image on timeout.
Correct: establish Job/process-group ownership before execution -> drain bounded output -> reclaim only owned descendants -> collect handles and report cleanup failures.

Wrong: target template parses -> M1 passed; missing binary -> passed with zero assertions.
Correct: template validation has its own evidence; target gates stay unverified; missing binary is blocked.
```
