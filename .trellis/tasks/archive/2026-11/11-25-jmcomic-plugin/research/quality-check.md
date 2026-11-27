# JMComic independent quality review

Date: 2026-11-25. Scope: current PRD, collection contract and cross-layer app/worker/Vue changes. This review used targeted `rg` and file reads because the existing CodeGraph database had no enabled CLI/MCP command. No user data or external comic content was used.

## Findings (fixed)

1. `web/src/components/CollectionSourceDialog.vue`: changing a new source from RSS/external to JMComic could retain hidden `allowPrivate` or `externalUrl` values. The fixed-source backend correctly rejects them, leaving the user unable to save. JMComic submission now explicitly clears unrelated root/path/output/link settings and private-network permission.
2. `web/src/components/CollectionJMComicDialog.vue`: upstream `hasNext=true` on page 1000 could enable a request for page 1001, outside the documented API boundary. The next-page action is now disabled at the maximum supported page.
3. `internal/collectionworker/config.go`: the initial `strconv.ParseBool` implementation also accepted `1`, `t` and uppercase values despite the documented strict `true`/`false` contract. Reported to the owning implementation agent, who replaced it with the exact literal switch. Relevant field comments were also reported for synchronization with the new comic profile.

## Cross-layer review

- Confirmed numeric album IDs are string fields with the same positive 1–12 digit/no-leading-zero validator in app and worker. Arbitrary URLs, paths, source identifiers and output profiles are rejected by the fixed adapter.
- Confirmed appended `albumId,omitempty` fields retain old JSON bytes when absent; worker `requestPreview` preserves album identity and hashes bind it. Existing request-ID reconciliation still precedes source-revision/preview-expiry rejection.
- Confirmed source search rechecks source revision/enabled state after worker I/O and does not turn failures into empty success. Results remain plain text with bounded tags; no upstream links enter the browser DTO.
- Confirmed Python uses fixed `-I -B -c` execution, no shell, bounded JSON/output, sanitized environment and the existing process-tree lifecycle. Both explicit client proxies and bootstrap `DEFAULT_PROXIES` point to the authenticated public-only proxy; the proxy independently validates resolved destinations.
- Read the installed JMComic 2.7.12 API/entity/downloader source and checked option-subclass construction, single-album search wrapping, integer metadata, sorted episode iteration, synchronous callback signatures and final `all_success` semantics against the bridge.
- Confirmed the bridge uses fixed numeric chapter/page paths, forces raster formats, validates actual images and budgets, compares metadata before/after download and rejects partial success. Worker checks returned identity/count and actual output before the existing explicit publication and exact comic-import workflow.
- Confirmed superseded searches abort and reject late results. Preview invalidates on album/source changes; task submission freezes its original UUID/body across uncertain retries. New dialogs use the existing single native modal transition and fixed comic target.
- No database migration or release image-manifest format change was introduced. The dedicated image installs a separate hash-locked venv; native mode remains optional and disabled by default. Dockerfile includes third-party notices.

## Executed checks

All commands ran through `python -B scripts/project_env.py --` with project-local runtime/cache directories.

| Check | Result |
| --- | --- |
| `go vet ./...` | Passed |
| `go test ./...` | Passed, empty existing suites; compilation only |
| `npm --prefix web run typecheck` | Passed, strict vue-tsc |
| `npm --prefix web run build` | Passed, 185 modules |
| Temporary `TestJMComicReviewBoundaries` | Passed: positive/nondigit/long/traversal IDs, Unicode length, C0/C1 controls and page defaults/limits |
| Temporary `TestJMComicReviewLegacyJSON` | Passed: old local preview and operation serialized byte snapshots remain identical |
| Temporary `TestJMComicReviewWorkerBinding` | Passed: fixed input whitelist, default normalization, historical request hash, preview-token exclusion and album-ID hash separation |

The two generated reviewer `_test.go` files were removed after the three behavior groups passed, as required by AGENTS.md. The reusable protocol and browser integration harnesses belong to their respective verification phases and are recorded separately by the primary agent.

## Findings (not fixed)

No remaining defect was identified in this review. Final integration/browser results and spec synchronization are owned by the primary task and must be included before completion. This report does not claim real-site search/download, Docker build/start, Linux runtime or target NAS behavior; package source inspection and Go compilation do not establish those outcomes.
