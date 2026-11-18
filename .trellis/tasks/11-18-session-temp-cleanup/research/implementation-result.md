# Reusable session-cache cleanup implementation

## Scope and files

- Added `scripts/clean-session-temp.ps1` as the fixed Windows maintenance entry point.
- This implementation agent did not modify application/password code, README, user documentation, or shared Trellis specifications.
- The independent checker subsequently owns targeted fixes to the script. It fixed PowerShell's null binding for `File.Replace` and tightened the Node repository-marker path boundary.
- Temporary behavioral test source: `.runtime/session-temp-cleanup/verify-temp.ps1`. All GUID-named E-drive fixtures were removed in their `finally` blocks. The parent agent will remove the test source with a narrow, exact-file command and confirm that final cleanup separately.

## Delivered behavior

The public parameters are `-Execute`, `-ManifestPath`, `-IncludeGoBuildCache`, `-IncludeGoModuleCache`, `-BrowserProfileName`, and `-Since`.

The default operation only previews Node cache files containing the current repository's absolute path. Shared Go build and module/checksum caches require independent opt-ins. A browser target requires an exact `mcp-chrome-<hex>` profile. Default Go configuration is checked through `go env`; redirected Go caches are rejected.

The schema-version-1 manifest defaults to `.runtime/session-temp-cleanup/manifest.json`. Its precise files contain target identity, absolute path, length, creation ticks and write ticks. Only ancestors of selected files are recorded as candidate empty directories; the shared Node root is never included for deletion. Explicit execution consumes that saved manifest without rescanning or accepting new selectors. Results are written beside it as `manifest.<UTC timestamp>.result.json`.

Validation covers repository identity, exact object shapes, numeric types, fixed target roots, path boundaries, Windows aliases and reparse points on targets and ancestors. Execution refuses relevant Go/build or selected-profile browser activity, and refuses unreliable process discovery. Changed or recreated files are retained, already-missing files are idempotent, and unplanned files never enter the deletion loop. Deletes are single-file operations and nonrecursive empty-directory operations. Chinese summaries distinguish confirmed deleted logical bytes from planned size and partial failures.

## Validation evidence

Date: 2026-11-18, local Windows host.

### Syntax and compatibility

- PowerShell **7.6.4** parser: passed.
- Windows PowerShell **5.1.19041.6456** parser: passed.
- Script UTF-8 BOM: verified present for Chinese source compatibility with Windows PowerShell 5.1.
- Both runtimes loaded real JSON with ISO timestamps, empty arrays, and Chinese results correctly. The test reader explicitly used UTF-8, as does the production manifest reader.

### Read-only real preview

```powershell
.\scripts\clean-session-temp.ps1
```

Result: **139 Node cache files, 5,733,716 logical bytes**, with the expected current-user Node root. No C-drive files were deleted.

An initial full-selection preview completed scanning but failed while replacing the existing manifest: PowerShell bound the third `File.Replace` argument `$null` as an empty string. The independent checker changed that argument to `System.Management.Automation.Language.NullString.Value`. Repeated manifest replacement then passed on both PowerShell runtimes in the isolated behavioral suite. The parent agent owns the final real full-selection preview and inventory comparison; this agent did not repeat the 25,000-file scan after the fix.

### Isolated behavioral verification

The generated test source dot-loaded production functions and supplied an explicit **E-drive fixture context**; it mocked only system context, Go configuration and CIM process discovery for end-to-end execution. It never executed cleanup against the production C-drive context. Each run used a fresh GUID-named fixture below `.runtime/session-temp-cleanup/`.

Commands (temporary source removed after task verification):

```powershell
.\.runtime\session-temp-cleanup\verify-temp.ps1
powershell -NoProfile -File .runtime/session-temp-cleanup/verify-temp.ps1
```

Final result: **55 checks passed in PowerShell 7; 55 checks passed in Windows PowerShell 5.1**.

Covered cases:

- Node ownership, slash variants, sibling-repository marker exclusion, creation-time filtering, and preservation of the shared Node root.
- Empty manifests/arrays, ISO-date readback, repeated atomic JSON replacement, and strict file-versus-directory record shapes.
- Relative paths, `..`, ADS/device aliases, repository-external manifests, sibling path-prefix escapes, browser wildcards, unknown kinds/case aliases, foreign repository identity, unknown file ownership and invalid numeric metadata.
- Target-root junctions, junction ancestors, a reparse point at the manifest itself, and a manifest ancestor junction.
- Simulated active Go compiler, selected-profile browser, unrelated normal browser, unreadable browser command line, failed CIM discovery, and Node-only independence from unrelated process discovery.
- A 9-file execution with **5 deleted, 1 missing, 2 changed/retained, 1 locked/failed**, returning partial-failure code 1 and recording exactly **74 confirmed deleted logical bytes**.
- Actual deletion and absence verification of a read-only Go-module sample; retention of new files, another project's Node cache, and changed files (including a Node file whose marker was removed after preview).
- Empty recorded-directory removal and nonempty-directory preservation.
- Retry after releasing the file lock deleted only the original failed file. A third run deleted zero bytes. A recreated same-name/same-size/same-write-time file was retained because its creation time differed.
- Rejection of selection switches during execution.

The first Windows PowerShell test attempt used plain `Get-Content` to read a UTF-8 result file and encountered the shell's legacy default decoding. This was confined to the temporary test reader; changing that reader to explicit UTF-8 produced the passing full result above. Production already used explicit UTF-8 and required no change for that issue.

## Remaining limits and follow-up

- **No real C-drive deletion was performed.** A prior direct cleanup action was rejected by tool policy; the implementation was not used to route around that rejection. Real cache deletion remains a user-run acceptance step after review and closing matching processes.
- Final full-selection preview, default manifest inventory comparison, independent-check report and exact temporary-source removal are owned by the parent/checker.
- No Go/Vue application build, Docker deployment, process termination, elevation, execution-policy change, commit or archive was performed for this maintenance-only change.

## Parent completion note

The parent completed the final real read-only preview with the documented session selectors: **25,182 files / 1,735,601,734 logical bytes**, matching the earlier inventory. The default manifest is available under `.runtime/session-temp-cleanup/manifest.json`. Both generated test-source files were removed with exact nonrecursive E-drive file commands; a subsequent directory read confirmed no test sources or fixtures remained. C-drive cache deletion was not executed.
