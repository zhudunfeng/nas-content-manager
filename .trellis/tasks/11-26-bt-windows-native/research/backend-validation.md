# Backend Windows BT validation

## Changes

- `download_config.go`: engine-path equality uses the path's own format. Drive-absolute Windows paths accept case/separator/trailing-separator differences; POSIX paths remain case-sensitive. Drive roots retain the slash (`E:/`), avoiding the ambiguous `E:` result from plain `path.Clean`.
- Windows engine roots reject UNC/device/drive-relative forms, alternate streams, parent traversal, invalid filename components, controls and excessive length. Existing server-generated task paths and identity/tag checks remain authoritative.
- `download_sync.go` and `download_import.go`: ownership, move reconciliation and publication use the same engine-path comparison; host-side stored root/final comparisons reuse the existing OS-aware `downloadPathKey`.
- `download_torrent.go`: the existing cross-platform filename contract now also rejects `< > " | ? *` before engine submission. Names are never automatically rewritten.
- Password and Torznab source-file readers remove one leading UTF-8 BOM after bounded reading. Password whitespace and internal bytes are preserved; JSON still requires strict known fields, EOF, 1–8 validated sources and the original byte budget.
- `verify_downloads.py --windows-engine-paths`: the real application sees a virtual Windows drive root, mixed case, backslashes and trailing separators, while actual fixture files remain in the isolated host root. BOM password writing is included. The default POSIX fixture behavior remains available. Six invalid-name upload assertions are retained in the reusable harness.

## Executed checks

All local commands ran through `python -B scripts/project_env.py -- ...` on the Windows development host, with controlled caches and temporary files under repository `.runtime/`.

- Three temporary Go behavior groups passed: path equality/rejection and drive-root handling; real `findOwnedTorrent` ownership reconciliation; password/source UTF-8 BOM with malformed, over-budget and trailing-JSON rejection.
- The temporary Go source was removed after execution as required by AGENTS.md.
- Production CLI build passed: `.runtime/verify/bt-windows-final-nas.exe`.
- `gofmt` completed on affected Go files; `git diff --check` passed for this subtask.
- Full Windows-path integration passed **36 assertions** with `.runtime/verify/bt-windows-nas.exe`; evidence: `.runtime/verify/bt-windows-backend-20261126/result.json`. This build includes engine-path/BOM changes; the subsequent six-character invalid-name check was covered by the final-code run below.
- Full default POSIX-engine-root integration passed **42 assertions** with the final CLI build; evidence: `.runtime/verify/bt-posix-backend-20261126/result.json`. This includes the six retained Windows-invalid-name upload cases.
- Both complete runs exercised original bytes after selected-file publication, all five media import types, comic unselected-page isolation, app restart/favorites/progress, cookie refresh, engine offline handling, independent import retry, v2/hybrid identities, external-task refusal, lost-add-response recovery without duplicate add, and archive preserving published files.

## Evidence boundaries

The controlled qB implementation validates application HTTP/SQLite/task/publication behavior, not real BT piece transfer, seeding, peer discovery or public indexer availability. No Docker, WSL, qB/Jackett installation, real tracker, user download directory or third-party process takeover was used.
