# Research: UAV-Downloader fixed video adapter

- Query: Integrate the user-selected UAV-Downloader into existing collection search, preview, download and publication.
- Scope: mixed (upstream static inspection and current NAS worker contracts).
- Date: 2026-11-25 (Asia/Shanghai).
- Source identity: `Alos21750/UAV-Downloader`, master commit `d40f09457fbd38dd7dc94580637242afafaf3939`, committed `2026-11-25T00:41:26Z`; upstream `pyproject.toml` and `src/uav_downloader/metadata.py` both declare **3.8.4**.
- Provenance: GitHub API commit/tree, raw files and codeload archive were read directly. No upstream code was executed and no video site was contacted.

## Findings

### Recommended scope and integration decision

Implement a fixed optional adapter for **JableTV, MissAV and Hanime1**. These have independently callable Python search/listing classes and page resolvers returning ordinary HLS or direct MP4. Use a bounded Python JSON bridge for search and resolution only; let the existing Go-owned, authenticated/public-network-proxied native yt-dlp process fetch video and offline FFmpeg remux it. Keep the existing generic `web` protocol filter unchanged; make supported native HLS an explicit UAV-only contract.

Do **not** advertise SupJav or Huangguo support through this simple path. SupJav's resolver supports multiple servers but its download implementation strips fake headers from some TS segments (`sites/supjav.py:342-343`), which a generic HLS downloader does not necessarily reproduce. Huangguo dynamically scopes `x-cdn-auth` / `x-cdn-app` to an exact media origin and resource type (`sites/huangguo.py:579-609`); flattening those into global yt-dlp headers can leak transient credentials or fail playback. Both need separate complete adapters and validation.

Three feasible patterns were considered:

1. **Fixed Python resolver + current Go/yt-dlp execution (recommended).** Reuses upstream parsing while preserving our task, process, budget, proxy, manifest and publication ownership. Adds only a narrow search/resolve contract. Costs: explicit source whitelist, bounded strict response checking, identity binding, and a reproducible Python source/dependency installation.
2. **Run upstream `CreateSite(...).start_download()` in a Python bridge.** Preserves every site-specific transfer behavior, but introduces a second download engine, segment tree cleanup, image sidecars, filename rules, multiple large staging copies and different completion semantics. Particularly unsuitable without overriding the TS-to-`.mp4` fallback below.
3. **Connect to upstream authenticated Web queue.** Upstream `web/server.py` exposes queue list/add, cancel/retry/resume/clear only; there is **no search or resolver API**. Adds a second durable queue and cannot satisfy integrated search without custom upstream extensions. Reject for this task.

### External files and exact callable APIs

All upstream line references below refer to the pinned commit, not a moving branch.

| Upstream path | Purpose / key lines |
| --- | --- |
| `pyproject.toml:5-30` | Package `uav-downloader`, version 3.8.4, Python `>=3.12`, Apache-2.0, full desktop/AI dependencies. |
| `requirements-docker.txt:1-10` | Official minimal no-GUI dependency list. |
| `sites/__init__.py:57-80` | `create_site` / compatibility `CreateSite`; imported package brings featured and legacy site modules, but no GUI app import. |
| `sites/jabletv.py:43-79` | `SiteJableTV` / `SiteJableTV_Backup` canonical URL validation and detail parsing. |
| `sites/jabletv.py:345-377` | `JableTVBrowser.fetch_page(url) -> list[dict]`. |
| `sites/missav.py:160-177` | Bounded `SiteMissAV.validate_url`; slug must contain an ASCII digit. |
| `sites/missav.py:203-251` | MissAV detail extraction, packed JS literal unpacking, media URL and referer/origin. No runtime JS eval required. |
| `sites/missav.py:303-360` | `MissAVBrowser.fetch_page`, `search`, `page_url`. |
| `sites/hanime1.py:138-156` | `_request` / `_prime_session`: anonymous homepage visit establishes site cookies; Cloudflare response is an error. |
| `sites/hanime1.py:403-451` | `SiteHanime1`, detail/download-page fallback and per-object resolution selection. |
| `sites/hanime1.py:529-607` | `Hanime1Browser.search_url`, `page_url`, `fetch_page`. |
| `apps/browse.py:3474-3492,4348-4370` | Actual upstream GUI search URL construction and **Jable pagination uses `from`, not `page`**. Read only for protocol reference; do not import this GUI module. |
| `sites/base.py:654-747` | Base constructor resolves detail immediately, traps exceptions into `_last_error`, and prints diagnostics. |
| `sites/base.py:749-752,791-809` | `target_name()`, `is_url_vaildate()` (upstream spelling), video destination and header helpers. |
| `core/config.py:767-785` | Explicit requests/curl proxy kwargs; default direct mode overrides environment proxies with empty routes. |
| `sites/base.py:961-1018,1035-1063` | Full downloader merge/remux behavior, including unsafe-for-our-contract fallback described below. |
| `web/server.py` | Authenticated same-origin persistent URL queue; no search interface. |

