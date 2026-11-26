# Windows Native JMComic Contract

## 1. Scope / Trigger

This contract governs the optional Windows x64 private Python distribution and
desktop-owned collection worker. It supplements the [JMComic contract](./jmcomic-contract.md)
and [cross-platform contract](./cross-platform-contract.md). Search, immutable
previews, selected chapters, publication, progress and explicit retry retain their
existing contracts. Application SQLite remains schema 6; desktop JSON schema is
independent. Local Windows verification does not imply clean-machine, live-site,
Linux runtime or ARM64 acceptance.

## 2. Signatures

```go
// Load 静态校验运行包身份，不执行 Python。
jmcomicruntime.Load(manifestPath string) (jmcomicruntime.Runtime, error)
// RunManaged 通过私有继承的标准输入输出管道接收控制消息。
collectionworker.RunManaged(ctx context.Context, config Config, logger *slog.Logger, input io.ReadCloser, output io.Writer) error
// CheckJMComic 仅在自有临时目录执行固定的本地依赖探测。
collectionworker.CheckJMComic(ctx context.Context, config Config, logger *slog.Logger) (collectioncontract.Capability, error)
```

Fixed command interfaces:

```text
nas-collection-worker.exe --managed-stdio
nas-collection-worker.exe --check-jmcomic
栖库.exe --collection-mode disabled|managed|external
栖库.exe --control collection-enable|collection-disable|collection-check
python -B scripts/jmcomic_runtime.py assemble --output <new-directory>
python -B scripts/jmcomic_runtime.py verify <manifest> --probe
python -B scripts/release.py --target windows-amd64 --jmcomic-runtime-manifest <manifest>
```

Use the repository environment entry for build/test commands. No request installs
dependencies, accepts a script/command or selects a Python argument.

## 3. Contracts

### Desktop ownership and migration

- Desktop schema 2 requires the original nine exact non-null fields plus
  `collectionMode: "disabled"|"managed"|"external"`. Strict JSON, absolute stored
  roots, stable identity, atomic replacement and all existing validation remain.
- Read valid schema 1 without changing identity or business roots. Derive
  `external` only when an external connection tuple was explicitly configured;
  otherwise migrate to `disabled`, even when the new program contains a runtime.
  Before the first persisted migration, preserve the exact original bytes as
  `desktop.v1.json`; refuse to overwrite a different backup. `--print-config`
  must remain read-only.
- New native profiles prefer explicit external configuration; otherwise select
  `managed` only when the adjacent worker and manifest are present. CLI overrides
  `NAS_DESKTOP_COLLECTION_MODE`, which overrides saved mode. Docker cannot use
  native `managed` mode. Incomplete explicit external tuples must produce their
  original validation error, not be silently replaced by a managed connection.
- Dedicated state is `<profile>/collection-managed/{owner.json,data,secrets,tmp}`;
  worker logs are `<profile>/logs/collection-worker.log`. Unknown existing roots,
  mismatched owners, reparse aliases and remote state are not adopted. Apply a
  private Windows DACL only to newly created or verified instance-owned roots;
  do not reset unrelated user directory permissions.
- Managed mode builds a child-only environment and clears inherited app worker
  URL/token/read-root/HTTP settings as a group before injecting its verified
  connection. Disabled mode clears that whole tuple. External mode keeps it and
  never owns or stops the external process. Do not mutate global environment.

### Private control protocol

The parent owns two inherited anonymous pipes. Child stdout contains only one
strict versioned JSON ready message; stderr contains human diagnostic logs. This
is not readiness detection by parsing logs or by probing an unreserved port.

| Action | Exact fields |
| --- | --- |
| `start` / `stop` | `version:1`, `action`, `nonce`, `instanceId` |
| `ready` | those fields plus `address`, `readRoot` |

Each message is one newline-terminated JSON object, at most 16 KiB including the
newline. Reject unknown/duplicate/missing/null fields, wrong version/action,
association mismatch, trailing data and oversized/unterminated messages.
`nonce` is a fresh random association value, never a logged secret. Ready address
is exactly `http://127.0.0.1:<canonical-nonzero-port>` without URL extras. Ready
root is canonical and absolute and must match the parent's owned root. The
worker actually binds `127.0.0.1:0` before ready; the parent additionally verifies
token-authenticated capabilities and a fresh file challenge in that root.

Create the Windows worker suspended, assign its private kill-on-close Job, then
resume. No successful launch exists without process ownership. Existing Python
process Jobs remain nested within that ownership. Stop/parent EOF cancels normal
service work and closes the worker; parent abnormal death also reclaims the owned
tree. Wait for owned processes and handles before allowing replacement/restart.
Never terminate by process name, kill unrelated Python or automatically replay
an uncertain download. Lifecycle failures degrade the optional capability and
must preserve the original error rather than announcing successful cleanup.

### Runtime selection and fixed probe

| Worker input | Meaning |
| --- | --- |
| `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED` | Strict plugin switch; desktop managed explicitly sets true |
| `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON` | Explicit trusted interpreter; must be absolute in managed/manifest mode; invalid explicit value never falls back |
| `NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST` | Canonical absolute `jmcomic-runtime.json`; managed defaults to program-adjacent file |

Selection is explicit Python, then validated private manifest, then legacy fixed
tool resolution only in ordinary unmanaged console mode. Managed mode never
searches user PATH. Execute the embedded bridge directly, without a shell, as
`-I -B -X utf8 -c <fixed bridge>` using its bounded JSON protocol. Dependency
probe verifies JMComic 2.7.12/protocol 1 without network; it does not prove site
availability. `--check-jmcomic` does not require worker token/data configuration,
listen, restore operations or initialize the network proxy; enabled probes use
and clean an owned temporary directory. Exit codes are 0 ready, 2 disabled or
unavailable, 1 diagnostic failure.

