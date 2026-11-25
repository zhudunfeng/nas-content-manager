# Research: UAV fixed collection adapter integration points

- Query: How should a fixed UAV video search/download adapter reuse the existing JMComic and web collection flows?
- Scope: internal; upstream behavior and licensing are researched separately in `uav-upstream.md`.
- Date: 2026-11-25
- Method: Read current Trellis workflow/specs and production source; use the available CodeGraph CLI (`query JMComic --json --limit 100`) before targeted textual searches. No application code was changed or third-party code executed.

## Findings

### Recommended architecture

Add one fixed optional source/capability kind, suggested `uav`, backed only by the independent collection worker. Reuse the existing source CRUD, expiring metadata preview, immutable operation request, persisted UUID task, worker cancellation/publication, manifest checking, scan/import and existing video reader. Do not create a separate download coordinator, arbitrary plugin upload system, browser downloader, or schema migration.

Two implementation variants depend on the upstream research:

1. **Preferred when upstream has a suitable importable API:** fixed embedded bridge or reviewed adapter in `internal/collectionworker/uav*`, isolated behind existing worker APIs and execution/network guards. Native installations explicitly enable and supply trusted dependencies; the dedicated image can install locked dependencies at build time.
2. **Only when a compatible fixed upstream extraction/search protocol is practical:** implement the small upstream protocol in Go and reuse existing video acquisition helpers after resolution. Avoid silently calling generic `fetchWeb` for HLS: current web format policy deliberately excludes HLS/external ffmpeg network fetch, so an HLS upstream needs its own bounded HTTP segment implementation or fixed audited bridge under the same proxy. The upstream research must settle this; do not weaken global web policy.

### Files found and exact integration points

