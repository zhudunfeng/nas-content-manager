# Windows Native UAV Contract

## 1. Scope / Trigger

Applies to the optional Windows x64 UAV private runtime, fixed downloader materials,
desktop-managed collection worker and release schema 3. Supplements the
[UAV contract](uav-contract.md), [Windows JMComic contract](jmcomic-windows-contract.md)
and [cross-platform contract](cross-platform-contract.md). The existing five-site
identity, proxy, finite HLS, SupJav fallback/TS normalization, publication and import
contracts remain authoritative. No database or desktop-JSON migration is added.

## 2. Interfaces

```go
// Load 静态校验视频私有运行时并返回受信解释器身份。
uavruntime.Load(manifestPath string) (uavruntime.Runtime, error)
// CheckUAV 仅在自有临时目录探测固定视频依赖和本地工具。
collectionworker.CheckUAV(ctx context.Context, config Config, logger *slog.Logger) (collectioncontract.Capability, error)
// Resolve 只读解析当前编译配方对应的已发布依赖，不联网。
uavbootstrap.Resolve(root string) (uavbootstrap.Paths, error)
// Initialize 在已核验的collection-managed根中还原固定材料，真实离线探测后才发布。
uavbootstrap.Initialize(ctx context.Context, root string, report func(uavbootstrap.Progress), probe func(context.Context, uavbootstrap.Paths) error) (uavbootstrap.Paths, error)
```

Fixed CLI additions:

```text
nas-collection-worker.exe --check-uav
python -B scripts/uav_runtime.py assemble --output <new-directory>
python -B scripts/uav_runtime.py verify <manifest> --probe
python -B scripts/ytdlp_runtime.py assemble --materials-manifest <materials> --output <new-directory>
python -B scripts/ytdlp_runtime.py verify <manifest> --probe
python -B scripts/release.py --target windows-amd64 --uav-runtime-manifest <manifest> --ytdlp-manifest <manifest> --tools-manifest <tools>
```

All build/test commands use `python -B scripts/project_env.py -- ...`. A private
runtime is a build output, never a copied development venv or runtime pip install.

## 3. Runtime and provenance

- Separate `runtimes/uav/` and `licenses/uav/` from JMComic. Share only reviewed
  low-level file/path/PE helpers; keep plugin manifests and dependency contracts
  independent. Do not mutate module globals to reuse an assembler.
- `uav-runtime.json` is strict schema 1. Bind exact Windows amd64 target, Python
  patch, UAV 3.8.4 and fixed commit, runtime/build requirements hashes, source
  archive identity, derived wheel hash and all runtime/license files.
- Preserve fixed source SHA-256 in `uav-downloader`'s `direct_url.json`. Building
  and unpacking a wheel alone does not establish this provenance. Verify the
  original source first, normalize only to its audited official source identity,
  update/verify RECORD, and bind resulting bytes to the runtime inventory. Never
  loosen the existing bridge source check to accommodate packaging.
- Static validation covers bounded strict JSON, exact source/dependency closure,
  regular files and ancestors, traversal/ADS/device/case/trailing aliases,
  reparse points, architecture, required CA/native/package data and unexpected
  files within the owned runtime/license trees. Keep fixed isolated `._pth`.
- Normalize wheel `dist-info` directory spelling during extraction, not during
  manifest validation. PySocks 1.7.1 uses `PySocks-1.7.1.dist-info`; on Windows use
  an owned intermediate rename to obtain the required lowercase spelling and
  rewrite its RECORD. Leave importable module paths unchanged. Case-insensitive
  `Path` equality is not proof of canonical manifest bytes.
- Probe executes the actual private Python with `-I -B -X utf8`, fixed bridge and
  bounded JSON. Check real imports, fixed source identity, native extensions and
  CA independently from network availability. Standard CPython 3.14.4 x64 is the
  initial fixed candidate; static wheel matching is not execution evidence.

## 4. Configuration and isolation

| Field | Meaning |
| --- | --- |
| `NAS_COLLECTION_WORKER_UAV_ENABLED` | Existing opt-in; manual native default false, desktop derives controlled plugin enablement |
| `NAS_COLLECTION_WORKER_UAV_PYTHON` | Trusted explicit interpreter override; managed mode requires absolute path; invalid explicit configuration never falls back |
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | Optional absolute private runtime manifest path; managed desktop selects valid packaged or profile-published manifest |
| `UAVPythonExplicit` | Internal distinction between explicit input and platform default; not a browser argument |
| `NAS_COLLECTION_WORKER_YTDLP` | Existing fixed downloader path; managed desktop supplies an absolute packaged or verified profile executable |
| `NAS_COLLECTION_WORKER_FFMPEG` | Existing offline validator path; managed desktop supplies an absolute packaged or verified profile executable |

