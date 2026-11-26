# Implementation coordination

用户于 2026-11-26 确认执行 plan.md；原规划报告保留其产生时的事实，本文件记录实施接口与责任。

## Components and ownership

- Runtime implementer: `internal/jmcomicruntime/`、Python 运行时装配／校验脚本、`scripts/release.py`、`scripts/verify_release.py`；拥有运行时清单格式并尽早通知其他实现者稳定 API。
- Worker implementer: `internal/collectionworker/`、`cmd/nas-collection-worker/`、`internal/collectioncontrol/`，及专用 worker 集成验证脚本；负责运行时选择、诊断、托管子进程控制端。
- Desktop implementer: `internal/desktop/`、必要的 `cmd/nas-desktop/`、桌面托管回归脚本；负责模式迁移、托管所有权、就绪校验、退出、托盘入口。
- Root: 跨层衔接、主应用必要诊断、文档／规范、最终构建回归与独立质量审查。

## Shared contracts

- Manifest path is `<program-dir>/jmcomic-runtime.json`; worker receives absolute `NAS_COLLECTION_WORKER_JMCOMIC_RUNTIME_MANIFEST`. Administrator explicit absolute Python override remains supported; explicit failure never falls back. Managed mode never implicitly searches PATH.
- New `internal/jmcomicruntime` exposes `Load(manifestPath string) (Runtime, error)` and `Runtime.Python string` (canonical absolute interpreter). Full validation covers manifest paths, versions, file digests, platform/architecture, licenses and required runtime resources. This package does not import desktop or worker.
- New `internal/collectioncontrol` defines a versioned, bounded, strict newline JSON protocol over **inherited anonymous stdin/stdout pipes**. Handles are created by parent, not a discoverable shared endpoint; stderr remains structured human logs. This chooses private inherited pipes instead of a named pipe, preserving per-instance ownership without publicly named control endpoints.
- Parent start message: `version:1, action:"start", nonce:<random>, instanceId:<profile-instance>`. Child ready message: `version:1, action:"ready", nonce:<same>, instanceId:<same>, address:<actual-loopback-URL>, readRoot:<canonical-worker-data-root>`. Stop message uses `action:"stop"` with the same association. Parent EOF also cancels; malformed/oversized/extra messages fail closed. No ordinary log is accepted as readiness.
- Worker CLI flag `--managed-stdio`; only this mode enables protocol and requires exact `127.0.0.1:0`. It must bind the real listener and initialize the worker before ready. Normal CLI stays compatible.
- Desktop waits for ready under timeout/cancellation, verifies association/root and authenticated capabilities, then injects the whole app connection tuple. Child is assigned to a parent-owned Windows Job before resumed; existing Python Jobs are preserved and nesting tested.
- New desktop modes `disabled|managed|external`, desktop schema v1→v2. Preserve old explicit external environment/config; no global `os.Setenv`. No automatic retry of uncertain operations, no unlimited worker restart.
- New configuration, entity fields and functions carry Chinese comments; significant lifecycle/failure logs are Chinese and do not expose tokens/nonces/private URLs.

## Verification boundaries

All commands inherit `python -B scripts/project_env.py -- ...`. Tests and build outputs stay under `.runtime/`; temporary unit test source files are deleted after execution. Run local Windows tests, Linux cross builds and release verification, recording exact evidence. Clean Windows VM / actual public site / ARM64 execution are separate target evidence and must not be claimed unless performed.
