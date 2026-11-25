# JMComic backend validation

- Date: 2026-11-25
- Scope: fixed Go app/worker adapter, real installed JMComic base dependency, embedded Python protocol, cross-platform dependency lock.
- User media and existing user processes were not used. All binaries, venvs, wheel downloads and evidence are under repository `.runtime/`.

## Implementation evidence

- Optional native worker settings: `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED=false`, `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON=python3` (`python` on Windows). Enable flag accepts exactly `true` or `false`.
- Docker worker creates separate `/opt/jmcomic` venv, installs `deploy/jmcomic-requirements.txt` with `--only-binary=:all: --no-deps --require-hashes`, checks dependencies and includes `docs/third-party-notices.md`. Image defaults enable the installed fixed adapter.
- Fixed adapter is embedded in worker binary and called as trusted Python `-I -B -c <embedded source>` with JSON stdin. Requests cannot select script, YAML, account, URL, output root or process arguments.
- Search/preview/download reuse existing worker auth, shared concurrency, owned process-tree cancellation, controlled public proxy, task-owned staging, disk/file/byte budgets, explicit publication and app-side immutable manifest validation. No schema migration; new request fields use `omitempty` to preserve old JSON/hash contracts.
- Full album ID, ordered chapter IDs and declared image count bind download to the original preview. Completion requires upstream `all_success`, no failed lists, matching chapter/page counts, actual file count, decoded image inspection and independent Go image header/dimension checks. Directory image comics reuse existing chapter aggregation at import.

## Executed checks

All commands ran through `python -B scripts/project_env.py -- ...`.

1. Native Windows Python 3.14.4 venv `.runtime/tools/jmcomic-venv` installed exact base dependency versions and then reinstalled using the complete production hash lock:

   ```powershell
   python -B scripts/project_env.py -- .runtime/tools/jmcomic-venv/Scripts/python.exe -m pip install --force-reinstall --only-binary=:all: --no-deps --require-hashes -r deploy/jmcomic-requirements.txt
   python -B scripts/project_env.py -- .runtime/tools/jmcomic-venv/Scripts/python.exe -m pip check
   ```

   Passed. Closure: jmcomic 2.7.12, curl-cffi 0.17.0, pillow 13.0.0, pycryptodome 4.6.0, pyyaml 6.0.4, cffi 2.1.1, certifi 2026.11.20, pycparser 3.12. No upstream `[plugins]` extras installed.

2. Official PyPI wheel metadata supplied SHA256 values for accepted CPython 3.11–3.14 Linux amd64/arm64 and Windows amd64 wheels. Linux Python 3.11 platform resolution and every selected artifact hash were exercised with:

   ```powershell
   python -B scripts/project_env.py -- .runtime/tools/jmcomic-venv/Scripts/python.exe -m pip download --platform manylinux_2_28_x86_64 --platform manylinux2014_x86_64 --python-version 3.11 --implementation cp --abi cp311 --only-binary=:all: --no-deps --require-hashes -r deploy/jmcomic-requirements.txt --dest .runtime/verify/jmcomic-wheels/linux-amd64
   python -B scripts/project_env.py -- .runtime/tools/jmcomic-venv/Scripts/python.exe -m pip download --platform manylinux_2_28_aarch64 --platform manylinux2014_aarch64 --python-version 3.11 --implementation cp --abi cp311 --only-binary=:all: --no-deps --require-hashes -r deploy/jmcomic-requirements.txt --dest .runtime/verify/jmcomic-wheels/linux-arm64
   ```

   Both passed for all eight packages. This proves compatible wheel selection/hash availability, not Linux execution or Docker image acceptance.

3. Reusable real dependency/adapter check:

   ```powershell
   python -B scripts/project_env.py -- .runtime/tools/jmcomic-venv/Scripts/python.exe scripts/verify_jmcomic_bridge.py
   ```

   Passed. A real isolated subprocess first imports the actual installed dependency and executes the unchanged production bridge probe using `-I -B -c` and JSON stdin/stdout. Remaining tests execute the same bridge with actual `JmOption.construct`, client construction, `JmApiClient.search_site/search`, `JmSearchPage`, `JmDownloader`, callbacks, `JmImageResp.transfer_to` and `JmImageTool.decode_and_save`. Fixtures use real upstream album/photo/image entity classes and real generated raster bytes.

   Explicit replacements: external domain discovery disabled/fixed; album/photo metadata responses replaced; search API response replaced (including `/setting` cookie bootstrap); image HTTP response replaced. Real curl and socket connection entry points fail on any attempted connection. Zero real network calls occurred. This fixture does not prove real source access, proxy socket behavior or upstream service availability.

   Covered: correct version/import probe; ordinary search pagination; zero results; numeric-query redirect; ordered two-chapter album; four actual output images (three decoded JPEGs, one preserved GIF); upstream image indices and GIF property; fixed numeric paths despite unsafe source titles; partial image failure rejection; both file-count and byte budget rejection; safe JSON error output from an actual partially failed download; explicit client proxy config equals worker-generated proxy.

   Evidence: `.runtime/verify/jmcomic-real-bridge-17e05a1c7d4f4db8befe6948a2a028e4/report.json`. Each run creates a new isolated evidence directory.