#### Search examples (source pseudocode; not executed)

```python
from urllib.parse import quote
from uav_downloader.sites import jabletv, missav, hanime1

# query/page must already be bounded; these addresses are created by trusted code.
jable_url = f"https://jable.tv/search/?q={quote(query, safe='')}&from={page}"
jable_items = jabletv.JableTVBrowser.fetch_page(jable_url)

missav_base = "https://missav.ai/search/" + quote(query, safe="")
missav_items = missav.MissAVBrowser.fetch_page(
    missav.MissAVBrowser.page_url(missav_base, page))

hanime_items = hanime1.Hanime1Browser.fetch_page(
    hanime1.Hanime1Browser.page_url(
        hanime1.Hanime1Browser.search_url(query), page))
```

Common returned fields: `url`, `title`, `thumbnail`, `duration`. Hanime may add `date`, `author` and `_source_listing_url`; do not blindly expose unknown fields. Returned list does **not** provide an authoritative total or next-page flag. Do not invent a total. An explicit UI next-page control or documented `hasNext` heuristic is preferable to claiming exact pagination metadata.

Jable search cards use `div[id^=list_videos] div.video-img-box`; title/link `div.detail h6 a`, thumbnail `img[data-src]`, duration `span.label`. MissAV uses `div.thumbnail` or alternate group/grid selectors, then `a[href]`, `img[alt]` or title anchor, duration `span.absolute.bottom-1.right-1`. Hanime uses `.video-item-container a.video-link` and compact `.home-rows-videos-div.search-videos` cards.

**Important failure semantics:** Jable/MissAV/Hanime browser methods sometimes return `[]` for unknown HTML, nonmatching page structures or parsing exceptions. A bridge must add transport and structural validation so bad HTML, denied requests and parser failures do not become a successful empty search. Only known valid empty-list structures should yield success. Jable's old `JableTVList` filters `ribbon-top-left` member cards, but newer `JableTVBrowser.fetch_page` does not; detail resolution must still require a publicly available media source and never add credentials or access bypass.

#### Detail/resolve examples (source pseudocode; not executed)

```python
classes = {
    "jable": jabletv.SiteJableTV,
    "missav": missav.SiteMissAV,
    "hanime1": hanime1.SiteHanime1,
}
site = classes[site_key](
    validated_url, savepath=trusted_worker_directory,
    silence=True, max_workers=1, resolution="highest")
if site._last_error is not None or not site.is_url_vaildate():
    raise ValueError("upstream resolution failed")
media_url = getattr(site, "_direct_url", None) or site._m3u8url
title = site.target_name()
headers = site._extra_headers  # reduce to fixed Referer / Origin keys and revalidate
```

The constructor performs network requests but does not create output directories until downloading. It catches and prints details including URL; isolate stdout/stderr and return only a strict bounded JSON object. `silence=True` does not suppress every exception diagnostic. Do not echo `_last_error`, raw URL, signed media query, proxy credentials or raw stderr to application logs/UI.

Jable canonical video: `https://jable.tv/videos/[a-z0-9_-]+/`; optional upstream mirror `fs1.app` uses the same shape. MissAV known hosts: `missav.ai`, `missav.ws`, `missav.live`, `missav123.com`; optional `/dmN/` and language prefix precede a bounded alphanumeric/underscore/hyphen slug that contains a digit. Hanime: `https://hanime1.me/watch?v=<digits>` (optional www in upstream), query is part of canonical identity. Main service, worker and Python must share exactly the same canonical URL/ID contract. Reject credentials, fragments, arbitrary ports, unsupported query keys and cross-site result links.

Suggested private bridge outputs: search `{site,query,page,items:[{url,title,duration}]}`; resolver `{site,url,title,mediaUrl,headers}`. A site-specific stable media identity should bind preview to the selected source item, while signed/transient media URLs remain private and may be refreshed for execution. Public API must never accept arbitrary `mediaUrl` or headers supplied by the browser.

### Dependency, packaging and license

- Source archive: `https://codeload.github.com/Alos21750/UAV-Downloader/tar.gz/d40f09457fbd38dd7dc94580637242afafaf3939`.
- Independently read archive SHA-256: **`88934f8920f836b064212a6d997a0eedb9a8149e38d112569d8c766dae208b41`**, size **1,952,930 bytes**. Pin this content and commit; do not fetch mutable master at runtime.
- Upstream declared supported Python: **>=3.12**. Current NAS worker Docker base is Debian bookworm apt Python (3.11), so an ordinary package install is incompatible by metadata. Add a compatible reproducible runtime, or explicitly maintain/test a source-only compatibility adaptation for the existing worker runtime. Do not silently claim upstream supports 3.11.
- Official headless list: `requests>=2.28.0`, `PySocks>=1.7.1`, `beautifulsoup4>=4.11.0`, `m3u8>=3.0.0`, `pycryptodome>=3.15.0`, `cloudscraper>=1.2.68`, `curl_cffi>=0.7.0`, `certifi`. Pin all actual resolved transitive dependencies and hashes for every distributed platform; do not install from requests or at service start.
- Full pyproject install additionally includes desktop/AI packages (Pillow, customtkinter, ctranslate2, numpy, PyYAML, sentencepiece, OpenCC, imageio-ffmpeg). Avoid them for a no-GUI search/resolver bridge. The upstream official Docker image uses only `requirements-docker.txt` and `PYTHONPATH=/app/src`.
- License is **Apache-2.0**, not MIT. Distribute upstream LICENSE and preserve relevant copyright/attribution; identify local modifications if adapting source. Source/dependency metadata must accompany the optional worker package. Separate system FFmpeg license/source obligations follow the project's existing distribution process.

