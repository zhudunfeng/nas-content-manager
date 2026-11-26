# Independent quality review: Windows desktop dotenv

Date: 2026-11-26. Reviewed the uncommitted task changes against HEAD `af70adc` and the task PRD/check context. The earlier portable-profile change is already part of HEAD and was preserved.

## Findings (fixed)

No defects requiring a source change were found. No production, documentation or verification-script edits were made by this review.

## Findings (not fixed)

None within the task scope.

## Review coverage

- Read `.current-task`, PRD, check context, applicable launcher/runtime/Windows plugin/BT specifications, implementation notes, the new parser/snapshot file, all changed Go call sites, documentation/example/release changes and the permanent dotenv verifier.
- Traced one startup snapshot through ordinary/print-config loading, schema-1 external-mode migration, CLI/environment precedence, BT configuration, shared application lookup and managed-worker environment construction. Environment presence (including empty strings) correctly suppresses file fallback; case normalization matches the Windows contract. Runtime-owned listener and managed connection values remain above administrator input.
- Confirmed parsing is bounded to 64 KiB and NAS-prefixed file values, accepts documented BOM/CRLF/quotes/literal backslashes/comments, validates irrelevant assignments without applying them, does no expansion/execution, and omits input values from parser errors. File loading closes its handle before startup.
- Confirmed explicit control routing and help precede dotenv loading, internal restarts retain the controller snapshot, and neither global/user/system environment nor ordinary service/worker CLI gained dotenv loading.
- Confirmed the managed worker still has its existing configuration allowlist and fixed state/token/tool/runtime/address ownership. The Docker empty `--env-file`, existing container environment and strict Compose validation remain intact.
- Confirmed root Compose `.env.example` is not reused as the Windows template; Windows releases copy `.env.windows.example` as inactive `.env.example`, never the user's `.env`. The documentation explains copy/name/restart/presence precedence, complete plugin dependencies and upgrade preservation.

## Verification

- Lint/static: independent `python -B scripts/project_env.py -- go vet ./...` passed (exit 0), log `.runtime/verify/windows-dotenv/check-go-vet.log`.
- Type/compile: independent `python -B scripts/project_env.py -- go test ./...` passed after temporary test removal (exit 0), log `check-go-test.log`. This clean-source run verifies package compilation; it is not counted as behavioral unit coverage.
- Formatting: `gofmt -l` returned no changed Go files.
- Python syntax: `ast.parse` of `scripts/release.py` and `scripts/verify_desktop_env.py` passed without generating bytecode.
- Behavioral source evidence reviewed: the implementation's five temporary Go test groups, including 24 parser subcases, passed and were removed; saved test source and full log remain under `.runtime/verify/windows-dotenv/`.
- Independent real Windows integration rerun: `scripts/verify_desktop_env.py` passed all 11 groups with the built embedded desktop EXE and actual worker; evidence `check-integration.json` and `check-integration.log`. This exercised actual API configuration, authenticated worker readiness, explicit false/empty process overrides, quotas, fixed-path pollution rejection, cross-CWD/profile decoys, non-readable files, malformed-file status/quit, snapshot-preserving recheck and secret-free logs.
- No new test source files were created by this reviewer; `internal/desktop` contains no remaining `*_test.go` from this task.

## Limits

The independent integration uses a development package without private Python/plugin/media tool bundles. It proves configuration transport and graceful dependency-unavailable behavior, not real site search/download or complete offline distribution readiness. No Docker daemon, Linux execution, clean Windows VM or physical NAS acceptance was performed here. The main agent owns final cross-build, archive/evidence summaries and task status.
