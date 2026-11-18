# Windows Actions release console encoding fix

## Report and reproduction

- Reported command: `python scripts/release.py --target windows-amd64 --go go --development-without-tools`.
- On local Windows, launching the original script with `PYTHONIOENCODING=cp1252`, `PYTHONUTF8=0`, captured stdout/stderr, a dedicated `.runtime/ci-encoding-before` output directory and `--skip-web-build` reproduced exit 1 and exactly `'charmap' codec can't encode characters in position 0-6: character maps to <undefined>`.
- Failure occurs in `run()` while printing the first Chinese progress message, before invoking Go. Error diagnostics were escaped as `\u....` on stderr. Existing release file writes already specify UTF-8; changing archive filenames or file content encoding would not fix the failing stream.

## Scope and implementation

- Configure CLI stdout/stderr as UTF-8 before argument parsing in release, archive verification and desktop-image packaging commands. Share `configure_console() -> None` in `release.py`, with no import-time stream mutation and support for replacement streams without `reconfigure`.
- Explicitly document and set job-level `PYTHONUTF8=1` and `PYTHONIOENCODING=utf-8` in the shared Actions matrix.
- Extend the reusable `verify_release.py --self-test` harness to force legacy encodings in fresh subprocesses. Keep Chinese messages and original success/failure exit codes.
- No application behavior, Docker deployment, image/resource operations or business regression scope changes.

## Verification

- Original failure reproduced before editing.
- Local Windows Python 3.14.4: all three modified Python scripts compile. Dedicated quality review found no confirmed defects; `git diff --check` passes.
- Reusable self-test: 68 assertions pass, retaining the original 52 and adding 16 console cases. The final parent run explicitly starts the verifier with `PYTHONIOENCODING=ascii`, `PYTHONUTF8=0`.
- Actual complete Windows development build with `PYTHONIOENCODING=cp1252`, `PYTHONUTF8=0` passes: `npm ci`, Vue strict typecheck, Vite production build, shared `nas.exe`, tray `栖库.exe`, ZIP and outer checksum creation. No `--skip-web-build` or mocked build used in this evidence.
- Result: `.runtime/ci-encoding-fix/releases/qiku-0.1.0-windows-amd64-development.zip`; actual archive verification passes for all eight members plus inner/outer checksums. Logs are under `.runtime/ci-encoding-fix/{build,verify}.{stdout,stderr}.log`.
- Shared release lock and embedded-resource staging directories are absent after build. No temporary unit-test source files or Docker resources were created.
- `npm ci` reports an existing deprecated type package and six high-severity dependency audit findings; dependency files are unchanged by this scoped fix. No dependency audit/remediation was requested or performed.
- This is local validation, not execution of GitHub-hosted Actions or the exact Python 3.12 runner. Development package still intentionally omits reviewed FFmpeg redistribution materials.

## Follow-up: Node action runtime warning

- User additionally reported the Node.js 20 action-runtime warning. Updated all five official actions in the build workflow to v6, including upload-artifact, keeping application toolchain pins and the Linux/Windows build matrix unchanged.
- Verified `runs.using: node24` in each official v6 `action.yml`:
  - https://raw.githubusercontent.com/actions/checkout/v6/action.yml
  - https://raw.githubusercontent.com/actions/setup-go/v6/action.yml
  - https://raw.githubusercontent.com/actions/setup-node/v6/action.yml
  - https://raw.githubusercontent.com/actions/setup-python/v6/action.yml
  - https://raw.githubusercontent.com/actions/upload-artifact/v6/action.yml
- Deprecation notice supplied by user: https://github.blog/changelog/2025-09-19-deprecation-of-node-20-on-github-actions-runners/
- Action runtime and `setup-node` application build version are separate contracts; no application Node upgrade is required to address this warning.
