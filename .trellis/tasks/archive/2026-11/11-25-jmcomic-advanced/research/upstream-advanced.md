# Research: JMComic advanced search, selected chapters, progress and safe page reuse

- Query: Which exact JMComic 2.7.12 APIs/enums can extend the existing fixed adapter, and how can selected chapters, per-image progress and validated retry reuse preserve publication safety?
- Scope: mixed; installed official pinned upstream distribution and the current project adapter. No site traffic.
- Date: 2026-11-25 (Asia/Shanghai)

## Findings

### 1. Exact upstream identity and search support

The inspected source is the real installed `jmcomic==2.7.12` distribution under `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic`. It is already pinned in `deploy/jmcomic-requirements.txt:6` and the embedded adapter checks that exact version in `internal/collectionworker/jmcomic_bridge.py:21,77`. This report does not claim that it is the newest upstream release.

The common `search_site(search_query, page=1, order_by='mr', time='a', category='0', sub_category=None)` delegates to `search(..., main_tag=0, ...)` (`jm_client_interface.py:529-557`). Constants are defined in `jm_config.py:78-134`:

| Parameter | Confirmed values | Recommended exposed labels / constraint |
| --- | --- | --- |
| `order_by` | `mr`, `mv`, `mp`, `tf` | Latest / views / picture count / likes; safe common API subset |
| Additional HTML-only sort constants | `tr`, `md` | Score / comments; upstream explicitly says these are only observed on web, not app (`jm_config.py:84-86`); do not silently expose on API search |
| `time` | `a`, `t`, `w`, `m` | All / today / week / month |
| `category` | `0`, `doujin`, `single`, `short`, `another`, `hanman`, `meiman`, `doujin_cosplay`, `3D`, `english_site` | Must distinguish keyword search from category browsing; see below |
| `sub_category` | `chinese`, `japanese`; `CG` for doujin; `other`,`3d`,`cosplay` for another; `youth` for single | Relevant only when deliberately using and validating the HTML client's supported category combinations |

**Critical: the current API client does not implement category filtering for keyword search.** `JmApiClient.search` explicitly documents that it does not support `category` or `sub_category` and creates request parameters using only `main_tag`, `search_query`, `page`, `o`, `t` (`jm_client_impl.py:806-824`). Passing category to `search_site` on the existing `impl='api'` would produce a misleading no-op.

Three technically honest approaches:

1. Keep the current API search and expose only its four sort values plus four time values. Hide/disable category rather than pretending it works. Smallest risk and old search behavior is retained.
2. For a non-default keyword category, explicitly construct a separate fixed HTML client through the same controlled proxy. `JmHtmlClient.search` builds a category URL and uses actual HTML parsing (`jm_client_impl.py:377-429`). This adds HTML markup parsing and domain discovery acceptance work; it must be verified with a real upstream parser and controlled HTML response, not just a mocked `search_site` method.
3. Add a separate “category browse” mode: `JmApiClient.categories_filter(page,time,category,order_by,sub_category=None)` genuinely sends `c=<category>` but does not use a keyword and ignores subcategory (`jm_client_impl.py:842-864`). It combines time into `o=<order>_<time>`; the source comment only lists view/month/week/day orders, so do not assume every sort combination is site-supported.

The parent was notified immediately about this distinction. Do not label local filtering of a single page as full-result upstream category search.

### 2. Selected chapters through the real downloader

`JmAlbumDetail` sorts its `episode_list` by semantic numeric chapter index and deduplicates that index (`jm_entity.py:538-550`). Album iteration returns placeholder `JmPhotoDetail` objects with chapter ID, title, parent and sort index (`jm_entity.py:553-585`); placeholder chapter `page_arr` is absent, so `len(photo)` is not yet a reliable preview count. `JmDetailClient.check_photo(photo)` fetches metadata when `page_arr`/domain are missing and updates the original object (`jm_client_interface.py:209-229`). This preserves its parent album.

`BaseDownloader.do_filter(detail)` is the native chapter/image selection hook and defaults to identity (`jm_downloader.py:174-184`). `JmDownloader.execute_on_condition` invokes the hook before scheduling (`jm_downloader.py:614-624`). Therefore extend the existing fixed `JmDownloader` subclass:

