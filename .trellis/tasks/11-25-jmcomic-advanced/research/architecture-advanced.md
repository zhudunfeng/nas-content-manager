# Research: JMComic advanced cross-layer integration

- Query: Extend the current fixed JMComic adapter with bounded search filters, chapter selection, independent batch tasks, page progress and safe explicit retry reuse.
- Scope: internal repository architecture; no real source requests, user content or runtime instance changes.
- Date: 2026-11-25

## Findings

### Existing authority and code intelligence

The worker remains the only Python executor. The application owns source revision, expiring public preview, persisted task identity, publication authorization and actual import. The worker owns immutable operation input, per-operation staging, acquired file inspection and same-volume publication. There is no reason to introduce another task coordinator or database schema.

CodeGraph is now installed as `D:/adunsoftware/nodejs/codegraph.ps1`. Invoking `codegraph` directly through `project_env.py` resolves the Windows `.cmd` shim, which the project wrapper intentionally refuses. The native entry point works:

```powershell
python -B scripts/project_env.py -- D:/adunsoftware/nodejs/node.exe D:/adunsoftware/nodejs/node_modules/@colbymchenry/codegraph/npm-shim.js query requestPreview
python -B scripts/project_env.py -- D:/adunsoftware/nodejs/node.exe D:/adunsoftware/nodejs/node_modules/@colbymchenry/codegraph/npm-shim.js impact retryHTTP --json
```

`query collectionCreateTask`, `query requestPreview`, `callers collectionCreateTask` and `impact retryHTTP` were executed. The index is older than current UAV changes: e.g. it reports `collectionCreateTask` at line 282 while current disk begins at 305. Use CodeGraph to locate symbols; use bounded current file reads/`rg -n` to verify line evidence. Do not synchronize the index as part of this read-only research.

### 1. Search filters

`JMComicSearchRequest` currently contains only `query` and `page` (`internal/collectioncontract/models.go:89`). The shared validator in `internal/collectioncontract/jmcomic.go` is the correct whitelist owner for both app and worker. `internal/app/collection_jmcomic.go` already rechecks source revision/enabled state after worker I/O; preserve that check. `internal/collectionworker/jmcomic.go:157` builds a trusted bridge request and validates the response envelope.

Add optional `orderBy`, `time` and `category` fields to the request, normalize only the pinned version's accepted literal values, and pass named arguments to real `search_site`. Default empty values must behave exactly like old query/page requests. Keep a bounded UTF-8 query, page maximum 1000 and at most 100 results. If response fields echo normalized filters, validate them instead of silently accepting a response for another query/filter combination.

### 2. Chapter choice should be part of the preview, not a mutable task option

Recommended request shape:

```text
POST /api/collection/previews
{sourceId, albumId, chapterIds?: string[]}

worker PreviewRequest and OperationRequest
...existing fields..., chapterIds?: string[]

Preview additional field
jmcomic?: {
  albumId: string,
  totalPages: number,
  selectedPages: number,
  chapters: [{id:string,title:string,index:number,pageCount:number,selected:boolean}]
}
```

`chapterIds` should use `omitempty` in both Go request structs; empty means the historical full album. Validate positive numeric IDs, reject duplicates and unknown IDs, and normalize a supplied subset into upstream chapter order. A chapter selection change must invalidate the displayed preview and obtain a new bound preview before confirmation. Keep `/tasks` input unchanged; it already accepts only preview ID, stable request UUID and target authorization (`internal/app/collection_preview.go:305`).

The complete propagation path is mandatory:

1. `collectionCreatePreview` strict input and source-kind check (`collection_preview.go:133`).
2. Shared `PreviewRequest` -> `previewJMComic` -> trusted Python request.
3. App `collectionPrivate.Request` construction (`collection_preview.go:281`).
4. `requestPreview` extraction (`internal/collectionworker/service.go:570`).
5. `requestHash`/`previewHash` (`service.go:575,583`).
6. Download selection and final expected count/order check.

