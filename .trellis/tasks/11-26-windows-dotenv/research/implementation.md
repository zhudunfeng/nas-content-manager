# Implementation: Windows desktop dotenv

## Changed production files

- `internal/desktop/environment.go`: immutable case-normalized environment snapshot; real executable directory resolver; bounded ordinary UTF-8/BOM dotenv parser; only NAS-prefixed file entries participate; process presence wins including empty values.
- `internal/desktop/config.go`: snapshot-aware desktop load and CLI merge; schema-1 external-mode migration uses same source; compatibility wrappers preserved.
- `internal/desktop/collection_config.go`: default/external detection receives same lookup and real executable directory identity.
- `internal/desktop/launcher.go`: parse after help/control routing; both print and normal startup use snapshot; store it on controller for internal restarts; native BT and shared app consume same source; Chinese count-only loaded log.
- `internal/desktop/collection_managed.go`: existing child-only allowlist uses snapshot; fixed paths, runtime manifests, secrets, address and tools remain launcher-owned.
- `internal/desktop/platform_windows.go`: default profile shares real executable directory resolution with dotenv and worker loading.

No global/user/system environment mutation, CLI/standalone-worker dotenv auto-loading, desktop schema migration, or Docker container environment expansion was introduced.

## Behavioral checks executed

Temporary `internal/desktop/dotenv_task_test.go` executed successfully and was removed after copying to `.runtime/verify/windows-dotenv/dotenv_task_test.go.txt`.

- 24 parser cases: blank/BOM/CRLF/Chinese, literal backslash Windows paths, quotes, equals/hash/comments, empty values, duplicate case keys, no expansion, NAS-only input, bad keys/quotes/trailing/multiline/CR/NUL/UTF-8/UTF-16/double-BOM, validation even for irrelevant keys.
- File boundary: absent/empty/nonregular/64-KiB/over-limit/encoding; readonly snapshot input and no directory creation.
- Snapshot: process false and explicit empty shadow file values, Windows case-insensitive keys, copied input state, no process-environment mutation.
- Configuration: CLI > process > dotenv; app workers/interval same-source values; v1 migration uses dotenv external mode and does not modify original saved bytes.
- Managed worker: both plugin switches, Python and four quotas; process override and empty-switch strict rejection; fixed token/data/manifest/tool/address ownership; unrelated and Python loader environment excluded.

`python -B scripts/project_env.py -- go test -v ./...` passed with those behavioral tests.
`python -B scripts/project_env.py -- go vet ./...` passed.

Evidence: `.runtime/verify/windows-dotenv/go-test.log`, `go-vet.log`, source copy above; post-cleanup compile record `go-test-post-cleanup.log`.

## Remaining verification

Primary agent owns real Windows EXE/worker integration, packaging/example/docs/spec, Linux cross-build and final quality review. Unit coverage proves source contracts, not complete private UAV dependencies, remote site connectivity or clean-machine acceptance.