- On `detail.is_album()`, return only the explicitly selected chapter objects in original album order.
- On photo/image iteration, return the original photo unchanged, so **all images of each chosen chapter** are still mandatory.
- Validate selected IDs for nonempty/unique/bounded digit IDs and membership against the original preview; normalize ordering by upstream album order, not caller input.
- Keep a separate full-album identity and selected chapter list. Do not mutate `album.episode_list`/`page_count` merely to make a success predicate pass.
- For total selected pages/progress, fetch/check only selected chapter metadata within the existing timeout/budgets before transferring any images, then sum `len(photo)` and bind the resulting per-chapter data to that attempt/checkpoint.
- Use the existing numeric directory mapping based on full album order. This leaves ordering stable whether downloading all chapters or a subset.

**Critical: `downloader.all_success` is intentionally false for a chapter-filtered album.** The property compares `len(album)` to the successful photo dictionary and then every photo length to its image list (`jm_downloader.py:186-206`). The source explicitly warns that filters make it false. The existing bridge's unconditional `all_success` and `album.page_count` equality (`internal/collectionworker/jmcomic_bridge.py:227-234`) must be replaced for selected mode, rather than dropped without an alternative.

Required selected-mode completeness checks:

1. No failed image or failed photo entries, and no cancellation.
2. Actual successful chapter ID set exactly equals the normalized selection; no unselected or duplicate chapter can enter staging.
3. For each selected photo, actual successful unique image indices/paths exactly cover its declared `1..len(photo)` sequence.
4. Actual raster file count and byte totals agree with those success records and budgets.
5. Selected chapter metadata and the original full-album preview identity remain unchanged.

`download_photo` is also available (`api.py:285-322`), but looping it introduces separate manifests/top-level calls and does not avoid parent identity checks. Native `download_album` plus controlled `do_filter` is a smaller extension to this adapter.

### 3. Native per-image callbacks and progress

The real synchronous downloader resolves a fixed save path, checks `exists`, sets cache, invokes `before_image`, optionally transfers bytes and **always invokes `after_image` for a cache hit** (`jm_downloader.py:591-612`). The base `after_image` records the image in `download_success_dict` and then runs callbacks/cancellation checks (`jm_downloader.py:280-296`). Existing bridge overrides already validate file/raster/budgets before `super().after_image`.

Recommended trusted progress data: `{completedPages,totalPages,completedBytes,reusedPages}`; no title, source URL, cookie, proxy URL or raw exception. Emit/update it only after a page passes real raster, size/hash and budget checks. Reused pages count as complete but should be distinguishable from bytes newly transferred. Preserve the current single JSON result on stdout; use a separate trusted operation-private progress path or bounded pipe agreed by the Go worker, not unframed arbitrary stdout lines. The application's durable published/imported stages remain authoritative after acquisition.

The current fixed one-photo/one-image concurrency (`jmcomic_bridge.py:137`) makes counters simple; if future bounded parallelism is added, counters/checkpoint updates require a lock and atomic replacement. `DownloadControl.cancel` is a thread-safe native cooperative signal (`jm_task_context.py:27-53`) but cannot replace the worker's process-tree hard stop; an external one-shot worker needs a separately trusted control channel to use it.

### 4. Cache semantics and safe retry page reuse

Upstream `JmOption.decide_download_cache` merely returns `download.cache` (`jm_option.py:317-318`). With it enabled, `JmDownloader` treats `os.path.exists(path)` as enough to skip network transfer (`jm_downloader.py:591-603`). This is **not** hash/raster/identity validation and cannot alone satisfy the project's retry safety requirements.

Use upstream caching only after the fixed adapter has independently proven a cache hit:

