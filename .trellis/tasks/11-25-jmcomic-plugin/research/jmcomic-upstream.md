# Research: JMComic official Python API and controlled worker adapter

- Query: Integrate the user-selected JMComic-Crawler-Python repository as an optional comic search/download plugin without exposing arbitrary Python/YAML execution.
- Scope: mixed; official upstream wheel/source/PyPI metadata plus the existing collection worker contract.
- Date: 2026-11-25

## Findings

### Official release and dependency identity

- Official repository: https://github.com/hect0x7/JMComic-Crawler-Python
- Official documentation: https://jmcomic.readthedocs.io
- Official package metadata queried directly: https://pypi.org/pypi/jmcomic/json
- The observed current package version is **2.7.12**, matching `jmcomic/__init__.py` on the repository master branch. Pin this exact version rather than trusting an unconstrained future release.
- The published wheel `jmcomic-2.7.12-py3-none-any.whl` has SHA256 **127af57c476b22dead1c706ff897ce64b46de71295f7a43680f7c294f1a77bae**.
- The source archive has SHA256 **d1a6ec5f0935ecd492e47c138b0abfa8815f62454c42b6d2444ade5d00d85bb1**.
- Wheel URL: https://files.pythonhosted.org/packages/3a/c0/835dc8137eda3875c9388ad8d827cc7a47519420fe0135cad9598ddf59f9/jmcomic-2.7.12-py3-none-any.whl
- Distribution metadata declares **Python >=3.9**. This does not mean the newest dependencies work on 3.9: observed current Pillow 13.0.0 requires >=3.11, curl-cffi 0.17.0 requires >=3.10. Current Debian bookworm Python 3.11 is compatible in principle; actual wheel installation still needs verification for the supported architectures and Windows Python.
- Base required distributions are **curl-cffi, pillow, pycryptodome, pyyaml**. Old guides that expect jmcomic to require commonX / requests / postman are stale for this release. Do not install `[plugins]`: its PDF/archive/browser-cookie extras are unnecessary for ordinary images and enlarge the execution surface.
- The package is MIT, copyright (c) 2023 hect0x7. Include its copyright and MIT permission text in third-party notices when distributing it. The wheel includes `jmcomic-2.7.12.dist-info/licenses/LICENSE`.
- For a reproducible deployment lock, resolve the full base dependency closure in a project-local venv, pin every resolved distribution, obtain hashes for platform-appropriate wheels from each package's official PyPI JSON, and install with `--no-deps --require-hashes`. Include all approved wheel hashes needed for Linux amd64/arm64 and Windows rather than a single developer-platform wheel. Never auto-install dependencies during a search/download request.

### Search API (exact 2.7.12 wheel source)

- `jmcomic/jm_client_interface.py:546-557`: `client.search_site(search_query: str, page: int=1, order_by='mr', time='a', category='0', sub_category=None)` returns `JmSearchPage`. It calls a common `search` API with `main_tag=0`.
- Construct an option with `JmOption.construct({...})`, then `option.new_jm_client()` or `option.build_jm_client()`. `jm_option.py:494-500` retains the latter alias.
- `jm_entity.py:626-646`: use `page.iter_id_title()` for `(album_id, title)` or `iter_id_title_tag()` for `(album_id, title, tags)`. The iterator normalizes missing tags to `[]`; no need to fetch every result's detail just to display a list.
- `jm_entity.py:606-617,773-798`: `page.total` is the result count; `page.page_count` derives total pages. `page.is_single_album` and `page.single_album` represent a numeric-query redirect. `JmApiClient.search` explicitly supports a response with `redirect_aid` and loads that album (`jm_client_impl.py:828-840`).
- `client.get_album_detail(album_id)` supplies `album.album_id`, `name`, `page_count` (total images), `authors`, `tags`, and iterable chapter details (`jm_entity.py:469-508,581-585`). `len(album)` is chapter count. API detail may redirect a chapter ID to its containing album (`jm_client_impl.py:866-875`). Bind the canonical returned ID in preview identity.
- Limit accepted query length and page number, result count, title/tag lengths, total output JSON size, execution time and simultaneous searches. A failed source call must be an error rather than a fabricated empty result.

### Download, output and completeness

