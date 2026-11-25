# Research: SupJav fixed adapter and playback sources

- Query: Integrate SupJav search, canonical video identity, playback-source resolution and validated download into the existing UAV adapter.
- Scope: mixed; immutable upstream source and tests plus existing Go/Python adapter. No live video-site/media request was made.
- Date: 2026-11-25 (Asia/Shanghai).
- Upstream: UAV-Downloader 3.8.4, commit `d40f09457fbd38dd7dc94580637242afafaf3939`, Apache-2.0. Keep the existing archive/dependency pins.

## Findings

### Source provenance and inspected files

Initial `web.run` access to the immutable GitHub source returned no content; a later raw-source call succeeded. A direct HTTPS read of the same official raw source also succeeded and exactly matched the existing extracted archive file (34,818 bytes; both SHA-256 `28abe43c2b5c9a06aa4190eed2431cc16e4bdc12093b8490464f1486eadd81af`). This comparison did not execute upstream code or contact video sites.

All upstream paths below are relative to `.runtime/research/uav/UAV-Downloader-d40f09457fbd38dd7dc94580637242afafaf3939/`:

- `src/uav_downloader/sites/supjav.py` — real browser, source extractors, candidate order, TS transform and upstream download implementation.
- `src/uav_downloader/sites/base.py:1129-1175` — download/decrypt/transform ordering and TS validation.
- `src/uav_downloader/core/config.py:18,21-26` — one fixed SupJav mirror and rate-limit retry waits.
- `tests/test_supjav.py:71-166,347-444,453-575` — identity, fake-header, provider, redirect, fallback and encrypted LUC regressions.
- `tests/test_supjav_fst.py:20-141` — static FST player source selection, relative/extensionless HLS and malformed-data handling.
- `tests/test_supjav_browser_errors.py` — actual empty pages versus transient, blocked or changed markup.

Local integration paths:

- `internal/collectionworker/uav_bridge.py:36-84,189-276,284-343` — fixed identity, browser search, finite HLS and generic resolver; requires SupJav special cases.
- `internal/collectionworker/uav.go:39-47,174-203` — private media struct and independently validated resolve output.
- `internal/collectionworker/uav_relay.go:18-25,128-218` — operation-owned resource registrations; currently rewrites only URLs and forwards encrypted/plain bodies unchanged.
- `internal/collectioncontract/uav.go:45-95` — shared site and canonical-ID gate.

CodeGraph CLI `query SupJav --limit 15 --json` returned `[]` because the project has not integrated it yet. Bounded `rg` and exact file reads supplemented the index.

### Search and canonical identity

`SupJavBrowser.search_url(query)` returns `https://supjav.com/?s=<percent-encoded-query>` (`supjav.py:840-842`). `page_url(base, page)` uses `/page/<n>/?s=...` for search pages (`829-837`), rather than `?page=n`. A next-page flag must inspect a real anchor with the same search query and the expected generated path; the existing generic query-only pagination logic will miss it. Do not accept an unrelated search's next link merely because its path matches.

`fetch_page()` (`807-826`) invokes `fetch_with_mirrors()` with structural validation. Real listings have `div.post a[href*=".html"]`; real empty pages are `body.search-no-results` or `section.no-results`. Unknown HTML and HTTP failures raise; they must never become `items:[]`. `_parse_videos()` (`303-333`) returns `{url,title,thumbnail,duration:'',date}`. Public output should retain only canonical ID, title and empty duration, with existing 100-item/120-character query/page budgets.

The upstream pattern is `https://supjav.com/(?:(?:zh|ja)/)?\d+\.html$` (`337-338`). Use a site-local **positive decimal ID**, preferably the existing 12-digit numeric contract, no leading zeros, paths, `.html`, language prefixes or URL syntax in the persisted ID. Canonical detail URL should be `https://supjav.com/<id>.html`. Search links can accept only HTTPS `supjav.com` / `www.supjav.com`, optional `/zh/` or `/ja/`, one positive numeric `.html` path, no arbitrary query/fragment/credentials/port; normalize to the same ID. Upstream itself does not enforce a digit-count ceiling; the project should document its explicit bound.

### Actual providers and static extraction

`_server_links()` (`84-94`) reads only `a.btn-server[data-link]`; the label is uppercased, first duplicate wins. `get_url_infos()` (`401-444`) resolves candidates in this exact order: **EVS, FST, VAS, LUC, VOE, TV, ST**. These are playback alternatives for one SupJav video, not independent search sites. Each provider exception is isolated. `_hls_sources` records deduplicated `(name, mediaURL, matchingHeaders)`; ST is stored separately in `_direct_url` and `_direct_referer`.

