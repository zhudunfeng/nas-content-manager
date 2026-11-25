# JMComic plugin contract

## 1. Scope / Trigger

Read this contract for the fixed `jmcomic` collection source, Python bridge,
search API, worker packaging, or comic-image publication. Also read
[Collection contract](collection-contract.md). This is a fixed optional adapter,
not a runtime plugin-upload system. The application does not execute Python.
SQLite stays at schema 6: sources use existing JSON and tasks use existing kinds.

## 2. Signatures

```text
POST /api/collection/sources              # existing source CRUD, kind=jmcomic
POST /api/collection/sources/{id}/test     # dependency/shared-root check only
POST /api/collection/sources/{id}/jmcomic/search
POST /v1/jmcomic/search                   # private authenticated worker API
POST /api/collection/previews             # {sourceId, albumId}
POST /api/collection/tasks                # existing durable task protocol
```

The shared Go contract owns `ValidJMComicAlbumID(string) bool` and
`NormalizeJMComicSearch(*JMComicSearchRequest) error`. New `AlbumID` members on
persisted preview/operation request types MUST use `json:"albumId,omitempty"`:
the empty value must not change hashes for historical operations.

## 3. Contracts

### Source, search, preview and task

- Sources accept a name and enabled flag. Directory, URL, private-network
  permission, external service/connection, and connection-reset fields are
  rejected. The generic form's `outputProfile` is normalized to empty.
- Search request: `{query:string,page:int}`. Trim query, require 1–120 Unicode
  characters, reject control characters. Page 0 defaults to 1; allow 1–1000.
- Response: `{query,page,total,hasNext,items:[{albumId,title,tags}]}`. Preserve
  requested query/page, total 0–100000000, at most 100 items and 20 tags per item.
  Empty items/tags are JSON arrays. Display text is bounded plain text.
- `albumId` is a string matching `[1-9][0-9]{0,11}`. Never accept an arbitrary
  upstream URL, YAML, code, proxy, credential, output path, or command argument.
- Preview uses `kind=jmcomic`, `profile=comic`, metadata-only empty file list,
  and an identity bound to album ID, ordered chapter IDs and page count. User
  titles never participate in paths. Preview TTL and source revision still apply.
- Task creation requires `contentType=comic`, the original preview and a stable
  request UUID. Unknown submission results must retry the identical body.
- Keep the existing state machine, cancel/retry, events, manifest/publication,
  exact import coverage and read authentication. For comic image directories,
  image pages must survive collection type filtering; scanner aggregation then
  creates ordered chapter comics. Do not change global image classification.

### Worker environment and bridge

| Environment key | Meaning / default |
| --- | --- |
| `NAS_COLLECTION_WORKER_JMCOMIC_ENABLED` | Strict `true`/`false`; native default `false`; dedicated Docker image `true` |
| `NAS_COLLECTION_WORKER_JMCOMIC_PYTHON` | Trusted interpreter; native Linux `python3`, Windows `python`; Docker `/opt/jmcomic/bin/python` |

Pin `jmcomic==2.7.12` and its base dependency closure in
`deploy/jmcomic-requirements.txt`, install with `--only-binary=:all: --no-deps
--require-hashes`, and run `pip check`. Requests never install dependencies.
The Docker image includes `docs/third-party-notices.md` with the upstream MIT
notice. Native distribution does not silently install Python.

The embedded bridge is invoked without a shell using `-I -B -c <fixed script>`.
Input is bounded JSON; output is one bounded strict JSON object. `probe` returns
`{version:"2.7.12",protocolVersion:1}` without contacting the site. Other actions
are `search`, `album`, and `download`; only the worker creates their parameters.
The worker creates an authenticated loopback public-network proxy and explicitly
sets it for upstream bootstrap, API and image traffic; inherited environment
proxy exclusions cannot grant private-network access.

Album output is `{albumId,title,pageCount,chapters:string[],files:int}`. Metadata
uses `files=0`. Download gets an empty operation-owned absolute staging directory,
`maxBytes` and `maxFiles`; fixed numeric chapter/page names avoid title traversal.
Use real upstream downloader hooks, finite concurrency, decoded JPEG/GIF pages,
per-image raster validation, budgets, `check_exception=True`, `all_success`,
failure collections, actual file count, page totals and unchanged metadata.
The Go worker independently validates its manifest and raster data before
publication. Partial success, extra non-images and identity drift cannot publish.

Logs report Chinese business events, IDs and bounded counts; never log search
terms, upstream links, proxy credentials, raw stderr or user authentication.

## 4. Validation & Error Matrix

| Condition | Expected result |
| --- | --- |
| Missing application session / worker token | 401 |
| Cross-origin application write | 403 |
| Invalid query/page/ID, unknown JSON member, forbidden source parameter | 400 |
| Missing or wrong-kind source | 404 |
| Disabled source or changed revision after search | 409 |
| Worker search concurrency exhausted | 429 |
| Plugin disabled or dependency unavailable | 503, other worker capabilities remain available |
| Failed/malformed upstream response | 502, never a successful empty result |
| Partial pages, corrupt/non-image output, changed identity, exceeded budget | Failed operation, no published/imported item |
| Cancellation | Stop owned process tree; no staging publication |

## 5. Good / Base / Bad Cases

- Good: search page 1, preview a selected string ID, confirm a comic library,
  acquire all chapters, validate, publish, scan, and read authenticated images.
- Base: plugin disabled or missing Python; collection history and existing comic
  readers remain available. No database migration or dependency installation.
- Bad: pass a URL as ID, change comic content type to video, retry with a modified
  UUID body, or mark three of four pages as complete. Reject each at its boundary.

## 6. Tests Required

- `scripts/verify_jmcomic.py`: real app and worker with a protocol tool fixture;
  assert auth/origin, paging/empty/error, validation, fixed source rules,
  metadata-only preview, comic-only tasks, identical retry, one download,
  complete four-image import/read, invalid output isolation, retry/cancel,
  restart persistence, schema 6, disabled/missing/offline capability and local
  task compatibility. This fixture does **not** execute the Python bridge.
- `scripts/verify_jmcomic_bridge.py`: exercise the embedded bridge with the installed pinned upstream package and
  controlled offline transport: real search entities, downloader hooks, image
  conversion, partial failure, budgets and fixed path ordering. Record precisely
  where transport is substituted. Probe/import alone is insufficient.
- `scripts/verify_jmcomic_browser.py`: real application flow at
  375/768/1024/1440 widths, light/dark themes, no overflow, one dialog,
  Escape/focus restoration, search states, preview invalidation, fixed comic
  target, task handoff and real reader page load. Mark intercepted UI-only
  submission/expiry/concurrency responses separately from backend evidence.
- Run original collection/core/release regressions, Go vet/test/build and Vue
  typecheck/build. Remove only temporary unit tests generated for this task.
  Do not describe fixture, wheel-resolution or cross-build results as live-site,
  Docker or physical NAS acceptance.

## 7. Wrong vs Correct

Wrong: expose a free-form Python/YAML/URL plugin configuration, trust the
downloader's return value as complete, and import every file in staging.

Correct: accept a validated album ID, run the fixed isolated bridge through the
controlled proxy, compare preview identity and all page counts, independently
validate actual raster files, and use the original durable publication gate.