- `jmcomic/api.py:234-282`: `jmcomic.download_album(album_id, option, downloader=None, *, check_exception=True, extra=None, control=None, login=True)` downloads **all chapters**. A single string/int returns `DownloadResult`; passing an iterable is a different batch contract and should not be exposed accidentally.
- `jm_downloader.py:428-455`: the result remains unpackable as `album, downloader`; fields are `.detail`, `.downloader`, `.manifest`, `.duration`, `.album`. Avoid assuming the result is only an album object.
- `check_exception=True` is the default and calls `downloader.raise_if_has_exception()` after the action; per-image exceptions are otherwise retained in failure lists. `downloader.all_success` checks both chapter and image completeness (`jm_downloader.py:186-210`). Reject any partial download before acquisition/publication; do not interpret a normal return or nonempty directory as completeness.
- `JmOption.construct` accepts a fixed internal dictionary and merges defaults (`jm_option.py:343-364`). Set `log=False`, `dir_rule.base_dir=<owned staging>`, a trusted rule, `download.cache=False`, `download.threading.image=1..small bound`, `download.threading.photo=1`, `download.image.decode=True`, optional fixed raster suffix, `client.retry_times=1..small bound`, `client.postman.meta_data.timeout=<bounded seconds>`.
- Default output rule is `Bd / Pname`, image decode is enabled, suffix is unchanged, image threads=20 and photo threads=CPU count (`jm_config.py:519-542,587-591`). Override these defaults for bounded NAS work. Raw source names should not determine root ownership.
- Directory DSL `Bd / P...` is native; `P<attribute>` reads chapter properties (`jm_option.py:130-158`). `photo.album_index` is the chapter's semantic sequence (`jm_entity.py:381-390`); `Pindextitle` yields its sequence plus title. Safer still: own `JmOption` subclass methods that derive numeric padded chapter/image names and verify every output path stays within staging. Do not accept DSL/YAML/plugins from HTTP clients.
- The project scanner can import ordinary chapter image directories; no PDF/ZIP postprocessor or optional plugin is needed. Preserve chapter order deterministically. A single directory of pages can be used if names include padded chapter and image indexes, or separate numeric chapter directories if the existing scanner aggregates them as expected.
- Enforce known `album.page_count <= maxFiles` before transfer, and actual file/byte/disk budgets throughout download. Chapter metadata may reveal a more accurate total. Final Go-side manifest validation remains required; Python result JSON alone is insufficient authority.

### Logs, progress, cancellation and networking

- `log=False` invokes `disable_jm_log()` (`jm_option.py:346-349`). JM's default logger writes to **stdout** (`jm_config.py:51-59`); disable it before API activity and catch exceptions without printing their original text, which may include URLs, response bodies and cookies.
- Set/redirect Python logging and stdout consistently around imports and calls. Reserve original stdout for exactly one bounded JSON response. Send only controlled stage messages to the service log, never unfiltered upstream stderr/tracebacks.
- No stable percentage callback is needed for minimal integration. The host can expose queued/running/acquired/published via existing durable operation states. If page progress is required, a trusted `JmDownloader` subclass can override `after_image`/`after_photo`, call `super`, update bounded counters and validate budgets. Do not register arbitrary upstream plugins from requests.
- 2.7.12 provides `DownloadControl()` with `.cancel(reason)` and `download_album(..., control=...)` (`jm_task_context.py:27-53`, `api.py:255`). Use it for cooperative budget cancellation if implementing per-image hooks. The Go worker's existing owned-process cancellation remains the authoritative hard-stop and process-tree cleanup mechanism.
- Default client is **api**, default postman is **curl_cffi**, and default proxies are inherited from system discovery (`jm_config.py:507-509,531-537,581-585`). Explicitly supply the worker's controlled proxy as `client.postman.meta_data.proxies={'http': proxy, 'https': proxy}` rather than relying only on inherited environment.
- All metadata, initial domain discovery and image/CDN requests must use the same controlled outbound policy. `JmOption.new_jm_client` creates the postman from the fixed config before constructing the client (`jm_option.py:514-574`). Session/global domain/bootstrap requests must be inspected too: `JmModuleConfig.new_postman` uses `DEFAULT_PROXIES`; set that controlled value inside the trusted one-shot adapter as well, or otherwise ensure bootstrap requests cannot fall back to direct connection.
- Do not allow public input to select proxy, domain URLs, cookie files, accounts or arbitrary config. Additional private admin connection support can be added later as a separately specified capability.

### Suggested trusted JSON subprocess contract