All provider `data-link` values are reversed and percent-encoded into `https://lk1.supremejav.com/supjav.php?c=<reversed>` (`348-351`). `_fetch_embed()` (`230-250`) bounds six HTTP hops with static HTTPS guards. The parent project's connection-time public-address policy remains mandatory: upstream URL checks alone do not establish DNS safety.

| Label | Pinned resolver | Format and caveat |
| --- | --- | --- |
| EVS | `_extract_packed_m3u8()` (`118-132`) after known static unpacker | HTTPS `.m3u8`; no remote JavaScript execution |
| FST | `_extract_fc2stream_links()` (`175-227`) then packed fallback | JSON `links` plus literal player's OR-chain order; relative links resolve against final embed URL; declared `type:'hls'` allows `.txt` or extensionless manifest. Do not infer direct MP4 from absence of `.m3u8`. |
| VAS | `_resolve_server()` (`376-390`) | POST final embed origin `/api/stream`, body `{filecode,device:'web',codecs:[]}`, retain permitted query except `filecode`; 200 JSON `streaming_url` must be HTTPS `.m3u8`. Existing bridge wraps curl/request POST through controlled proxy. |
| LUC | packed extractor | HTTPS HLS; actual upstream regression explicitly covers whole-segment AES encryption. Transform must occur after AES decrypt. |
| VOE | `_extract_voe_m3u8()` (`253-277`) plus up to two known literal `/e/<alnum>` redirects (`355-369`) | ROT13, fixed noise removal, base64, minus-three/reverse, base64 JSON; only static data, no JS execution; deliberately unrelated decoy source ignored. |
| TV | `_extract_m3u8()` (`53-59,393-394`) | HTTPS HLS; Referer is SupJav root |
| ST | `_streamtape_direct_url()` (`97-115,374-375`) | HTTPS progressive MP4 from known string/substring expression; ignores static decoy. Must use `_direct_referer`, not `_extra_headers` from another provider. |

Upstream source-level coverage is not live-provider availability. No new package is needed: these extractors use the current requests/curl-cffi/BeautifulSoup/PyCryptodome closure and Python standard library.

### Selection defects to avoid in the current generic bridge

1. Existing `resolve()` picks `_direct_url or _m3u8url`, so a page with ST and HLS would choose ST first, while `_extra_headers` still belongs to the first HLS provider. SupJav must select one candidate with its own matching headers.
2. Constructor resolves all embed pages, but validates none of their manifests. A dead EVS CDN must not prevent choosing a valid later FST/VAS/LUC/VOE/TV manifest. Validate candidates sequentially within a fixed overall deadline and return the first supported finite manifest. ST is the final fallback. An unknown media's content type/extension must not route a known FST HLS URL into direct download.
3. Do not call upstream `start_download()` for fallback. It owns another queue/staging/FFmpeg chain and weakens the existing project publication boundary. Its useful semantics are candidate order and isolated failure, which can be applied in the fixed bridge while keeping parent yt-dlp and budgets.
4. Final implementation decision: a candidate's transport/content failure may advance to a later fixed provider only after releasing its relay scope, checking staging budgets, removing expected owned regular files and checking disk reserve. A private monotonic `sourceOffset`/`sourceIndex` pair preserves provider order without a public selector. Cancellation, storage/budget errors, unexpected staging entries, process cleanup failure or changed identity stop acquisition. Never splice partial candidates or reset operation timeout/budget accounting. This supersedes the initial recommendation to require a task retry after any segment failure.

### Critical media detail: fake PNG prefix and AES ordering

`SiteSupJav._transform_segment()` (`342-343`) invokes `_strip_fake_header()` (`67-81`): plain `0x47` input returns unchanged; otherwise it searches only the first 8,000 bytes for five MPEG-TS sync bytes spaced 188 bytes apart, discards preceding bytes, and returns empty on no valid run. Upstream final TS validation is separate. This behavior applies to the SupJav site, not exclusively to one named provider. The source does **not** prove which live providers currently use wrappers.

`base.py:1154-1175` explicitly decrypts AES first, then transforms plaintext. Its comment and `test_supjav_luc_encrypted_segment_decrypts_before_ts_transform` confirm **LUC encrypts the entire segment**. Stripping ciphertext cannot work; omitting the transform may pass a wrong payload to native HLS. Full final decode/duration is still required, but cannot replace a clearly specified segment transform contract.

Initial minimum fallback option (superseded by the preferred offline option below if its real-downloader tests pass):

