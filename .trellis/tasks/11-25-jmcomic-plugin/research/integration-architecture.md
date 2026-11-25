# Research: JMComic integration architecture

- Query: Where should an optional JMComic search/download plugin join the existing Go worker, collection API and Vue UI?
- Scope: internal repository architecture, contracts and test/deployment patterns; upstream Python API is researched separately.
- Date: 2026-11-25

## Findings

### Recommended approach

Add one fixed optional acquisition adapter with `kind: "jmcomic"`, controlled by trusted worker configuration and exposed through ordinary collection sources. Embed a reviewed Python bridge into the worker and invoke one configured Python interpreter with fixed arguments. Reuse collection previews, durable tasks, stage/publish gates, file verification and comic import. This provides the requested plugin behavior without building an arbitrary executable/YAML/plugin marketplace.

Alternatives considered:

1. Extend the `web` audio/video adapter: initially fewer source changes, but the existing web profile/output/type assumptions would become ambiguous and the user could not distinguish plugin readiness. Rejected.
2. Add a separate generic plugin service and task database: isolates dependencies, but duplicates authentication, persistent task recovery, publication and import. Unnecessary for one fixed adapter.
3. Fixed optional `jmcomic` collection adapter: recommended; preserves the existing coordinator and gives clear source/capability labels.

### Database compatibility: no migration required

- `internal/app/collection_models.go:207` defines historical `schemaV3`; `collection_sources(id, body)` stores a JSON source body.
- `internal/app/collection_models.go:214` declares `collection_tasks.kind TEXT NOT NULL` without a CHECK whitelist. Preview bodies and private request snapshots are also JSON.
- Therefore additive `jmcomic` source/task kinds and optional request fields need no schema change: preserve schema head 6 and do not edit historical SQL.
- Actual whitelists are application checks: `internal/app/collection_api.go:99` validates source kinds; `:112` validates the video/audio source output profile; `internal/app/collection_preview.go` independently validates preview profile and filters task files. Update these deliberately.
- `collectionCreateTask` checks request UUID ownership before preview expiry/source-revision rejection; preserve this ordering for uncertain POST retries. `internal/app/collection_store.go:278` also enforces fingerprint and target intent uniqueness.

### Proposed cross-layer DTO contract

Source configuration:

```json
{"kind":"jmcomic","name":"JMComic 漫画","enabled":true,"rootId":"","path":"","outputProfile":""}
```

`name` and `enabled` are user-configurable. The plugin has fixed upstream endpoints; do not accept arbitrary Python code, module names, shell parameters, URLs, cookies, YAML or output paths. Reject unrelated source fields rather than interpreting them as plugin configuration.

Search:

```text
POST /api/collection/sources/{id}/jmcomic/search
POST /v1/jmcomic/search                 (private authenticated worker)
request: { query: string, page: number }
response: {
  query: string,
  page: number,
  total: number,
  hasNext: boolean,
  items: [{ albumId: string, title: string, tags: string[] }]
}
```

Use a bounded query (e.g. <=200 runes), positive bounded page, bounded result count (e.g. <=80), safe bounded titles/tags and `[]` instead of null. Search only with a configured, enabled `jmcomic` source and available worker capability. Keep empty success different from timeout/unavailable. Do not render upstream links or HTML.

Download preview and task reuse:

```text
POST /api/collection/previews
{ sourceId, albumId: "123456", profile: "comic", mode: "copy" }
→ existing { preview: CollectionPreview }

POST /api/collection/tasks
{ requestId, previewId, contentType: "comic", createLibrary, libraryId }
→ existing { task: CollectionTask }
```

- Add `AlbumID string json:"albumId,omitempty"` to shared worker `PreviewRequest` and `OperationRequest`. Update `service.go:564 requestPreview` to retain it. Include it in request/preview hashes and source identity.
- `omitempty` is important: old persisted request hashes must serialize exactly as before when no album ID exists.
- Treat IDs as digit strings with a strict length and positive-value rule; never accept paths/URLs as IDs. The upstream bridge receives only the fixed numeric identity.
- Preview returns kind `jmcomic`, profile `comic`, actual upstream title and stable identity, but may have no files because acquisition has not happened. Permit this exception alongside existing web previews.
- Force task content type `comic`; reject video/music/image/novel targets even if a forged request bypasses the UI. The usual target library/create authorization still applies.
- No worker protocol major-version bump is necessary for additive kind and optional fields; an old worker advertises no `jmcomic` capability and remains unavailable for it.

### Worker implementation map

