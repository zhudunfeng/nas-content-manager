# UAV worker implementation and verification

Date: 2026-11-25. Scope: shared UAV contract, worker, private Python bridge, finite HLS relay, deployment dependency locks, third-party notice and reusable bridge verification.

## Implemented files

- `internal/collectioncontract/models.go`, `uav.go`: additive omitted selectors, shared bounded search DTO and four-site canonical identity rules.
- `internal/collectionworker/config.go`, `service.go`, `tools.go`, `proxy.go`: optional capability, authenticated search, fixed preview/acquisition dispatch and authenticated virtual-resource interception.
- `internal/collectionworker/uav.go`, `uav_bridge.py`, `uav_relay.go`: fixed parser protocol, real upstream search/detail parsing, private scoped CDN metadata, registered finite HLS resources, exact 16-byte AES-128 keys, native downloader and offline remux/full-frame decode.
- `Dockerfile.collection-worker`, `deploy/uav-requirements.txt`, `deploy/uav-build-requirements.txt`: separate Python 3.12 environment, fixed source archive and hashed headless dependencies.
- `docs/third-party-notices.md`: fixed upstream identity and Apache-2.0 license.
- `scripts/verify_uav_bridge.py`: reproducible real-package/real-downloader offline checks.

Main application, Vue UI, public docs/spec and final release regression are owned and reported by the parent task; they are not counted as worker-agent execution here.

## Fixed dependency identity

- UAV-Downloader 3.8.4, commit `d40f09457fbd38dd7dc94580637242afafaf3939`.
- Archive `https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/d40f09457fbd38dd7dc94580637242afafaf3939`.
- SHA-256 `88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`.
- Real local isolated environment: `.runtime/research/uav/venv`, Python **3.14.4**, yt-dlp **2026.10.26**. FFmpeg **N-122527-g4561fc5e48-20260122**.
- The production bridge requires Python >=3.12, distribution version 3.8.4 and matching `direct_url.json` archive hash. It imports only the audited headless package graph, without starting upstream GUI, queue, download or AI features.

Local environment creation and installation used the following form, through the required project environment wrapper:

```powershell
python -B scripts/project_env.py -- python -m venv .runtime/research/uav/venv
python -B scripts/project_env.py -- .runtime/research/uav/venv/Scripts/python.exe -m pip install --only-binary=:all: --no-deps --require-hashes -r deploy/uav-build-requirements.txt -r deploy/uav-requirements.txt
python -B scripts/project_env.py -- .runtime/research/uav/venv/Scripts/python.exe -m pip install --no-deps --no-build-isolation .runtime/research/uav/source.tar.gz
```

The downloaded local source archive was separately verified against the fixed SHA-256. The deployment docs use the same fixed HTTPS archive with a `#sha256=` fragment. Complete upstream metadata also declares GUI/AI dependencies deliberately absent from this headless adapter; this report does **not** claim a full upstream `pip check` pass.

## Executed evidence

### Real fixed package and real native HLS

Command:

```powershell
python -B scripts/project_env.py -- .runtime/research/uav/venv/Scripts/python.exe -B scripts/verify_uav_bridge.py
```

Final report: `.runtime/verify/uav-real-bridge-0b35ad1ba5f64af4893f6edcb59f20bf/result.json`.

- Four real upstream search parsers, four detail resolvers, four valid empty searches and 24 invalid-input/page/manifest branches passed. Only HTTP transport and deterministic public DNS were replaced; actual site sockets were forbidden.
- Huangguo searched actual series cards and returned observed EP1/EP3 only, with canonical per-episode IDs and actual next-page links. 101 observed episodes failed explicitly instead of truncating.
- Huangguo legitimate empty `<main><section class="hg-list-page"></section></main>` passed without invented empty text; unknown markup still failed.
- Anonymous playback POST sent `{}` without Cookie/Authorization, and did not follow its redirect. Identity mismatch and invalid flags failed.
- Original CDN master received scoped access headers; cross-origin selected variant did not. Returned CDN origin remained the initial source origin.
- Live/GAP/parts, DRM/keyformat, fMP4 for Huangguo, SKIP, server-control, part-info, preload, rendition-report, session-key, content-steering, external-media, unknown EXT tags and HTTP HLS segment were rejected.
- Actual locked yt-dlp used inline `hls_media_playlist_data`: normal and standard AES-128 fixtures exited 0; missing fragment and unsupported DRM exited 1. It did not fetch the supplied manifest URL or fall back to network FFmpeg.
- Actual FFmpeg decoded successful local output and reported `progress=end` with a positive decoded frame count.
- Both requests and curl transport replacements preserved `allow_redirects=False` and discarded intermediate redirect bodies from the final response body.

