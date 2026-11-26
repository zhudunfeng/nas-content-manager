# Final validation: Windows executable-adjacent dotenv

Date: 2026-11-26. Task is ready for human review; no automatic commit, deployment or archival.

## Delivered behavior

Windows desktop reads optional `.env` beside the real executable, preserving default executable-adjacent `qiku-data`. Explicit CLI > process environment presence > dotenv > saved JSON > defaults. Empty process values suppress file values and keep original consumer semantics. NAS keys are case-insensitive; only NAS file assignments participate. One startup snapshot serves desktop migration/options, native BT, shared app and controlled collection worker. No global environment mutation or standalone/Linux CLI dotenv loading.

Parser supports UTF-8/BOM, CRLF/LF, Chinese and literal Windows paths, comments, export, quoted/unquoted single-line values and last duplicate wins. It does not expand variables/commands/escapes. Read is bounded to 64 KiB with Chinese value-free errors. Help and explicit controls remain available even after the file is damaged. Worker allowlist/fixed paths and Docker empty env-file remain unchanged. Logs record load/count only.

Source Windows example `.env.windows.example` is shipped as `.env.example`, separate from the original Compose example; no active `.env` is shipped. Windows/UAV/JMComic docs describe precedence, full restart, accepted keys and dependency prerequisites. Existing active user files were not modified.

## Executed evidence

| Check | Result | Evidence |
| --- | --- | --- |
| Temporary Go behavior | 5 groups, 24 parser subcases passed | `.runtime/verify/windows-dotenv/go-test.log`, `dotenv_task_test.go.txt` |
| Go vet and cleanup compile | All packages passed, independently repeated | `go-vet.log`, `go-test-post-cleanup.log`, `check-go-vet.log`, `check-go-test.log` |
| New real Windows config/worker integration | 11 groups passed twice independently | `integration.json`, `check-integration.json` |
| Existing real desktop regression | 20 groups, embedded HTML, 10 icon sizes and tray passed | Main tool output; source unchanged in final package |
| Existing real collection regression | 11 groups passed | `collection.json` |
| Release boundaries | Existing 131 checks passed | Main verify_release self-test output |
| Actual final archive | 24 files passed verifier, correct Windows example, no active .env | `final-result.json` |
| Formatting / Python AST | gofmt, git diff --check, AST passed | Independent review and `static-cross-build.json` |
| Linux amd64/arm64 | 6 entry artifacts + both desktop-package compilations passed, not executed | `static-cross-build.json`, `build-linux-*.log` |

All evidence filenames without full paths are under `.runtime/verify/windows-dotenv/`. Independent review found no fixes needed: `quality-review.md`.

The real Windows integration proves actual app settings through authenticated HTTP; actual worker plugin true/false/empty and quota selection; fixed token/data paths; CWD/profile decoys; invalid/locked/nonregular/oversized files; immutable snapshot after component restart; bad-file status/quit; no secret-value logging. Plugin enabled with missing private dependencies correctly reports unavailable rather than claiming successful video readiness. No site network was used.

Temporary `internal/desktop/dotenv_task_test.go` was removed precisely after execution and evidence copy. The reusable Python integration verifier remains. Frontend and shared API/storage logic were unchanged; existing embedded frontend assets were used.

## Final artifact identity

Archive: `.runtime/releases/windows-dotenv/final/qiku-0.1.0-windows-amd64-development.zip`.

- Archive SHA-256: `476d9d6ab9b7ad9eb79a159ed0853c11eb161e521da1e3a8bece642669576162`.
- `栖库.exe` SHA-256: `72d042178918571a5672537f306cf24e4eb0bf82d401c6a47885cc58772695ba`.
- Final executable/service/worker bytes exactly match the earlier tested package. Final bundled Windows/plugin docs and example exactly match final source. Source hashes and build identities are in `final-result.json`.

This is an incomplete development verification package, without the full private Python/UAV/FFmpeg materials. Complete dependency bundles, real public sites, Docker deployment, Linux execution and clean Windows VM remain untested. Turning a plugin switch on does not install these dependencies.

One auxiliary cross-build script initially could not discover Go through inherited PATH. It was corrected to use the repository `.tools/go/bin/go.exe`, consistent with project_env command resolution; no product failure or source fix was involved. Final cross-build evidence is passed.