Interpreter resolution is explicit override, else manifest, else a fixed tool name
only in ordinary non-managed CLI mode. No managed PATH fallback. Specific local
dependency failure disables UAV independently, with Chinese sanitized reason/logs.
No query, source HTML, signed media URL, token, nonce or raw tool error is logged.

`--check-uav` neither serves HTTP nor initializes durable state or site requests.
Use owned temporary state and the same resolver as service startup. Check local
bridge, yt-dlp version and FFmpeg execution. Exit 0 means local ready, 2 means
disabled/unavailable, and 1 means diagnostic execution/cleanup failure.

## 5. Desktop behavior

- Reuse the one worker, private inherited pipe protocol, random loopback binding,
  authenticated capabilities/root challenge and Windows process ownership.
- New native profiles may choose managed when the adjacent worker and either
  supported plugin manifest exist, or UAV is explicitly enabled. Presence or
  opt-in selects a mode, not availability.
  Preserve old disabled/external settings and explicit external tuple precedence.
- Build child-only configuration for each configured plugin and absolute tools.
  Keep per-kind expected capabilities; do not report an absent optional package
  as corrupt. Show manga/video independently in status/tray. One unavailable
  plugin must not disable another plugin or basic media playback.
- At least one expected plugin available permits ready with partial reasons;
  all expected plugins unavailable means unavailable. Process/handshake failure
  means failed. Preserve existing state enums and fixed IPC control actions.
- Program files may be read-only when an explicit writable `--profile` is used.
  The default executable-adjacent `qiku-data` layout requires a writable parent.
  State/secrets/temp/logs use the owned profile.
  Quit, cancellation and upgrade wait for owned process and image handle release;
  never terminate by image name or stop an external worker.

### 5.1 Windows x64 automatic UAV initialization