Do not add the field only in the UI or only in the subprocess. IDs must be rejected on non-JMComic sources just as album IDs are rejected today.

#### Preserve historical identities

The current worker computes preview identity as `digestJSON(jmcomicAlbum)` after clearing title (`jmcomic.go:210`), and final download clears title/files before comparison (`jmcomic.go:225`). Adding metadata detail fields directly to this struct and digest changes the identity of every old full-album operation, even if the new public request field is omitted. Use an explicit identity value with exactly the legacy serialized fields for the empty/full legacy case; append selected identity fields only with `omitempty`. Alternatively support a deliberately versioned legacy comparison, but do not silently declare all historical failed tasks changed.

App fingerprints are based on `private.Identity`, profile and content type (`collection_preview.go:335`). Thus a correct selection-bound identity gives distinct tasks for distinct chapter subsets, while the same UUID with a changed selection must still conflict. Request JSON compatibility alone is not enough: verify historical serialized bytes, worker request hash, preview hash, full-album identity and app fingerprint.

#### Large albums and bounded metadata

The existing `previewJMComic` rejects the entire album when total page count exceeds request `MaxFiles` (`jmcomic.go:190`). If this is retained before returning chapters, a user cannot discover a large album's chapters in order to select a smaller allowed subset. Choose an explicit design:

- A separate read-only chapter catalogue request bounded by an absolute metadata limit; selected-page budget enforced when creating the actual preview; or
- A catalogue-bearing preview that visibly cannot be submitted while over budget; selection then creates a fresh budget-valid preview, with enforcement at task/operation creation.

Do not apply the full-album page budget to a selected small subset, and do not relax the actual download budget merely to show metadata. The existing bridge output cap is 256 KiB (`jmcomic.go:109`), so bound chapter count and title bytes or use a separate bounded catalogue. A 1000-chapter response with long UTF-8 names can exceed the cap even when all individual values are bounded.

### 3. Batch should orchestrate existing per-album APIs

Pure frontend orchestration is sufficient and minimizes protocol/state changes:

- Accept 1–20 IDs (example bound; choose and document one value), trim separators, reject nonnumeric/duplicate entries before network I/O.
- Read independent previews with bounded sequential or at most two-way concurrency, show each result/title/error and let the user confirm the actual items.
- Use one immutable task body and `createRequestId()` per item. Freeze source, chapter choice, preview and library authorization once that item's submit begins.
- Keep successful items and task IDs visible. An error on one item must not reissue successful tasks or cause a newly generated UUID for an unknown result.
- For transport/5xx unknown outcomes, retry exactly that item's stored UUID/body before any preview refresh. A 409 duplicate is a conflict to inspect, not permission to submit with another UUID.
- Closing the dialog may abort local HTTP waits but cannot promise it canceled an already accepted server task. The task list remains the durable recovery surface.

`CollectionJMComicDialog.vue:100` already preserves a single `TaskInput`; extend that pattern to a per-item record. `CollectionCenter.vue`'s single-dialog navigation means immediately emitting `open-task` on the first successful item will hide the remaining batch; retain batch results until the user chooses an individual task or closes.

### 4. Page progress should have its own DTO

Recommended additive model:

```text
JMComicProgress {completedPages:int,totalPages:int,bytes:int64,reusedPages:int}
Operation.jmcomicProgress?: *JMComicProgress
CollectionTask.jmcomicProgress?: *JMComicProgress
```

`Operation.Bytes` is currently finalized by summing `Files` inside `execute` (`service.go:468`). Updating it during transfer without changing completion logic doubles the total. Preserve its existing final-product meaning. Progress bytes should mean verified output bytes in the current attempt, including explicitly identified reused bytes/pages, not a claimed total remote download size. Each verified page advances progress only after image/budget validation.

The existing process runner returns stdout only on process exit (`tools.go:103`) and allows exactly one JSON object. Keep that protocol rather than parsing upstream console logs. A generated private progress sidecar plus bounded polling is the smallest coherent extension:

- Worker chooses every sidecar path; browser and app never provide it.
- Write atomic snapshots after verified pages (or bounded throttling) to a per-operation private directory.
- Parse with strict bounded JSON; validate nonnegative counters, `completed<=total<=MaxFiles`, `reused<=completed`, bytes<=MaxBytes and monotonic progress within the attempt.
- Worker operation GET can merge current progress under its operation lock while checking current ID/state/attempt. Do not let an old process/snapshot overwrite a subsequent retry.
- App task detail can request the original worker operation on the existing 3-second detail polling cycle and attach only validated progress after checking operation ID, kind and immutable request hash. Treat unavailable progress as unavailable, never as task failure. Keep the normal task list/DB read available if worker is offline.

`CollectionTask` fields are explicitly scanned/persisted in `collection_store.go`; simply adding a struct pointer will not persist it. Attaching transient progress in `collectionGetTask` (`collection_api.go:601`) avoids a schema change, per-page task revision churn, CAS conflicts with cancel and a flood of task events. If persistent progress is needed, deliberately use an existing JSON record or separate bounded private snapshot and document reset semantics. Progress is observational and is never publication authority.

### 5. Safe reuse requires changes at both retry and acquisition

Current behavior always destroys partial staging:

- `retryHTTP` checks failed/canceled and at most three attempts, validates staging path, then calls `os.RemoveAll` (`retry.go:35–58`).
- `acquire` requires `os.Mkdir(staging/<id>)` success, so any retained directory is rejected (`tools.go:403`).
- The bridge requires an empty operation-owned working directory (`jmcomic_bridge.py:103`).

For JMComic only, explicit failed retry may preserve a strictly verified operation-owned directory. Other adapters must retain their current clean-start behavior. A resumed directory must pass all existing root/link/alias checks; use canonical root and `os.OpenRoot`/bounded enumeration where available. No external path, previous operation or generic user cache is admissible.

A defensible fixed cache record must bind:

- Operation ID / immutable request hash / attempt lineage.
- Album, selected chapters in upstream order, per-chapter page count and stable per-page upstream identity.
- Fixed numeric relative output paths and format.
- Verified file size plus SHA256 for every completed page.

On retry, compare current source metadata and the frozen selection before reusing pages. Independently re-open each retained ordinary raster page, confirm path, image validity, size and SHA256. Missing or uncommitted pages can be fetched again; corrupted cached pages may be explicitly discarded and refetched if ownership is proven. Unexpected files, links, changed metadata or an untrustworthy checkpoint must reject reuse or invoke a clearly defined clean reacquisition. File existence alone is insufficient, and `download.cache=True` alone does not establish the contract.

Keep control data outside media staging. `inspectFiles` refuses every unsupported file (`files.go:280`) and `publishHTTP` re-enumerates the exact acquired manifest (`files.go:325`). A checkpoint or progress JSON left in staging will prevent publication, and excluding it broadly would weaken existing publication rules.

**Critical startup caveat:** `Service.New` treats every root-level `private-state/*.json` as a saved operation (`service.go:170–195`). Sidecars must therefore live in a subdirectory, e.g. `private-state/jmcomic/<operation-id>/`, not `private-state/<id>-progress.json`. Otherwise worker restart fails parsing sidecar JSON as operation history.

`checkWriteBudget` counts all current staging bytes including partial files (`tools.go:550`), and `reserveSpace` currently reserves the entire configured request size before starting (`files.go:479`). Preserve actual total-byte/file/free-space checks while resuming; a request budget cannot become a per-attempt allowance. Reserving full max bytes again is conservative and may reject an otherwise feasible resume; do not subtract retained bytes unless measured safely without exceeding budget.

Final success must still verify all selected expected pages, exact file list, raster integrity, upstream completion/failure collections and expected identity before `acquired`. `acquired` is private until app validation and explicit publication. Canceling or failed partial output must remain unreadable. A progress sidecar saying 100% cannot authorize publish.

### 6. Recovery and concurrency constraints