| File / location | Existing purpose | Change / reuse |
| --- | --- | --- |
| `internal/collectioncontract/models.go` | Version 1 fixed DTOs | Add search/result DTOs and optional album ID fields; preserve JSON stability for old kinds. |
| `internal/collectionworker/config.go:23` | Trusted environment config, validated tool paths | Add opt-in enable flag and configured Python interpreter path with Chinese comments; missing optional interpreter should disable the capability rather than break service startup. |
| `internal/collectionworker/service.go:105` | Probe tools and recover durable operations | Resolve/probe Python and bridge module imports once; report actual compatible version. |
| `internal/collectionworker/service.go:227` | Bearer-authenticated private routes | Register bounded fixed search endpoint. |
| `internal/collectionworker/service.go:275` | Actual capabilities | Append `jmcomic` with explicit disabled/dependency/version reason. |
| `internal/collectionworker/tools.go:130` | Fixed-kind validation | Add numeric album-only request validation; reject unrelated URL/path/source parameters. |
| `internal/collectionworker/tools.go:220` | Preview kind dispatch | Dispatch fixed JM metadata preview and budget checks. |
| `internal/collectionworker/tools.go:369` | Bounded isolated acquisition | Dispatch JM download into the already created operation staging directory, revalidate album identity and run common final inspection. |
| `internal/collectionworker/tools.go:75,105` | Sanitized env and bounded process stdout/stderr | Reuse `runTool`, fixed interpreter args, JSON stdin, private staging CWD, no shell. |
| `internal/collectionworker/proxy.go:32` | Public HTTP(S) proxy with DNS/IP policy | Reuse for all Python network calls, explicitly force the JM postman/client to use it. Environment alone may be insufficient. |
| `internal/collectionworker/files.go:276,328` | File allowlist, version facts and publication | Reuse unchanged where possible. Output ordinary supported raster pages in numeric chapter/page directories. |
| `internal/collectionworker/retry.go:12` | Retry original stopped operation | Reuse original request and UUID; no new download coordinator needed. |

Suggested new files: `internal/collectionworker/jmcomic.go` plus an embedded `jmcomic_bridge.py`. Use an exact bridge protocol for `probe/search/preview/download`; read/write bounded JSON, discard unreviewed upstream stdout/stderr from public DTOs and emit only fixed Chinese business failures. The bridge may customize internal JM classes to force bounded downloads, but must not enable user-supplied upstream Python/YAML plugins.

### Cancellation, budgets and publication

- `tools.go:369 acquire` creates `staging/<UUID>`, applies a 45-minute operation deadline, reserves disk and runs a 500ms budget monitor over all files including partials. Integrate inside this method rather than launching an unrelated goroutine/process.
- `process_windows.go:20,38` starts the configured command suspended, assigns it to a private kill-on-close Job, resumes and waits for all descendants. `process_unix.go:16,21` uses a private process group and confirms cancellation/normal-exit cleanup. Reuse these paths for Python.
- Parent completion and cancellation alone do not establish publication. Preserve the existing `queued → running → acquired → publishing → published` worker facts, app independent validation and exact directory-comic coverage checks.
- `internal/app/collection_import.go:123 collectionImportTargets` already aggregates images by direct parent directory for comic items. A numeric chapter directory with zero-padded page files naturally produces one readable item per chapter.
- Multi-chapter output must preserve deterministic directory and page order; do not place all album pages under one directory if chapter grouping is intended.
- `tools.go:276 previewWeb` assumes metadata only. A JM preview likewise must not download pages or create a durable task.
- `internal/app/collection_worker.go:20 collectionWorkerCall` uses a client configured with 25-second timeout in `collection_runtime.go:18`; choose search/preview budgets that fit or explicitly reconcile the timeout across the boundary. Current worker preview budget is 75 seconds, so do not mistakenly promise all such requests can complete through the existing app client.
- Runtime proxy rejects private/reserved IPs and fixed-port violations. Never disable TLS verification or bypass the policy because an upstream library has a different default HTTP stack.

### Application implementation map

| File / function | Required work |
| --- | --- |
| `internal/app/collection_api.go:18 collectionRoutes` | Add authenticated per-source search handler. |
| `collection_api.go:56 collectionCapabilities` | Include unavailable `jmcomic` when worker is absent, preserve clear reasons. |
| `collection_api.go:93 collectionSaveSource` | Accept fixed JM kind, disallow irrelevant fields and retain ordinary revision checks. |
| `collection_api.go:236 collectionTestSource` | Existing capability branch can validate JM; avoid claiming real upstream connectivity if only import/version probe ran. |
| `collection_preview.go:137 collectionCreatePreview` | Accept numeric album ID and comic profile for JM, call private worker preview, retain private immutable request. |
| `collection_preview.go:286 collectionCreateTask` | Permit JM fileless preview; require comic target; preserve UUID/fingerprint ordering. |
| `collection_models.go` | Update comments/DTOs without changing SQL. |
| `collection_runtime.go:82 runCollectionTask` | Existing non-BT/non-register path already transports private OperationRequest and checks request hash, publishes and imports. Should not need a parallel JM-specific task runner. |
| `collection_worker.go` | Reuse bounded authenticated calls, root challenge and file verification. |

### Vue entry and behavior

