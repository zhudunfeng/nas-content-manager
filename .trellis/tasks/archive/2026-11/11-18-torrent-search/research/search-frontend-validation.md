# Torrent search frontend validation

Date: 2026-11-18. Scope: the frontend implementation and browser UI states. No Docker commands were executed, and no temporary unit-test files were created.

## Changed frontend files

- `web/src/components/TorrentSearchDialog.vue`: explicit search, safe DTO results, source/category filters, bounded local sorting/paging, cancellation, partial failures and expiry.
- `web/src/components/DownloadCenter.vue`: independent search entry and sequential search-to-add modal handoff.
- `web/src/components/AddDownloadDialog.vue`: optional readonly search selection and token-only submission through the existing type/library confirmation workflow.
- `web/src/types.ts`: commented search DTOs.
- `web/src/style.css`: existing semantic theme tokens, mobile layout and search result styling.

The existing uncommitted BT implementation was preserved. Backend and user-facing documentation are owned by the parent task and were not modified by this frontend agent.

## Build checks

- `npm --prefix web run typecheck`: passed (`vue-tsc --noEmit`).
- `npm --prefix web run build`: passed (Vite 6.4.6, 143 modules).
- No new frontend package was added. This package has no separate lint script.

## Executed controlled browser checks

A local Vite instance on `127.0.0.1:18124` rendered the actual Vue components. Playwright intercepted only `/api/**` with controlled DTO fixtures; these checks do not assert Torznab or qBittorrent behavior.

- Initial opening did not search automatically; a one-character query produced validation feedback without a search request.
- A 45-result snapshot rendered 20 / 20 / 5 rows over three pages. Default ordering selected the greatest known seed count first. Size ascending reset the page, placed the smallest known value first and unknown size last. Sorting and paging did not issue new search requests.
- A delayed old query could not replace a newer query. Cancelling a pending query displayed an explicit cancellation notice and discarded its delayed result.
- Successful empty results, complete search failure, partial-source failure and unconfigured search each rendered their distinct states. A failed status read remained an error rather than becoming setup guidance.
- Partial-source failure retained usable results and identified the failed source. A short-lived fixture became expired and disabled all add buttons.
- Source and category controls were included in the search input. Unknown size/count/date metadata remained visibly unknown.
- Search remained usable with the download engine offline, while adding was disabled with an explanation.
- Choosing a result closed search before opening the existing add form: exactly one native open dialog remained. The readonly title/source summary appeared and magnet/upload input switching was absent.
- Two deliberately failed add attempts sent the same `requestId` and `searchResultId`, with no `magnet` field. Cancelling restored the search-entry focus and the original body overflow. Escape closed the search dialog.
- At 375, 768, 1024 and 1440 CSS-pixel widths, both light and dark themes had no document/dialog/result-section horizontal overflow. At 375 pixels every card fit, including a 210-character title. Search controls remained readable and touch sized.

The expected network console errors were the deliberately returned 502 add/search failures and 503 status-read failure. No unhandled UI exception or warning was observed. Two test-harness locator/actionability timeouts were corrected in the harness (explicit sort selector and immediate forced cancel click); they did not require application changes.

## Visual evidence

- `.runtime/search-ui-mobile-light.png`: 375-pixel light theme, inspected.
- `.runtime/search-ui-desktop-dark.png`: 1440-pixel dark theme, inspected.

Both show the actual modal, filters and result cards. The dialog scrolls internally; full-page screenshots also include the inert background page.

## Handoff and limits

The Playwright page was closed and the Vite process was stopped after verification, releasing the shared browser for parent-task validation. Parent-task validation will exercise the real Go application with controlled Torznab/qBittorrent fixtures and the complete result-to-file-selection flow. Real indexer quality, Docker deployment and NAS performance remain outside this UI fixture check.
