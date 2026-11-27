# ASCII Password Policy Quality Review

Reviewed on 2026-11-18 against the task PRD, backend runtime contract, frontend application contract, and Trellis quality-check guidance.

## Findings (fixed)

- No implementation defects found; no application code was changed by this review.
- Confirmed both validators admit only U+0021–U+007E, require uppercase ASCII, lowercase ASCII, digits and ASCII punctuation, and enforce the existing 10–72-byte limit.
- Confirmed backend setup validates before bcrypt/account creation and preserves the original input. Neither backend nor frontend applies the new character/complexity policy to existing-account login.
- Confirmed the setup hint is associated with the password input through `aria-describedby`, and login retains its original hint-free path.
- Confirmed modified code has the required Chinese method/field/critical-logic comments and contains no credential logging.

## Findings (not fixed)

- None within the reviewed implementation scope.

## Verification

- Lint: PASS — `.tools/go/bin/go.exe vet ./...`; scoped `gofmt -l internal/app/auth.go` returned no formatting findings; `git diff --check` passed. Git emitted only the existing Windows LF/CRLF checkout warnings.
- TypeCheck: PASS — `npm --prefix web run typecheck` (`vue-tsc --noEmit`).
- Tests: PASS — independently executed the actual transpiled `web/src/password.ts` validator against 65,553 assertions, including all 65,536 BMP code units appended to an otherwise valid password, representative supplementary Unicode, 9/10/72/73-byte boundaries, each missing required category, leading/trailing spaces and trailing LF/CRLF. Every U+0021–U+007E BMP character was accepted in the valid prefix context; every other BMP code unit was rejected.
- Cross-layer review: PASS — reviewed the actual `setup`, `login`, `validNewPassword`, and `authenticate` paths, matching backend/frontend hints and explicit punctuation ranges.
- The parent-owned shared matrix currently contains 71 cases. Parent-owned Go/HTTP suites, production builds, browser verification and temporary Go test cleanup remain the parent agent's responsibility; their running processes and source files were not modified or polled by this reviewer.
- No Docker deployment, automatic commit or task archive was performed. No temporary test source was created by this review.