- `web/src/types.ts:319 CollectionKind` and `web/src/collection.ts:4,6` are central type/label/icon maps. Extend with `jmcomic` and add the precise search DTOs with Chinese field comments.
- `CollectionCenter.vue:81 modalActive` explicitly owns all current modal state. Add one JM dialog state, overview entry and source action. Exclude JM from generic `CollectionPreviewDialog` source choices so its video/audio defaults cannot submit malformed requests.
- `CollectionSourceDialog.vue:120 save` already normalizes fields by kind. Add a simple JM explanation/readiness state; no directory selector or video/audio output field. The generic source select derives its kinds from label maps.
- A dedicated `CollectionJMComicDialog.vue` should combine search, manual album number and inline preview/target confirmation inside one native dialog. Reuse `CollectionTargetFields` with fixed comic type, ordinary immutable UUID task submission, and existing task detail navigation.
- Abort superseded searches and guard late responses; clear selected preview when the query/source/album changes. Keep a failed query different from empty results. While task POST is uncertain preserve the exact `requestId`, preview and target values.
- `CollectionCenter.vue:238 taskCreated` closes prior dialogs then awaits `nextTick` before opening task detail; extend this same path rather than nesting dialogs.
- Keep the five mobile bottom entries; no global navigation redesign needed. Existing `collection.css` dialog padding and semantic theme should be reused.

### Deployment and dependency lock

- `Dockerfile.collection-worker:7–10` copies the entire worker package before building. An embedded Python file inside that package is included automatically; no loose runtime script path is needed.
- Runtime already has Python 3 + venv at `/opt/collection`, but `deploy/collection-requirements.txt:2` currently pins only yt-dlp.
- Its pip command is `--no-deps --require-hashes`; adding just `jmcomic==...` cannot install its transitive dependencies. Pin the complete dependency graph with tested platform-compatible wheel/sdist hashes, or add a separate reviewed hash-locked requirements file installed into the same/own venv during image build.
- Docker config should point at `/opt/collection/bin/python` (or separate JM venv) and explain opt-in enable semantics in Chinese. Do not run pip from GET/capability/startup handlers.
- Native Windows/Linux worker deployments need a documented dedicated venv install command and exact `NAS_COLLECTION_WORKER_*` interpreter setting. Native app-only development does not currently start a worker automatically; document this requirement explicitly.
- `compose.desktop.collection.yaml`/desktop normalized service spec has a strict environment whitelist per cross-platform contract. Avoid adding arbitrary Compose overrides to it without updating validator and release regression fixtures. A default-disabled JM adapter can be configured in the existing trusted deployment path, or enabled by the reviewed worker image defaults if that choice is documented and validated.

### Meaningful verification plan

1. Build real Go app/worker, run existing collection verifier as a regression.
2. Add a reusable integration verifier patterned after `scripts/verify_collection.py` and its `CollectionHarness` (`:90`), creating isolated roots, generated raster pages, temporary ports, private token and separate app/worker processes. Never use the user's live DB/data or upstream comic content.
3. Run the real embedded Python bridge against a deliberately controlled temporary JM module/interpreter environment, label that evidence as a protocol fixture rather than real upstream acquisition. Verify search, metadata, multi-chapter pages, persistent task, explicit publication, comic import and exact reader bytes.
4. Separately import/probe the exact real JM package in a project-local venv without network comic acquisition; this proves dependency install and bridge compatibility probe only.
5. Cover disabled/missing/wrong dependency, malformed numeric IDs, unknown fields, invalid query/page, no session/foreign origin, false capability, source disabled/stale revision, expired preview, fixed-ID task retry, cancellation, non-image output, unsafe paths, zero/over-budget pages and output sanitization.
6. Browser checks should exercise the real isolated app UI for source creation, search, empty/error states, manual ID, target confirmation, task details and imported comic item; both themes at 375/768/1024/1440 and keyboard/Escape. If intercepted responses are used for edge states, mark them as presentation evidence only.
7. Run `go vet ./...`, `go test ./...`, `go build` for app/worker, Vue typecheck/build, relevant release/desktop checks when deployment changes. Run all local commands through `python -B scripts/project_env.py -- ...`; remove newly generated temporary unit-test files after execution but preserve the reusable integration script.

## Files and related specs

- `.trellis/spec/backend/runtime-contract.md`: auth, same-origin protection, schema preservation, bounded requests, shutdown and media boundaries.
- `.trellis/spec/backend/collection-contract.md`: authoritative worker/app state machine, publication and exact import coverage.
- `.trellis/spec/backend/cross-platform-contract.md`: native process ownership and immutable/strict desktop packaging.
- `.trellis/spec/frontend/app-contract.md`: one modal, type safety, request cancellation, polling and responsive controls.
- `docs/collection.md`: deployment/configuration and real worker verifier commands.
- `docs/api.md`: endpoint documentation to extend.

## Caveats / Not Found

- `.codegraph/codegraph.db` exists, but `Get-Command codegraph` found no CLI and the enabled tool catalog contains no codegraph MCP action. Used targeted `rg` and native file reads as fallback; did not alter or regenerate the graph.
- Upstream JM APIs, package version and transitive dependency hashes are intentionally left to `research/jmcomic-upstream.md`; this report makes no claim that actual upstream downloads were tested.
- Existing `collectionWorkerReady` proves worker identity and shared-root challenge plus tool availability, not reachability of the JM site. UI/source test wording should preserve that distinction.
- A Docker image build or Go cross-compilation does not prove a real NAS filesystem, ACL, external network or actual user content acquisition. Report remaining deployment boundaries accurately.