### Package identity

The manifest schema is 1, target `windows-amd64`. Required fields are
`schemaVersion,target,pythonVersion,jmcomicVersion,python,runtimeRoot,
requirementsSha256,sources,files`. Runtime root is `runtimes/jmcomic`; interpreter
is `runtimes/jmcomic/python.exe`. Sources bind each exact component version,
official HTTPS source and source SHA-256; files bind safe case-unique relative
path, SHA-256, size, role (`runtime|license`) and component. The dependency lock
identity uses LF-normalized bytes so Git CRLF conversion cannot change it.

Validate the compiled dependency version/wheel hash contract, source closure,
required Python ZIP/`_pth`, native DLL/PYD architecture, CA bundle, package data,
`.dist-info`, component licenses, complete ordinary file inventory and bounded
size. Reject traversal, Windows device aliases/alternate streams/trailing dots or
spaces, links/reparse aliases, mismatched size/hash and undeclared files in the
owned runtime/license trees. Do not reject unrelated program tools outside those
trees. No runtime pip, user-site imports or copied development venv is allowed.
Checksums establish consistency with a trusted package, not an independent
publisher signature or clean-system DLL compatibility.

Every release contains the collection worker. Optional runtime input is accepted
only for Windows amd64 and is copied with its licenses/manifest and revalidated.
New `release.json` schema 2 requires `jmcomicRuntime: null` or the exact summary
`{manifest,sha256,pythonVersion,jmcomicVersion}`. The summary binds the actual
runtime manifest. Schema 1 archives retain their original verification contract;
schema 2 also requires the worker and executable target architecture. Included
runtime archives add `-jmcomic` before an optional `-development` suffix.
Release metadata distinguishes basic media distribution from an included
JMComic runtime; development-without-tools remains an independent limitation.
Do not label a development package a complete offline media distribution.

The app may receive a desktop-only `CollectionConfig.UnavailableReason` when its
worker URL is empty. It is fixed sanitized startup context, not an environment or
browser parameter. Preserve authentication and existing 503 behavior while
showing why optional collection is unavailable; media and published comics stay
readable.

## 4. Validation & Error Matrix

| Condition | Required result |
| --- | --- |
| Invalid or incomplete saved schema 2 | Refuse; preserve original file and identity |
| Valid schema 1, no explicit external connection | Read as disabled; byte-exact backup before persisted migration |
| Missing worker / invalid ownership / startup timeout | Main media service remains available; sanitized optional-feature reason and Chinese logs |
| Missing/corrupt private runtime | Worker serves other capabilities; JMComic unavailable, no PATH fallback |
| Explicit interpreter invalid with otherwise valid runtime | JMComic unavailable; no fallback |
| Foreign ready/stop, malformed control data, parent EOF | Reject or cancel, clean only owned process tree |
| Actual dependency probe succeeds, site unavailable | Capability means local readiness only; real request reports network error |
| Download interrupted at stop | No partial publication or blind replay; original recovery rules remain |
| Runtime member/hash/architecture/lock/license mismatch | Reject assembly/verification/package activation |
| Native managed requested for Docker | Reject configuration; no implicit Windows worker launch |

## 5. Good / Base / Bad Cases

- Good: trusted complete Windows package, new local profile, verified worker and
  private runtime, authenticated search/preview/download, complete publication,
  normal exit and immediate DLL/program replacement.
- Base: basic package or migrated profile stays disabled, or local dependencies
  fail. Existing media/history/published comics remain usable; user can explicitly
  enable/recheck or keep a configured external worker.
- Bad: poisoned PATH/PYTHONPATH, unrelated worker at a guessed port, foreign
  marker, a changed DLL, user-supplied script or forced external-process kill.
  Reject at the relevant boundary without adopting or rewriting user data.

## 6. Tests Required

- `verify_collection_managed.py`: actual worker private control/authentication,
  strict malformed boundaries, stop/EOF/restart, interrupted download recovery,
  diagnostic exit codes, explicit override precedence and real private runtime.
- `verify_desktop_collection.py`: saved config matrix/migration, environment
  contamination, missing-worker degradation after authenticated app login,
  actual worker identity/root, mode IPC, normal/abnormal exit, unrelated sentinel
  survival, private dependencies, nested Jobs and file-release assertions.
- Runtime verifier: real assembly and isolated dependency probe/bridge; tampered
  hash, member inventory, versions, PE, links/aliases and licensing rejection.
  Release self-tests plus a real archive validation, including basic/development
  metadata and optional runtime contents.
- Original desktop, JMComic base/advanced, collection/core/release and affected
  optional adapter regressions. Run Go vet/test/build, Vue typecheck/build and
  Linux amd64/arm64 cross-builds. Remove newly generated temporary unit-test
  source after execution; preserve reusable integration harness and truthful logs.
- Record fixture vs real-dependency vs live-site evidence separately. Local
  administrator tests do not establish clean ordinary-user machine acceptance.

## 7. Wrong vs Correct

Wrong: search global PATH for Python, parse stdout logs as ready, connect to a
preselected port, and kill every `python.exe` before overwriting the runtime.

Correct: validate the trusted private manifest, use inherited bounded control
pipes plus authenticated root challenge, reserve the real loopback listener,
own the process tree before resume, and wait for owned handles to release.
