# Windows desktop application icon

## Goal

Give the Windows desktop executable and native tray a consistent Qiku icon, reusing the existing dark background and mint library mark from `web/public/favicon.svg`.

## Requirements

- Embed a multi-resolution application icon into the desktop EXE so Explorer and shortcuts can identify the application.
- Load the same brand into the native tray and window class, including recovery after Explorer restarts.
- Keep plain Go builds and the existing Windows release pipeline usable without installing a system resource compiler or GUI framework.
- Keep Linux builds unchanged and preserve desktop startup, shutdown and executable-file release.
- Use repository-local caches and temporary outputs; do not change user environment settings.

## Acceptance criteria

- [x] The desktop EXE contains a valid application icon resource at multiple Windows sizes.
- [x] The native tray and window use the branded icon instead of the generic system application icon.
- [x] Resource generation is reproducible and documented; normal builds need no additional tool installation.
- [x] Windows desktop build, Go checks and Linux cross-build succeed.
- [x] Focused native verification checks icon loading and process exit/file replacement in an isolated profile.
- [x] No temporary unit-test sources were introduced; the reusable desktop integration harness was extended instead.

## Technical approach

- Inspect Windows resource embedding conventions and the current release workflow.
- Reuse the existing favicon geometry and colors; supply suitable small and high-DPI images.
- Keep Windows resources scoped to `cmd/nas-desktop` and native API changes scoped to `internal/desktop`.

## Scope and boundaries

No desktop framework migration, frontend redesign, installer/association changes or Docker validation. Leave the completed `windows-complete-exit` task and its recorded evidence intact. A full offline tool bundle is not required for icon verification.

## Research references

- `research/windows-icon-resources.md` — resource embedding and native icon ownership investigation.

## Verification

Completed on 2026-11-19. Both ordinary GUI and embedded development-package EXEs passed the real native-tray integration harness, all ten resource frames matched the source ICO, and immediate executable replacement/restart passed. Resource regeneration (`--check` and `go generate`), Go vet/package compilation, release verification (68 cases), Linux amd64/arm64 cross-builds and task-context validation passed. Details and boundaries are in `research/implementation-validation.md`.
