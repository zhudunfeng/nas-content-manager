# JMComic advanced frontend implementation and validation

Date: 2026-11-25 (Asia/Shanghai).

## Ownership and changed files

- `web/src/types.ts`: typed optional chapter-preview metadata, verified-image progress, and fixed upstream search enums. Existing preview/task fields retain their meaning.
- `web/src/components/CollectionJMComicDialog.vue`: actual upstream-supported sort/time search, selected-chapter preview binding, over-budget catalogue handling, and sequential per-album batch preview/submission (shared limit: 10).
- `web/src/components/CollectionTaskDialog.vue`: a native bounded progress element for verified image acquisition. Final-product bytes and acquisition/import phases are unchanged.
- `web/src/components/CollectionCenter.vue`: accepted task events refresh the durable list while leaving a batch open.

No backend, public documentation, verification script, or spec files were changed by the frontend implementer. No temporary unit-test classes/files were generated.

## Reviewed contracts

- Search sends `query`, `page`, `orderBy` (`mr|mv|mp|tf`), and `time` (`a|t|w|m`). Controls are labelled and folded under an advanced summary; no ignored category control is exposed.
- First preview omits chapter selection and means the complete album. Selection changes disable confirmation until the server supplies a new bound preview. Empty selection is blocked. Full selection again omits `chapterIds`; a subset follows server chapter order.
- Album-ID/source changes invalidate the old selection. Clicking the same search result explicitly resets to the full-album preview. Searches isolate aborted/late responses. Switching modes or starting a preview retains already-read search results.
- `overBudget` leaves the chapter catalogue available but disables new task submission. Batch over-budget entries explain that a single-album selection is required.
- Batch parsing rejects invalid/duplicate IDs and more than 10 IDs before network calls. Each item retains preview, error, immutable task body and accepted task independently. Accepted items are skipped in later submissions.
- Each task body is frozen before sending, with its own UUID. Unknown responses keep the exact body, lock source/target/mode, and permit original-body retry after preview expiry. An item with a stored task body cannot refresh its preview. Batch submission does not immediately hand off to the first task.
- Closing aborts the local queue; the interface explains that already accepted tasks continue and remain available in the durable task list.
- Progress rejects negative/unsafe integer counters, nonpositive totals, `completedPages > totalPages`, and `reusedPages > completedPages`. It describes verified bytes as including reuse and explicitly retains validation/publication/import phases. A failed task read labels the old progress snapshot.
- Shared `DialogShell`, `CollectionTargetFields`, collection styles, semantic theme variables and `collectionBatchLimit` are reused. No nested modal or navigation entry is introduced.

## Static checks

Executed through `python -B scripts/project_env.py -- ...`:

- `npm --prefix web run typecheck`: passed (`vue-tsc --noEmit`).
- `npm --prefix web run build`: passed (strict typecheck and Vite production bundle); repeated after final source review.
- `git diff --check -- web/src/types.ts web/src/components/CollectionJMComicDialog.vue web/src/components/CollectionTaskDialog.vue web/src/components/CollectionCenter.vue`: passed. Git may report repository CRLF conversion notices; there were no whitespace errors.

The final production build exited 0 (188 transformed modules, Vite build 5.16s). Its collection chunk is `CollectionCenter-DEn3OwUc.js` and entry chunk is `index-DZQiq6F3.js`. The frontend read-back covered the new state transitions, methods and template. This is static validation, not browser or live-site acceptance.

## Browser handoff selectors

- Existing fields: `#jmcomic-query`, `#jmcomic-album`; existing actions: `搜索漫画`, `预览漫画`, `确认下载并入库`.
- Source: `#jmcomic-source`.
- Advanced summary: `高级搜索筛选`; labelled selects `#jmcomic-order` (`排序方式`) and `#jmcomic-time` (`时间范围`).
- Mode buttons: `搜索与选章`, `批量编号`, with `aria-pressed`.
- Chapter fieldset legend: `下载章节`; chapter checkbox `value` equals the chapter ID. Actions: `全选章节`, `清空选择`, and `更新章节预览` after a change.
- Changed-selection status: `章节选择已变更，请点击“更新章节预览”后再确认下载。`
- Batch textarea: `#jmcomic-batch`; preview: `预览批次`; per-album row: `article[data-album-id="<id>"]`.
- Batch submit: `确认下载 N 本`; retry: `重试未受理条目 N 本`; accepted status: `任务已受理`; task handoff: `查看收集任务`; untouched-item refresh: `重新预览此本`.
- Task progress: `getByRole('progressbar', {name: '漫画图片获取进度'})`. The enclosing group also carries the same accessible label; use the role when selecting the actual native progress element.

## Remaining verification boundary

The parent owns protocol/bridge integration, browser tests at 375/768/1024/1440 in both themes, screenshots, keyboard/focus/Escape, delayed/failed responses and old-flow regression. Those checks were not executed by this frontend implementer and must be recorded separately. No public comic site, user account, Docker or physical NAS was accessed by this frontend work.