| File | Role / change |
| --- | --- |
| `internal/collectioncontract/models.go:47` | Shared `PreviewRequest`, optional source-specific identifier fields, and `OperationRequest` at line 71. Existing `AlbumID` is `omitempty` in both. Add new search/detail DTOs with Chinese field comments and bounded public metadata. |
| `internal/collectioncontract/jmcomic.go:24` | Shared search input normalization pattern, keyword Unicode/control checks and finite pagination. A UAV contract file should own identifier/site allowlists and normalization. |
| `internal/collectionworker/config.go:21` | Trusted configuration only; current fixed adapter has `JMComicEnabled/JMComicPython`, strict boolean parsing at line 64 and safe tool resolution. Add only fixed trusted UAV settings with Chinese explanations. |
| `internal/collectionworker/service.go:44` | Worker service holds fixed tools, two shared web preview slots, root/process lifecycle. Initialize optional adapter alongside JMComic, register private API at line 234, append real capability at line 293. |
| `internal/collectionworker/service.go:297` | Operation start validates canonical preview request before request hash; identical accepted UUID is returned before looking up the current in-memory preview. Preserve this ordering for expired/restarted retries. |
| `internal/collectionworker/service.go:567` | `requestPreview` must copy every new identifier field from operation into preview input; omission breaks hash binding. |
| `internal/collectionworker/jmcomic.go:54` | Pattern for dependency probe, explicit degraded capability, fixed isolated process invocation, search route, metadata preview and verified output acquisition. |
| `internal/collectionworker/tools.go:129` | Central request normalization: add `uav` case and reject UAV-specific fields on other kinds; normalize defaults identically for preview/operation. |
| `internal/collectionworker/tools.go:188` | Shared preview HTTP handler owns concurrency, timeout, finite cache, expiry and immutable input. New adapter uses this unchanged. |
| `internal/collectionworker/tools.go:229` | Metadata preview dispatch; add adapter branch. |
| `internal/collectionworker/tools.go:376` | Existing acquire creates the operation-owned target and enforces write budgets; type dispatch starts at line 424. Re-read and compare preview identity before downloading. |
| `internal/collectionworker/tools.go:75` | Child process environment strips user config/secrets and explicitly sets proxy variables; temp files remain worker-owned. `runTool` at line 105 uses fixed tools without shell, bounded stdout/stderr and owned process tree cancellation. |
| `internal/collectionworker/proxy.go:32` | Authenticated loopback public-network proxy; `publicAddress` at 176, `publicDial` at 195 and `publicIP` at 221 enforce ports, DNS and actual destinations. All search, page, CDN, redirect and subresource traffic must use this. |
| `internal/collectionworker/files.go:276` | Actual manifest inspection after acquisition; reject links/unsafe paths, empty files, limits. Publication at 346 verifies staging independently and persists authorization before rename. Reuse without special bypass. |
| `internal/app/collection_api.go:27` | Register authenticated app-side source search route. Capability fallback kinds at 73, source kind allowlist at 100, fixed-source forbidden parameter validation at 104, and fixed-source test wording at 289 require UAV extension. |
| `internal/app/collection_jmcomic.go:13` | App-side search forwarder pattern: source kind/enabled checks, real worker/root readiness, bounded metadata validation, and source revision recheck after network I/O. |
| `internal/app/collection_preview.go:123` | App preview input fields and fixed source validation. Add new source-specific selector and reject cross-kind use. Worker branch allowlist at 192 and output-profile handling at 210 need extension. |
| `internal/app/collection_preview.go:258` | Persist the entire fixed worker operation request in the private preview snapshot; include source-specific identifiers. Browser receives only sanitized preview. |
| `internal/app/collection_preview.go:312` | Task fingerprint is kind + preview identity + profile + target content type. Existing accepted request handling precedes expiry checks. Preserve exact pending request replay. |
| `internal/app/collection_preview.go:341` | Allow metadata-only empty file lists for UAV like web/JMComic, while requiring video content type near the existing fixed-type check at 355. |
| `internal/app/collection_runtime.go:161` | Independent operation ownership and expected request hash verification; new kind should flow through existing generic worker task state machine. |
| `internal/app/collection_models.go:214` | Existing SQLite task kind is plain TEXT without an enum constraint; sources store JSON. Schema 6 can be preserved with additive optional JSON fields. |
| `web/src/types.ts:319` | `CollectionKind`, new typed search/detail DTOs; all fields require Chinese comments. |
| `web/src/collection.ts:4` | Source display label and local icon map. |
| `web/src/components/CollectionSourceDialog.vue:58` | Reset hidden state when switching kind; defaults at 22/70, strict fixed-source payload at 132, Chinese guidance at 179. |
| `web/src/components/CollectionCenter.vue:84` | Single-modal owner; add explicit UAV entry/dialog, include it in `modalActive`, route source save/preview/action and close it before opening task detail (`taskCreated` at 250). |
| `web/src/components/CollectionJMComicDialog.vue:18` | Search→select→preview→confirm UI pattern with stable request body, latest-search abort guard, expiry timer, disabled editing on uncertain submit and task handoff. Use the neighboring layout rather than redesigning management. |
| `web/src/components/CollectionTargetFields.vue` | Shared target picker already supports `fixed-type`, so UAV can be video-only and reuse actual libraries. |
| `Dockerfile.collection-worker:22` | Install locked optional dependencies at image build, copy third-party notices at 31; adapter image ENV defaults can enable the bundled feature without adding untrusted request configuration. |
| `internal/desktop/docker_compose.go:241` | Strict worker Compose environment allowlist. If UAV is enabled via image ENV, no Compose change is needed. If overlay environment changes, synchronize the exact native map and relevant release tests. |
| `scripts/verify_jmcomic.py:112` | Reusable real app+worker harness pattern, with fixed protocol tool fixture owned by a unique `.runtime/verify` root; distinguishes fixture evidence from real bridge/site acceptance. |
| `scripts/verify_jmcomic_bridge.py:113` | Real installed adapter dependency with controlled offline transport pattern; fixed probe alone is insufficient verification. |
| `scripts/verify_jmcomic_browser.py` | Real UI verification pattern at 375/768/1024/1440, light/dark, one dialog, focus, stale search, empty/error/expiry/uncertain submission, import+reader flow. |

### Proposed fixed API shape

Conditional on upstream site/identifier model, prefer:

```text
POST /api/collection/sources/{id}/uav/search
POST /v1/uav/search
  input: {query:string,page:int,site:<fixed enum only if needed>}
  output: {query,page,total,hasNext,items:[{videoId:string,title:string,...bounded display text}]}

POST /api/collection/previews
  input: {sourceId,uavSite?:<fixed enum>,videoId:<safe upstream identifier>,...optional episode selector}
  existing output: {preview:{kind:"uav",profile:"video",files:[],...}}

POST /api/collection/tasks
  unchanged: {requestId,previewId,contentType:"video",createLibrary,libraryId}
```

If upstream has episodes, a real typed detail/episode list step must precede preview. Do not silently download all episodes, all search results, or the first episode. Fixed selected video/episode identity belongs in the private immutable request. Prefer exposing canonical IDs rather than CDN URLs, image URLs, headers or secrets; only worker resolution produces runtime links.

### Compatibility and safety details

- Every new field in persisted worker `PreviewRequest`/`OperationRequest` needs `omitempty`. Historical zero-valued requests must serialize identically, keeping request/preview hashes and app-worker reconciliation valid. Include all new fields in `requestPreview`, app private snapshot creation and any copy/retry path.
- Keep `collectioncontract.Version=1` and schema 6 if the extension is additive and old kinds retain exact serialization/semantics. Do not add a new media `ContentType`; UAV imports `video`.
- Source save must reject URL, path, rootId, allowPrivate, external connection and command/executable fields for the fixed adapter. Hidden UI fields must be cleared too; backend validation is authoritative.
- Preserve metadata-only previews with empty arrays, never claim bytes/files acquired until actual manifest inspection. Bind preview identity to canonical site/video/episode identity and relevant immutable content facts; user titles must never form paths.
- Existing web yt-dlp helper supports HTTP and DASH only (`webFormat`, tools.go:267), not native m3u8. Never enable ffmpeg network protocols to make HLS work; existing `fetchWeb` limits ffmpeg input to offline file/pipe/crypto/data.
- New HTTP clients must use a controlled transport/proxy for both resolution and downloads. Merely inheriting HTTP_PROXY is insufficient for libraries that ignore environment proxies, use `trust_env=False`, custom sessions or external ffmpeg.
- Download into the one operation-owned empty target, use fixed generated filenames, finite buffers and concurrency, enforce actual streamed byte/file budgets, reject partial HLS segments/truncated output/encrypted unsupported streams, and independently validate actual video output before publication.
- Preserve worker process tree/job ownership, cancellation and shutdown waits. Never call unbounded upstream CLI that starts detached processes or GUI/download-manager workflows.
- Log Chinese events with opaque IDs/counts only; no query text, raw upstream HTML/stderr, direct media links, proxy credentials, accounts or headers.
- Keep existing generic reader and Range streaming; application publication gate and exact import coverage remain authoritative even while worker is offline.

## Minimal change groups

1. Shared contract + worker adapter/config/dispatch/capability, optional locked dependency packaging if required.
2. App search/detail forwarding and strict source/preview/video task integration, preserving existing persistence.
3. Typed UAV dialog, fixed source UI and collection center entry using current components.
4. `docs/uav.md`, `docs/api.md`, `docs/collection.md`, README, third-party notice if redistribution, final `docs/verification.md` and executable Trellis contract/index.
5. Reusable isolated protocol/end-to-end verification and actual-adapter offline verification if dependency code is used; existing Go/Vue/core/collection/release suites. Newly generated temporary unit-test classes/files must be removed after execution per AGENTS.md; reusable integration harnesses may remain.

## Related specs

- `.trellis/spec/backend/runtime-contract.md`
- `.trellis/spec/backend/collection-contract.md`
- `.trellis/spec/backend/jmcomic-contract.md`
- `.trellis/spec/backend/cross-platform-contract.md`
- `.trellis/spec/frontend/app-contract.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

## Caveats / Not Found

- This document deliberately makes no claim about UAV's actual supported sites, search paging, playable URL extraction, license or dependency API; see the dedicated upstream research before choosing request fields or implementation language.
- CodeGraph indexed the latest JMComic files and supplied concrete symbol locations; targeted `rg` was needed for string literals and packaging files. Some model comments still describe the original three kinds, but code and JMComic contract are authoritative; update only relevant comments when editing.
- No production service, external site, video transfer, Docker build, NAS acceptance or browser test was executed for this internal architecture research.