- Store checkpoints only under a task-owned private retry area, separate from published media and browser paths. Checkpoint state must not enter the media manifest or be mistaken for an image.
- Bind to album ID, original preview identity, ordered selected chapters, exact page index/extension and a digest of source-image semantics (chapter metadata/page array/scramble identity). A changed source domain/query can conservatively invalidate reuse; never reuse unrelated content because the filename exists.
- Record a page only **after** the output is a no-link regular raster file, dimensions/pixels are valid, file size fits budgets and SHA256 is known. Save checkpoint metadata atomically. A crash between a file write and checkpoint save leaves an untrusted unrecorded page, which must be re-fetched/rejected.
- Before allowing upstream to skip, recheck every path component and final `lstat`, containment, recorded size/SHA256, actual image decoding/validation and source identity. Refuse links/junctions and unknown files, and never follow an unsafe path to delete/overwrite outside owned roots.
- `before_image` can set `image.cache=True` only for a proven hit. Otherwise keep false/re-fetch safely after removing only an identified owned regular file; fail closed for structural hazards. Because `before_image` runs before the upstream cache branch, this requires no third-party monkey patch.
- A reused page still goes through `after_image`, so it contributes to success records, total pages, total staged bytes and progress exactly once. All chosen pages, including reused ones, still pass final Go manifest/image checks.
- On failure keep only validated retry input; cancellation must not publish. On successful final publication or terminal cleanup, remove retry-owned leftovers with the existing worker lifecycle. Do not let failed partial artifacts become public simply to implement resume.

There is no claim of byte-range HTTP resume here: this is reuse of individually completed and verified pages. Partial files are re-downloaded.

### 5. Batch API caveats

Upstream `download_album` treats any non-string/non-int iterable as a native batch (`api.py:234-270`). `download_batch` deduplicates IDs, uses one album task per thread and can create `len(ids)` temporary workers (`api.py:123-194`). The `check_exception` argument only governs a single-ID call; native batch callers must inspect `BatchResult.failed` (`api.py:251-253`, `196-222`).

For this NAS project, retain one normal durable task per album, each with its own preview/UUID/target/publication checks. A bounded UI/API batch should submit/manage those existing tasks and report each result. Do not route a user list directly into `jmcomic.download_album(list)` inside one worker operation: that would bypass per-album budgets, task ownership and clear partial acceptance semantics.

## Files Found

- `internal/collectionworker/jmcomic_bridge.py:71-235` — current fixed options, metadata, downloader hooks and strict all-album completion.
- `internal/collectionworker/jmcomic.go:91-222` — trusted subprocess invocation, search normalization, preview and actual download/identity check.
- `internal/collectioncontract/jmcomic.go` — shared request validation; advanced enums should remain owned by this layer.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/jm_config.py:78-134` — real installed sort/time/category constants.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/jm_client_impl.py:377-429,806-864` — HTML/API search differences and real category browse parameters.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/jm_downloader.py:174-206,280-296,536-643` — selection hook, all-success semantics, callbacks, cache branch and real scheduler.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/jm_entity.py:469-585` — album metadata, ordered chapters and lazy photo details.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/jm_option.py:311-325` — native fixed-path, cache and decode option hooks.
- `.runtime/tools/jmcomic-venv/Lib/site-packages/jmcomic/api.py:123-232,234-322` — native single/batch APIs and failure semantics.

## External References

- Official project: https://github.com/hect0x7/JMComic-Crawler-Python
- Official docs: https://jmcomic.readthedocs.io/
- Exact inspected dependency: `jmcomic==2.7.12`, whose locked wheel identity is recorded in `deploy/jmcomic-requirements.txt` and the first integration task's research.
- This research inspected the actual installed artifact rather than fetching current master. No latest-version or actual-site behavior claim is made.

## Related Specs

- `.trellis/spec/backend/jmcomic-contract.md` — original adapter protocol, full identity, bounded output, no arbitrary Python/YAML, fixed proxy and raster publication checks.
- `.trellis/spec/backend/collection-contract.md` — durable task IDs, partial acceptance/retry, private staging, immutable publication and exact import.
- `.trellis/spec/frontend/app-contract.md` — single modal, bounded controls, stable UUID under unknown response and task handoff.

## Caveats / Not Found

- No real comic search, download, account or public network request was executed; source/transport availability is not established by source inspection.
- API keyword category filtering is not implemented upstream even though the common method accepts its arguments. This is the most important PRD adjustment or implementation choice.
- A selected chapter has no reliable page count until its actual metadata is fetched; do not infer an even share of the album total.
- Existing upstream `all_success` cannot validate a filtered album. Conversely removing it without exact selected coverage would admit partial success.
- Upstream cache is existence-only. It does not supply the immutable, no-link, hash-bound retry checkpoint required by this project.
- This research agent wrote only this report; no production code, task state or existing service was modified.