4. Temporary Go application regression exercised normal image-page validation for comic, rejection for video and actual directory import-target aggregation. Passed, then generated test file `internal/app/jmcomic_directory_temporary_test.go` was removed per AGENTS.md.

5. `go vet ./...`, `go test ./...`, `go build ./cmd/nas`, `go build ./cmd/nas-collection-worker` passed. Working verification binaries are `.runtime/bin/jmcomic-final/nas.exe` and `.runtime/bin/jmcomic-final/nas-collection-worker.exe` (subsequent source-only comment/script changes do not change runtime logic).

6. Separate protocol integration agent ran 66 app/worker checks using controlled executable fixtures: actual PNG publication, authenticated four-page reading, idempotency, failure/retry, incomplete/malformed outputs, cancellation after actual process startup, app/worker restart recovery, dependency/plugin/worker unavailability preserving existing reads. Evidence: `.runtime/verify/jmcomic-protocol-20261125-2/result.json`. This fixture exercises Go protocol/publication/import paths; it is separate from the real dependency adapter check above.

7. Real native Go worker executable `.runtime/bin/jmcomic-final/nas-collection-worker.exe` was started against the real Windows venv interpreter in a new exclusive temporary root. Its actual `runTool` scrubbed environment and embedded `-I -B -c` probe completed; authenticated `GET /v1/capabilities` returned `jmcomic.available=true, version=2.7.12`. Only local health/capability endpoints and the import-only probe were invoked, with no search/download requests. The owned idle worker was terminated after read-back. Evidence: `.runtime/verify/jmcomic-real-worker-43212beeaf024b18993d34713ff98372/capabilities.json`; command `python -B scripts/project_env.py -- python .runtime/verify/jmcomic_real_worker_probe.py`.

8. Real installed `curl-cffi==0.17.0` was executed with `-I -B -c` through the production Go `runTool` scrubbed environment and actual authenticated `newPublicProxy`. This temporary package-level test used two independent exclusive loopback sentinel ports and made no public network or source requests:

   ```powershell
   python -B scripts/project_env.py -- go test ./internal/collectionworker -run TestJMComicRealCurlProxyTemporary -v
   ```

   Passed. Explicit HTTP and HTTPS proxy configuration produced HTTP403 / CONNECT403 for the nonstandard sentinel ports and both sentinels observed zero connections. This proves actual curl traffic reaches the controlled proxy and cannot bypass the port allowlist; the sentinel rejection alone is **not** isolated IP-policy evidence. Separate actual curl requests to loopback standard HTTP80 / HTTPS443 produced the expected proxy HTTP502 / CONNECT502 after the production IP policy rejected the destination. No standard-port sentinel was bound because existing local ownership/reservations prevented a safe bind; no existing service was stopped or contacted directly. Six direct production `publicDial` checks returned exactly `destination rejected` without a connection for loopback80/443, RFC1918, metadata, IPv6 loopback and IPv4-mapped loopback addresses. Both sentinel listeners, the proxy and Python subprocess were owned by the test and closed on completion. Generated `internal/collectionworker/jmcomic_real_proxy_temporary_test.go` was removed per AGENTS.md. Evidence: `.runtime/verify/jmcomic-real-proxy-20261125T012540798742400/report.json`.

## Boundaries still requiring external acceptance

- No real comic search/download was performed, and no claim is made about source availability, accounts, domain rotation or site policy.
- Linux packages were downloaded and hash-verified, but no cross-build, Docker build, Linux import/execution, actual NAS deployment or NAS performance test was run by this backend implementation agent.
- Actual curl-cffi HTTP/HTTPS private-destination denial was checked as described above. Live public redirects, CDN fallback, upstream domain discovery and site availability were not exercised; the real dependency bridge fixture separately verifies that fixed client/bootstrap configuration uses the same proxy.
