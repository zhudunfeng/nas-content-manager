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
| `NAS_COLLECTION_WORKER_UAV_RUNTIME_MANIFEST` | Optional absolute private runtime manifest path; managed desktop supplies executable-adjacent `uav-runtime.json` |
| `UAVPythonExplicit` | Internal distinction between explicit input and platform default; not a browser argument |
| `NAS_COLLECTION_WORKER_YTDLP` | Existing fixed downloader path; managed desktop supplies absolute packaged executable |
| `NAS_COLLECTION_WORKER_FFMPEG` | Existing offline validator path; managed desktop supplies absolute packaged executable |

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
  supported plugin manifest exist. Presence selects a mode, not availability.
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
```