- Add private `supjavTS` (or similarly fixed typed) media flag only for `site=supjav`; Go validates it independently. Retain selected label privately if useful, never expose media URLs/headers.
- Accept ordinary finite **unencrypted MPEG-TS** SupJav candidates and ST MP4. For TS candidates reject fMP4/init-map, BYTERANGE, SAMPLE-AES/DRM and **AES-128 for the initial minimal path**, then try the next known candidate. Clearly document that encrypted SupJav lines (including some LUC responses) are currently unsupported. This is a valid site integration, not a claim that all provider variants work.
- On the operation-scoped relay, apply prefix removal only to registered SupJav **segment** resources, never key/map resources or other sites. Bound prefix probing to 8,000 + enough TS packets; verify the full bounded original response EOF and expected Content-Length before declaring success; ensure removal cannot hide truncated or overlong input. Validate the resulting TS packet grid, strip no arbitrary payload beyond reviewed prefix. Use a small fixed per-segment memory ceiling (or bounded staging) plus existing total operation budget. Never buffer one entire arbitrary task budget in memory.
- Reject ranges for transformed resources, because source Content-Range describes pre-transform offsets. Keep the existing exact Range behavior unchanged for all ordinary HLS resources.
- Continue to use inline finite manifest, authenticated scoped relay, real pinned native yt-dlp, offline remux/full decode, duration lower bound and independent publication/import checks.

If full ordinary AES SupJav support is required in this change, implement an explicit typed per-segment crypto context in the scoped relay: exact-16-byte key, correct explicit IV or media-sequence-derived IV, bounded ciphertext, AES-CBC decrypt, valid padding treatment, TS wrapper transform and packet validation. Then return validated plaintext and rewrite the inline playlist's encryption tags accordingly. Do not retain `EXT-X-KEY` and return plaintext, and do not reuse one relay route merely by URL when the same resource occurs under different key/IV/sequence contexts. This is a larger implementation and needs real downloader encrypted-wrapper tests; it uses Go/Python existing crypto capability, not a new dependency.

An intermediate option which passes through encrypted bytes unchanged is acceptable only if it is explicitly limited and tested for ordinary AES TS. It cannot guarantee rejection of encrypted wrapper payloads without key/IV-aware plaintext inspection; therefore do not advertise wrapped AES/LUC support on the strength of a successful final decode alone.

### Preferred smaller AES-capable path: normalize local decrypted TS

The primary agent proposed keeping the relay unchanged and performing the site-specific transform against **local decrypted concatenated TS after native yt-dlp succeeds, before FFmpeg**. Inspection of the actual locked `yt-dlp 2026.10.26` installation confirms this is viable:

- `.runtime/research/uav/venv/Lib/site-packages/yt_dlp/downloader/fragment.py:349-363` decrypts each AES-128 fragment with its key and explicit/media-sequence IV, then calls `unpad_pkcs7`.
- `fragment.py:500,515` passes the decrypted fragment to append, in playlist order even when fragment downloads are concurrent.
- `fragment.py:146-154` writes bytes directly to the destination; the ordinary TS branch does not invoke FFmpeg or transform/strip wrappers before append. The native download therefore produces `wrapper+TS | wrapper+TS | ...` in plaintext, suitable for a bounded offline normalization pass.
- `yt_dlp/aes.py:33-34` implements unpadding as **`data[:-lastByte]` without PKCS#7 byte validation**. Normal valid PKCS#7 padding disappears before concatenation. Malformed or unpadded ciphertext may instead lose real trailing payload; do not claim strict padding authenticity from the downloader's success.

The recommended offline normalizer contract:

1. Run only for `site=supjav` with a validated finite ordinary MPEG-TS manifest; reject init maps/fMP4 and BYTERANGE for this path. ST MP4 bypasses it. Existing other sites/relay behavior stays unchanged.
2. Walk the file on strict 188-byte packet boundaries, retaining every complete TS packet. At a boundary, accept only a valid TS header or the **specific known PNG wrapper signature**. Do not search ahead for a new sync run after arbitrary damage.
3. For recognized wrappers, allow at most 8,000 bytes to find a candidate with five sync bytes spaced 188 apart, preferably valid TS headers (adaptation-field-control cannot be zero). Wrapper limits are per encountered prefix; the total file/operation budget still applies. Unknown wrapper shapes are unsupported and fail explicitly.
4. Validate each following 188-byte packet as consumed. A final partial packet, arbitrary junk, HTML, missing sync or a non-wrapper at an expected boundary is an error. Fewer than five packets near EOF can be accepted only after prior trustworthy lock and with complete packet boundaries; a wrapper followed by too little TS cannot establish an ambiguous new lock.
5. Use bounded buffered `ReadAt`/`WriteAt` or equivalent safe independent offsets with output position never beyond input position; compress in place only after consumed bytes have been buffered. Truncate only after full successful parsing, respect cancellation between chunks, reject zero packets. Failure never authorizes publication even if the operation-owned local file was partially compacted.
6. Keep offline FFmpeg complete decode, positive frames, finite-manifest duration lower bound and the independent publication gate.