### Real native worker capability

Built `cmd/nas-collection-worker` and started an isolated owned worker with the actual trusted Python environment, actual yt-dlp and installed FFmpeg. Authenticated `/v1/capabilities` returned UAV available/version 3.8.4.

Report: `.runtime/research/uav/native-probe-a7febde519e642d0837378136bb8ddcc/result.json`.

This checks the production Go startup/process/probe boundary and real imports. It makes no public-site request. The parent final build embeds the final empty-page correction after this startup probe; that correction does not alter probe behavior.

### Linux wheel/hash coverage

Executed `pip download --only-binary=:all: --no-deps --require-hashes` against official PyPI for CPython 3.12 (`cp312` and `abi3`) with Linux manylinux2014/2.17/2.28 x86_64 and aarch64 platform targets.

- `.runtime/research/uav/wheels-linux-amd64`: all **20** locked wheels selected and hash-validated.
- `.runtime/research/uav/wheels-linux-arm64`: all **20** locked wheels selected and hash-validated.
- Each target includes 17 runtime and 3 build dependencies. Actual downloaded filenames include CP312 charset-normalizer/cffi, abi3 pycryptodome/curl-cffi and architecture-independent wheels.

This verifies wheel availability and exact hashes, not Linux execution or shared-library closure.

### Go and independent relay review

Worker-agent `gofmt`, `go vet ./internal/collectioncontract ./internal/collectionworker` and `go test ./internal/collectioncontract ./internal/collectionworker` passed. The latter included reviewer-owned temporary behavior tests; the contract package had no retained test files and is not described as independent behavioral coverage.

The independent reviewer additionally reported standard-TLS local fixtures and actual locked yt-dlp using production `webArguments`/`toolEnvironment(true)` through the actual authenticated public proxy, including scoped redirect headers, key-length boundaries, cancellation, deregistration, truncated/unknown-length responses and missing segments. Detailed reviewer assertions, command outcomes and cleanup are owned by `research/quality-check.md` and its final runtime report.

Further completeness fixes enforce a single explicit byte range, exact 206/Content-Range bounds and matching declared or actual payload length. A server ignoring Range is rejected, and an unexpected 206 without a Range request is rejected. HLS acquisition sums the original EXTINF durations, then requires decoded `out_time_us` to reach at least the expected duration minus `max(0.5 seconds, 2%)`. This blocks obvious cleanly truncated tails which still decode successfully; it is a lower-bound check, not a proof of the upstream original's full length. Direct media without a trusted expected duration has no such full-length guarantee. The reviewer owns the final real 8-second-versus-truncated-tail regression evidence.

## Remaining verification boundaries

- No live public-site search or authorized real media transfer was executed by this worker agent.
- Docker image build/runtime, Linux interpreter/shared-library execution and target NAS throughput are unverified. Cross-platform wheel selection and Go cross-compilation cannot substitute for those checks.
- Search is bounded to 100 actual results and a 22-second Huangguo expansion deadline. The complete upstream 5000-result browser behavior is intentionally unsupported.
- HLS supports only reviewed finite complete HTTPS resources and approved tags. Unsupported/new extensions fail explicitly; Huangguo fMP4 and separate audio remain unsupported.
- Byte-range responses must preserve the exact requested subset. HLS duration comparison detects obvious missing tails within the stated tolerance; it cannot prove semantic completeness or unchanged upstream content, and direct MP4 has no trusted expected duration in this adapter.
- Full application persistence/cancel/restart/import/browser and legacy regressions are recorded separately by the parent task.

All source comments/config explanations and sanitized worker logs follow the project Chinese-comment requirements. No worker-agent temporary unit-test source was retained; reviewer-owned temporary tests are removed by that reviewer after execution.