- Trigger only for native + managed + expected/enabled UAV with no explicit
  `UAV_PYTHON`. Explicit false, saved disabled/external and ordinary external
  execution perform zero installation. Generic collection-enable does not
  choose an otherwise unconfigured video plugin. Invalid custom Python never
  falls back or installs. JMComic uses the same shared safety engine and desktop
  preparation batch with its own independent inventory; see the
  [JMComic automatic preparation contract](jmcomic-windows-contract.md#automatic-private-dependency-preparation).
- Prefer statically valid packaged runtime/tools, then `uavbootstrap.Resolve`
  from the owned profile. Actual worker capability is still authoritative.
  Start background initialization only after basic media reports ready and
  the real worker reports UAV unavailable. Downloads must not block media
  startup. Failure is attempted once per start; user recheck/next launch retries.
- The caller holds the profile instance lease and prepares the existing private
  `collection-managed` root. Installer owns only its fixed `uav-bootstrap`
  namespace with exact owner marker. Reject unknown existing directories,
  UNC/reparse/alias paths, and undeclared files. Never install into program root,
  global Python, PATH, registry or user-level environment.
- Embed `assets/runtime.json`, `recipe.json`, `payload.zip`. Recipe schema 1 binds
  runtime/payload SHA256, 20 fixed HTTPS artifacts with exact sizes/hashes/member
  mappings, and seven extra tool/license file records. Payload consists of
  verified installed UAV code, licenses and normalized metadata/._pth overlays;
  it is not the original wheel. Keep original `builtWheelSha256` provenance.
  JSON resource bytes use LF through scoped `.gitattributes` rules. Generation
  includes exact runtime bytes and `bootstrap-receipt.json` recipe bytes.
- Native Go downloads use standard TLS, inherited process proxy, finite time/
  size budgets, cancellation, and a narrow GitHub release CDN redirect list.
  Retry transient network/408/429/5xx at most three times with cancellable 1s/3s
  waits; never retry integrity/source/cleanup failures. Check the final native
  file path UTF-16 budget before downloads and in Resolve: at
  most 248 units, reserving loader overhead rather than assuming `<260` is
  sufficient. Real curl_cffi imports failed at 251–260 units in Windows
  reproduction. Reject with a Chinese shorter-profile instruction.
  Match exact SHA256 and length. ZIPs reject traversal, ADS/device/case aliases,
  duplicates, links, encryption, oversized members/expansion and CRC failures.
  Extract only compile-time mapped members; do not execute pip/download scripts.
- Stage under random `stage-<32hex>`; validate full file closure, tool PE/size/
  hashes and unchanged `uavruntime.Load`, then rename to immutable short
  `gen-<32hex>` and validate again. Run a desktop-owned Job-managed worker
  `--check-uav` (60s cap) at this exact final location before atomically replacing
  bounded strict `current.json`. A successful staging probe never proves final
  Windows native DLL readiness; retain all 32 hex identity characters.
  Current schema 1 has exactly schemaVersion, recipeSha256, generation. Failed
  stages never replace current. Only delete files owned by this exact attempt;
  cleanup errors are `ErrCleanup` and block unsafe restart/replacement.
- Completion is consumed by the controller serial loop after the fixed
  JMComic→UAV batch, which performs at most one ordered core/worker restart for
  successful results. Ordinary failure in one plugin does not block the other;
  cancellation or cleanup failure stops the batch. Do not mutate lifecycle fields
  from a background goroutine or claim uninterrupted media activation. Do not
  reopen the browser during automatic activation. Stop/recheck/disable/quit and
  unexpected core/worker exit cancel and wait; stale results cannot re-enable
  disabled UAV. Ordinary network failure only degrades video capability.
- IPC retains optional `collectionInitialization`: kind=`jmcomic|uav`, state=
  `initializing|ready|failed`, sanitized Chinese stage, downloadedBytes, totalBytes.
- `collectionInitializations` additionally preserves per-kind states for a
  multi-plugin batch. Counts represent the current material, never fabricated
  aggregate percentages. Tray and capability reasons use matching kind. A ready initializer state never
  substitutes for worker `Capability.Available`.
- Build generation: `scripts/uav_bootstrap.py --runtime-manifest <verified>`;
  `--check` regenerates and byte-compares assets. Reusable online integration:
  `scripts/verify_uav_bootstrap.py`. Local negative harnesses explicitly block
  child download proxies for deterministic offline assertions. Retain separate
  offline complete-package release audits; automatic local acquisition does not
  assert corresponding-source/redistribution review or mutate release identity.

## 6. Downloader and release identity

- The initial standalone choice is official yt-dlp 2026.10.26 x64 EXE. Its fixed
  EXE hash is distinct from the existing wheel hash. Do not insert `python -m`
  into a single executable string or copy an absolute-path console launcher.
- Downloader manifest lives under `licenses/yt-dlp/`, independently of the existing
  FFmpeg tools manifest. Bind executable/version/architecture, official materials
  and every license/source inventory member. Missing materials fail assembly.
  A machine check or inventory declaration does not replace human distribution
  review of GPLv3+/third-party/corresponding-source obligations.
- Release schema 3 keeps schema-2 fields and adds nullable `uavRuntime` and `ytDlp`.
  UAV summary is exactly `{manifest,sha256,pythonVersion,uavVersion,uavCommit}`;
  downloader summary exactly `{manifest,sha256,version}`. Verify each summary
  against the actual manifest and archive members. Read old schema 1/2 with their
  original exact field sets. Image manifest schema is a separate version domain.
- Legacy schema 1/2 complete packages may retain extra executables declared and
  verified by their original tools inventory, including `tools/yt-dlp.exe`.
  Apply the separate downloader-material contract to schema 3; do not reject
  previously valid archives retroactively. Undeclared old tools still fail the
  original inventory closure, and legacy development packages gain no exception.
- Full UAV distribution requires UAV, downloader and media tools together. Basic,
  JMComic-only and explicitly incomplete development packages retain accurate
  metadata and notices. Never claim fixture binaries or missing-license outputs
  as complete distributable packages.

## 7. Required verification

1. Static manifest adversarial checks in Python and Go, source/provenance/RECORD
   mismatch, extra/missing files, native architecture/CA, explicit override rules
   and diagnostic exit codes. Keep reusable integration harnesses.
2. Actual embedded imports/AES/CA, isolated paths and polluted Python environment;
   fixed actual downloader version and local finite HLS/SupJav regressions.
3. Desktop UAV-only, JMComic-only, combination and partial failure; old profile,
   external priority, Chinese/space/different-CWD/read-only paths; cancellation,
   quit/crash/restart and executable/DLL release without collateral termination.
4. Release old/new schema, null/non-null combinations, material closure and actual
   archive validation; PowerShell 5.1/7 parsing/forwarding; whole-repo Go vet/build,
   frontend checks, existing collection/JMComic/UAV and core media regressions.
5. Record clean Windows, live site, Linux/Docker and ARM64 separately. Cross-builds,
   offline fixtures and this developer machine cannot satisfy those acceptance
   categories. Delete only newly generated temporary unit-test sources after runs.
6. For automatic initialization, use the actual packaged desktop and worker with
   a read-only program directory, Chinese/space profile, foreign CWD and polluted
   Python environment. Assert 20 fixed artifacts, real final-path probe, actual
   capability ready, H.264 encode/probe/decode and post-quit image release.
   Restart with a failing proxy and assert every published byte/mtime/current is
   unchanged. Inject final-probe failure/cancellation and retain the old selection.

### Desktop harness prerequisites

- Give `--check-uav` a separate bounded 60-second harness timeout: its two
  10-second tool probes and 15-second bridge probe do not fit the short IPC
  client's budget. Keep production probe deadlines unchanged. Allow 30 seconds
  for the nested fixture marker because video tool checks precede Python startup.
- Verify a genuinely read-only owned program directory while preserving execute
  access. On Windows deny only inherited `(WD,AD,WEA,WA)` and assert that a new
  file cannot be written; restore that exact temporary deny ACE in `finally`.
  Generic `(W)` also denies `SYNCHRONIZE` and can falsely block `CreateProcess`.
- Import only packages present in each locked runtime. The UAV lifecycle fixture
  exercises real UAV/SSL, curl and AES; it must not require an unrelated `lxml`
  installation. Observe mapped native images through PSAPI and verify the owned
  process subtree, independent sentinel survival and post-exit exclusive access.
- A standalone fixture preflight establishes harness readiness only. Desktop
  cancellation/parent-crash, nested Job cleanup and same-profile restart must
  still run through the actual desktop and worker before being counted as passed.

## 8. Validation and error matrix

| Condition | Required behavior |
| --- | --- |
| UAV disabled | Diagnostic exit 2; no site request, runtime extraction or durable state |
| Enabled native/managed UAV, no explicit Python, unavailable worker capability | Initialize in background only after media ready; final probe succeeds before atomic publication and serial activation |
| Disabled/external mode or explicit custom Python | Zero automatic installation; an invalid override still never falls back |
| Final native path exceeds248 UTF-16 units | Refuse before download and give sanitized Chinese shorter-profile instruction |
| Download/integrity/final-probe failure or cancellation | No half-published selection; clean only this attempt, preserve old current; cleanup failure blocks unsafe restart |
| Invalid diagnostic boolean or temporary-directory/cleanup failure | Exit 1 with sanitized Chinese failure |
| Managed explicit Python is relative, absent or invalid | UAV unavailable; do not fall back to manifest/PATH; another ready plugin remains usable |
| Manifest/source/RECORD/native architecture or CA check fails | UAV unavailable with local integrity reason, worker and basic media remain available |
| Wrong yt-dlp version or unavailable media tool | UAV unavailable with the corresponding local tool reason; preserve ordinary web adapter compatibility |
| One of the expected plugin kinds is ready | Overall ready, with unavailable reasons and per-kind capability snapshots |
| No expected plugin kind is ready | Overall unavailable; a process/handshake failure remains failed |
| Optional plugin has no configuration | Omit it from IPC `collectionCapabilities`; tray shows unconfigured rather than corrupt |
| Old release schema receives a new field, nullable field is omitted, or summary mismatches members | Reject archive even if its outer SHA-256 was recomputed |
| UAV release input lacks downloader/media materials | Reject assembly; never label it a complete video package |

## 9. Good, base and bad cases

- Good: an offline-assembled UAV runtime and an independent JMComic runtime share
  one worker; real imports and matching local tools produce two ready capabilities.
  Breaking one explicit interpreter disables only its own plugin.
  Alternatively, enabled native/managed UAV without packaged files prepares fixed
  dependencies once, then reuses the verified profile generation offline.
- Base: a media-only package has no optional runtime. Its existing media and
  disabled/external settings remain usable, and status does not invent an expected
  video capability. A UAV-only package does not require JMComic files.
- Bad: copying a development venv, omitting `direct_url.json`, changing provenance
  to the derived wheel hash, or treating a successful metadata checksum as a
  native-dependency/clean-machine probe. The source/archive validators must fail
  these identities rather than loosening the bridge probe.

## 10. Wrong versus correct

```text
Wrong: NAS_COLLECTION_WORKER_YTDLP="python -m yt_dlp"
       (a command string is not a standalone executable path)
Correct: NAS_COLLECTION_WORKER_YTDLP=<absolute-package-root>/tools/yt-dlp.exe
         (fixed official EXE, separately reviewed materials, exact UAV version probe)

Wrong: reuse a JMComic assembler by monkeypatching its module globals;
       flatten both plugins into one site-packages directory.
Correct: share bounded low-level helpers in runtime_common/privateruntime,
         retain separate manifests, dependency/source contracts and directories.

Wrong: run the real Python probe in stage-<id>, rename to a longer directory,
       then publish current.json based only on `<260` canonical file paths.
Correct: reserve native loader path overhead, use short gen-<32hex>, revalidate
         and actually probe that final location before publishing current.json.
```