Important counterexample: a permissive “on any bad byte, scan the next 8,000 bytes for five syncs” normalizer can silently discard a TS packet truncated by malformed AES unpadding, recover at the next fragment, and still meet the existing 2% duration tolerance. Require recognizable wrappers at the expected packet boundary; do not generalize resynchronization into media repair. Deliberately corrupted input which happens to mimic valid TS and recognized wrappers cannot be cryptographically authenticated by this MPEG-TS structure check; it is a format/integrity gate, not content authenticity or semantic-completeness proof.

This approach supports ordinary AES and AES-wrapped TS **without moving crypto into Go or exposing keys**, provided the actual pinned downloader + production relay tests cover it. Include plain/wrapped plaintext, plain/wrapped AES, explicit/default IV, changing keys, malformed/unpadded AES tail, internal corruption, unknown prefix, >8,000 prefix, short final packet, wrapper followed by too few packets, and cancellation. Use a real fixture video and compare resulting decode duration; synthetic repeated sync bytes are only parser unit fixtures.

### Fixture and acceptance suggestions

Use real pinned parser methods with intercepted transport, not a replacement parser:

1. Search normal/lang URLs, expected actual path pagination with preserved query, wrong query/foreign host/category links, duplicate IDs, real empty CSS shapes and bad/blocked HTML; positive/zero/leading-zero/oversized numeric IDs.
2. Candidate fixtures for EVS/FST/VAS/LUC/VOE/TV/ST; first invalid manifest falls back; FST `.txt`/extensionless stays HLS; VAS proxy-routed POST, VOE known redirect bounds, ST correct embed Referer; all candidates fail returns sanitized non-empty error.
3. Actual native downloader + production scoped relay against controlled TLS upstream: plain TS; PNG-wrapped TS; malformed prefix/no five syncs; short/overlong/chunked wrapper bodies; canceled operation; unaffected other-site media and range behavior. If AES is rejected, explicit encrypted-candidate → next-valid/ST and all-encrypted rejection cases are required.
4. If AES transform is implemented: LUC encrypted wrapped TS, ordinary encrypted TS, valid/invalid PKCS#7 tail, explicit/default IV, changing key/IV, wrong key length, same URL different crypto context, truncation and cancellation. Compare expected TS/duration, not just exit code.
5. Five-site protocol/browser checks, SupJav preview→stable UUID task→download→video library→original player; original four sites and JMComic must remain valid. Existing reusable verification harnesses should be extended; delete generated temporary unit-test classes after use.

## External references

- Immutable official source: https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/src/uav_downloader/sites/supjav.py
- Successfully compared raw source: https://raw.githubusercontent.com/Alos21750/UAV-Downloader/d40f09457fbd38dd7dc94580637242afafaf3939/src/uav_downloader/sites/supjav.py
- Immutable FST regressions: https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/tests/test_supjav_fst.py
- Immutable provider/media regressions: https://github.com/Alos21750/UAV-Downloader/blob/d40f09457fbd38dd7dc94580637242afafaf3939/tests/test_supjav.py
- Locked native fragment/decryption implementation (also opened via `web.run`): https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/yt_dlp/downloader/fragment.py
- Locked padding implementation (also opened via `web.run`): https://github.com/yt-dlp/yt-dlp/blob/2026.10.26/yt_dlp/aes.py

## Related specs

- `.trellis/spec/backend/uav-contract.md` — fixed adapter identity, finite HLS, relay scope, ranges, actual downloader and publication.
- `.trellis/spec/backend/collection-contract.md` — private proxy, task lifecycle, staging budgets, retry/cancel and exact import coverage.
- `.trellis/spec/backend/runtime-contract.md` and `cross-platform-contract.md` — process ownership, clean environment and platform verification boundaries.
- `.trellis/spec/guides/cross-layer-thinking-guide.md` — shared site/ID/typed private selector propagation.

## Caveats / Not Found

- No live SupJav, provider embed or CDN was contacted; no live reachability, current provider status, account access or public media completeness is asserted.
- Source parser support for each label is established, but it is not evidence that every live response is compatible with the parent's finite-HLS format contract.
- The upstream has no supported stable external SupJav API; imported classes and helpers are source-pinned internals.
- Existing generic bridge is not enough by adding one import: path pagination, ST matching headers, candidate fallback and plaintext TS transformation need dedicated handling.
- These findings do not authorize broadening private-address rules, adding account/Cookie fields, running remote JS, altering the existing downloader or invoking upstream video transfer.