### Network, process and output boundaries

1. **Proxy cannot rely on environment alone.** `core/config.py:767-785` returns explicit empty `http`/`https`/`all` routes in default direct mode. Override the fixed bridge's internal `config.proxy_request_kwargs` to trusted worker proxy values (or preseed fixed in-memory manual proxy cache). Do not call persistent `set_proxy_url`, which writes preferences.
2. Bind requests, cloudscraper and curl_cffi to the same authenticated worker proxy. Clear `NO_PROXY` and inherited credentials/settings. Validate actual redirects/subrequests at connection time; a URL host allowlist alone cannot prevent DNS rebinding/private targets. No user-provided proxy, cookies, CF clearance, arbitrary code/config or HTTP header fields.
3. `core.paths.product_data_dir` can migrate legacy user state by default. Use the worker's existing clean HOME/APPDATA and private staging; avoid importing GUI/updater/subtitle modules or invoking persistent configuration setters.
4. Do not use upstream video transfer for this chosen design. Its `start_download` downloads image sidecars, permits title-derived filenames, creates segment caches, starts FFmpeg, and if remux fails/missing, **moves raw merged TS bytes to an `.mp4` destination** (`base.py:1003-1010`). It can therefore report completion without a true MP4.
5. Use fixed filename `media.%(ext)s` / existing operation-owned staging, native yt-dlp HTTP/HLS only, finite streams only (reject livestreams), fragment completeness, manifest/segment budgets, bounded stdout and strict JSON. FFmpeg should only read local files with an offline protocol whitelist. Keep all child processes in the worker's existing process-tree ownership/cancel mechanism.
6. Generic existing web download currently excludes HLS explicitly (`tools.go:266-268`); preserve that behavior. Add a UAV-scoped explicit HLS path and validate yt-dlp format protocol instead of widening global filters.
7. Native HLS may need cookies/client fingerprints on some live CDNs; neither successful source resolution nor a mocked transfer proves full live-site support. Missing files/partial fragments, invalid media, canceled/changed source and overflow must never publish/import.

## Local integration patterns inspected

- `internal/collectionworker/jmcomic.go:20-28,44-113`: fixed embedded bridge, optional dependency probe and bounded strict JSON dispatch.
- `internal/collectionworker/tools.go:75-126`: clean private subprocess environment, authenticated controlled proxy variables, fixed native process launch and bounded captured output.
- `internal/collectionworker/tools.go:262-292,476-481`: fixed yt-dlp options, existing generic protocol restrictions and offline FFmpeg postprocessing.
- `internal/app/collection_jmcomic.go:12-46`: authenticated source/kind/enabled/capability checks before forwarding worker search.
- `internal/collectioncontract/jmcomic.go:11-24`: shared input normalization/validation for application and worker.
- `Dockerfile.collection-worker:12-27`: current independent Python environments, requirements hashes and actual base runtime constraints.

Codegraph CLI was used first for `jmcomic`; the existing index predates the newly added plugin and returned only an older unrelated comic method. Exact local file reads and bounded `rg` searches supplemented the stale graph.

## Related specs

- `.trellis/spec/backend/collection-contract.md`: authenticated worker, durable task/UUID ownership, public-network proxy, staging budgets, explicit publication and exact import coverage.
- `.trellis/spec/backend/jmcomic-contract.md`: fixed optional bridge, immutable dependency version, source/type constraints, strict output handling and truthful fixture vs live evidence.
- `.trellis/spec/backend/runtime-contract.md` and `cross-platform-contract.md`: process and filesystem ownership.

## Caveats / Not Found

- No upstream programmatic search HTTP API or stable documented public Python API was found; imported class/member names are source-pinned internals.
- No actual media was fetched, no upstream Python code was executed, and no claim is made about live-site access, Cloudflare acceptance or CDN compatibility.
- `web.run` open/search calls produced no content in this environment; primary-source content was successfully read via direct GitHub raw/API/codeload HTTP. GitHub API later hit unauthenticated rate limiting; raw and codeload remained available.
- Search empty-page selectors for the three chosen sites are not exhaustively specified upstream. Unknown markup must be surfaced as unavailable/parse error until known supported structure is validated.
- Source and declared compatibility are verified; package installation, dependency closure, real bridge imports and controlled end-to-end transfer still require implementation-time verification.