- Worker restart converts `running/canceling` to `recovery_required` (`service.go:193`), and current retry excludes this state (`retry.go:27`). Keep this separate from known stopped failure reuse. Do not introduce automatic download restart under the guise of cache recovery.
- `retryHTTP` refuses an operation present in `active` before touching its directory (`retry.go:20`). Preserve this and include process-tree cleanup before any reuse.
- `execute` takes a snapshot then reloads current state after I/O (`service.go:451`), so progress writes must not overwrite a concurrent cancel or another attempt.
- Parent application serializes coordinator work, but task detail can read concurrently. Never hold the worker service mutex over filesystem hashing, Python I/O or network requests.
- App task state revision is a real optimistic concurrency token. Progress updates should not repeatedly invalidate a user's cancel/retry button token.

## Files Found

| File | Responsibility |
| --- | --- |
| `internal/collectioncontract/models.go` | Additive request, metadata and progress wire models; omitempty compatibility |
| `internal/collectioncontract/jmcomic.go` | Shared bounded validators for search and chapter IDs |
| `internal/app/collection_jmcomic.go` | Source-scoped search and revision recheck |
| `internal/app/collection_preview.go` | Strict preview input, worker mapping, immutable private request, task fingerprint |
| `internal/app/collection_models.go` | Public preview/task additive metadata |
| `internal/app/collection_api.go` | Bounded optional task-detail progress attachment |
| `internal/app/collection_runtime.go` | Existing request-hash ownership, acquisition/publish/import lifecycle |
| `internal/app/collection_store.go` | Explicit task persistence and duplicate fingerprint rules |
| `internal/collectionworker/jmcomic.go` | Trusted bridge input, metadata/identity, final count/raster validation |
| `internal/collectionworker/jmcomic_bridge.py` | Real pinned upstream calls, selected chapters, per-page hooks and private checkpoint |
| `internal/collectionworker/retry.go` | JM-only retained-staging retry after owned process exit |
| `internal/collectionworker/tools.go` | JM-only existing staging allowance, strict environment and total write budgets |
| `internal/collectionworker/service.go` | Operation public progress and attempt/state safety; requestPreview propagation |
| `web/src/components/CollectionJMComicDialog.vue` | Fixed search filters, chapter selection, per-item batch state/unknown retries |
| `web/src/components/CollectionTaskDialog.vue` | Existing polling and page-progress presentation |
| `web/src/types.ts` | Frontend response types |
| `scripts/verify_jmcomic*.py` | Existing protocol, real dependency and browser checks to extend without conflating evidence |

## Related Specs

- `.trellis/spec/backend/collection-contract.md`: immutable operation, strict preview/task, explicit publication, independent import and local/worker ownership.
- `.trellis/spec/backend/jmcomic-contract.md`: pinned bridge, numeric paths, complete comic validation and legacy JSON contracts.
- `.trellis/spec/backend/runtime-contract.md`: request size, Chinese errors/logs, runtime lifecycle and media boundaries.
- `.trellis/spec/frontend/app-contract.md`: modal/polling lifecycle, stale response and unknown submission handling.
- `.trellis/spec/guides/cross-layer-thinking-guide.md`: input and identity propagation through every layer.

## External References

- User-selected upstream: `https://github.com/hect0x7/JMComic-Crawler-Python`.
- Installed pinned dependency: `jmcomic==2.7.12`; exact upstream API/hook behavior is delegated to `research/upstream-advanced.md`.
- This research makes no new claim about current upstream release, real site availability, accounts, domain behavior or content.

## Caveats / Not Found

- No production code, tests, database, index, current-task state, user services or media were modified by this researcher.
- No real source requests were made. Internal control flow inspection is architecture evidence only, not runtime acceptance.
- The PRD says failed-page recovery; it must not be labeled arbitrary byte-range resume. The practical unit is a fully verified completed image/page.
- Bounds and exact enum DTO names above are recommendations to freeze with the upstream research before implementation.
- Existing CodeGraph line numbers are stale; cited disk file positions were checked with current targeted reads/rg.