Use an embedded/vetted Python script executed as a fixed argument by configured Python, with one bounded JSON object on stdin. The browser must not choose an executable, script, arguments, output root or option file.

- Probe: `{action:'probe'}` -> `{version:'2.7.12', protocolVersion:1}` using package metadata/import only, no live source access.
- Search: `{action:'search',query:<bounded>,page:<bounded>,proxy:<worker generated>}` -> `{items:[{albumId,title,tags:[]}],page,total,totalPages}`.
- Preview: `{action:'album',albumId:<digits>,proxy:<worker generated>}` -> `{albumId,title,pageCount,chapterCount,authors:[],tags:[]}`. The host persists canonical identity and budgets in an expiring preview.
- Download: `{action:'download',albumId:<canonical>,outputDir:<worker staging>,maxBytes,maxFiles,proxy:<worker generated>}` -> `{albumId,title,pageCount,files:<count>}` only after completeness and fixed-output checks. The Go worker enumerates/validates actual outputs and controls publication.
- Failure: return a nonzero code and a fixed safe error category/Chinese message without upstream exception text. Distinguish dependency unavailable from source/network failure when possible.
- Use `python -I -B -c <embedded script>` or equivalent isolated execution, a private working directory, scrubbed environment and existing `runTool`/owned process cleanup. A configured Python interpreter is trusted administrator configuration; it should be optional and capability-off when absent.

### Executable verification proposal (no actual comic fetch)

1. Create `.runtime/...` venv via project_env, install the exact hash-locked base package closure, run `python -m pip check`.
2. Run real imports, metadata version, `JmOption.construct`, search page iteration using synthetic `JmSearchPage` objects, and `DownloadResult`/completeness behavior with synthetic entities.
3. Run the actual embedded adapter against an import shim or fake client inside an isolated test process so search/preview/download/error paths are deterministic and contain generated raster fixtures only. Do not substitute this for real-source availability claims.
4. Exercise Go app -> worker -> Python protocol with local fixture search and generated images, then publish/import/read real item bytes through authenticated NAS endpoints; include cancellation, oversized output, invalid ID/page/query, stale preview, missing Python/dependency and original UUID retry.
5. Verify real curl-cffi requests traverse the worker's proxy and reject private/reserved targets, redirect/CDN fallback and alternate downloader bypass. A normal library import proves dependency compatibility only, not real upstream connectivity.

## Files Found

- `internal/collectionworker/tools.go:30-131` — bounded subprocess output, sanitized tool errors, scrubbed per-worker environment and controlled network proxy.
- `internal/collectionworker/tools.go:133-180` — existing fixed acquisition request validation; new comic acquisition must have its own validated fixed kind rather than masquerading as web video.
- `internal/collectionworker/config.go:22-38` — optional tool paths and Chinese configuration comments to mirror for Python/plugin activation.
- `internal/collectioncontract/models.go` — native protocol v1, capability snapshots, previews, operations and immutable publication facts to extend additively.
- `Dockerfile.collection-worker` — Python 3.11 venv already exists for yt-dlp, hash-checked dependency installation convention.
- `deploy/collection-requirements.txt` — dependency lock integration point (inspect before modifying to preserve existing yt-dlp package).

## Related Specs

- `.trellis/spec/backend/collection-contract.md` — mandatory preview/operation/publication/import boundaries and private/loopback network denial.
- `.trellis/spec/backend/runtime-contract.md` — authenticated APIs, strict JSON, Chinese errors/logs, no user media mutation.
- `.trellis/spec/backend/cross-platform-contract.md` — owned child process lifecycle and Windows cancellation.
- `.trellis/spec/frontend/app-contract.md` — actual worker capability, separate acquisition/import status and shared comic reader lifecycle.

## Caveats / Not Found

- No real source searches or comic downloads were performed. This research inspected official distribution metadata and the exact wheel bytes in memory; the integration must not claim source or NAS runtime acceptance from it.
- GitHub unauthenticated commit API returned HTTP403 rate-limit; no immutable repository commit was recorded. The PyPI wheel SHA256 is the precise inspected artifact identity.
- The research agent's write scope is task research only, so no venv was created and no package was installed. Installation validation is an implementation/check step.
- The source site's availability, accounts, geoblocking and eventual domain rotations remain external runtime conditions; capability probe should only promise a usable adapter/dependency, with search errors displayed truthfully.
