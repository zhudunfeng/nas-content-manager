# Backend implementation and validation

- Date: 2026-11-25 (Asia/Shanghai).
- Owner: backend implementer, coordinated with the parent, protocol verifier and independent quality reviewer.
- Scope: `internal/collectioncontract`, `internal/collectionworker`, relevant `internal/app` collection DTO/preview/detail handlers, and `scripts/verify_jmcomic_bridge.py`.
- No live comic site, user account, existing media directory or user service was accessed. No commit was created.

## Implemented contracts

- Search accepts optional fixed `orderBy=mr/mv/mp/tf` and `time=a/t/w/m`; old query/page JSON remains compatible. Category filtering is deliberately absent because the pinned API client's keyword search ignores it.
- Preview exposes ordered chapter IDs, safe titles, real per-chapter page counts, canonical selected chapter IDs and the selected page total. Empty request selection means whole album. Duplicate, malformed and unknown chapter IDs fail closed.
- Whole-album metadata may exceed the current task's MaxFiles while remaining within the absolute 10000-page ceiling. Such previews are marked `overBudget`; both application task creation and worker operation admission reject execution until a smaller selection is previewed. Acquisition independently checks the selected-page budget again.
- Optional chapter request members use `omitempty`. Whole-album identity explicitly serializes the original five-field album structure, preserving old request/preview identity bytes. Explicit selections bind canonical selection and chapter page counts. The quality reviewer additionally owns a frozen-metadata comparison for new full-album records without changing the legacy identity.
- The real downloader's chapter filter preserves every page of selected chapters. Its full-album `all_success` cannot validate subsets; equivalent stricter checks require exact selected chapters, no failure collections, complete unique page sequences, exact raster files and unchanged metadata. Go independently verifies selected numeric paths and real image headers before the original publication gate.
- Public progress is `{completedPages,totalPages,bytes,reusedPages}`. `bytes` means verified page bytes including reuse, not network traffic. It is a separate optional pointer and never changes final `Operation.Bytes` accumulation. Task detail obtains bounded progress from the matching worker operation without a DB schema change or per-page DB writes.
- Explicit JMComic retry retains its operation-owned staging directory. The bridge independently validates each cached ordinary page's source-semantics digest, request/preview binding, SHA256, length, raster format, dimensions, no-link path and current budgets before enabling the upstream per-image cache skip. Corrupt/unrecorded/hash-mismatched pages are refetched; incompatible metadata, hard links, unknown files and unsafe paths fail closed.
- Checkpoint and progress snapshots live only at `private-state/jmcomic/<operation-id>/checkpoint.json` and `progress.json`; staging contains only final selected images. Startup only scans the private-state root's operation JSON, so these sidecars cannot be mistaken for operations.

## Actual checks executed

All build/test/tool commands were launched through `python -B scripts/project_env.py -- ...`; caches and temporary evidence stayed under `.runtime/`.

1. `go vet ./...` passed on the implementation and passed again after temporary test cleanup.
2. `go test ./...` passed on the implementation and passed again after temporary test cleanup. The repository intentionally retains no generated unit-test source from this task.
3. `go build -o .runtime/verify/jmcomic-advanced-worker-v2.exe ./cmd/nas-collection-worker` and `go build -o .runtime/verify/jmcomic-advanced-nas-v2.exe ./cmd/nas` passed. These v2 binaries include the Python atomic replacement fix; the quality reviewer's later Go metadata comparison requires rebuilding the final binaries.
4. Gofmt ran with the repository toolchain at `.tools/go/bin/gofmt.exe`. The first bare `gofmt` attempt was unavailable on PATH; using the explicit repository executable succeeded. An initial worker build used a nonexistent `cmd/collection-worker` path, was corrected to the actual `cmd/nas-collection-worker`, and then succeeded.
5. Three temporary Go behavior-test groups passed with verbose output:
   - `TestJMComicAdvancedCompatibility`: unchanged legacy full-album identity and old request JSON, canonical selection, selected identity, duplicate/unknown rejection and read-only over-budget preview.
   - `TestJMComicAdvancedProgress`: valid strict progress, monotonic counters, page/byte/reuse bounds, unknown fields/trailing JSON rejection and unchanged final Bytes.
   - `TestJMComicAdvancedRetryAndSidecar`: retry preserves owned pages and clears old-attempt progress; nested malformed sidecars do not break real Service.New restart loading.
   The only generated test source, `internal/collectionworker/jmcomic_advanced_temp_test.go`, was deleted after success.
6. The retained real-dependency harness ran against installed `jmcomic==2.7.12`, real option/client/search entity/downloader hooks/image decode/Pillow behavior, with domain discovery, API metadata/search response and image response transport substituted and all real network entry points denied. Final evidence:

   `.runtime/verify/jmcomic-real-bridge-b2944294a242457a83df76bbfb410107/report.json`

   It reports zero real network calls and 22 successful checkpoints:

   - isolated subprocess probe;
   - actual generated API order/time/paging query;
   - numeric redirect;
   - empty search;
   - real chapter page counts;
   - canonical selected chapters and exact selected completeness;
   - invalid/duplicate selection rejection;
   - real four-page/two-chapter decode;
   - GIF preservation;
   - partial-result rejection;
   - private page progress;
   - three independently revalidated reused pages and only one new actual image-response call on retry;
   - corrupt/unreceipted page refetch;
   - checkpoint metadata digest rejection;
   - actual source scramble semantics and request binding drift rejection;
   - tampered page content digest refetch;
   - hardlink/extra-sidecar rejection;
   - retained-staging page and byte budget rejection;
   - fresh file/byte budget rejection;
   - safe partial-failure stdout/stderr protocol;
   - explicit controlled proxy binding;
   - bounded atomic replacement contention retry.

## Windows atomic replacement finding

The protocol verifier observed that the Go progress reader's transient Windows file handle could block the fixture's atomic snapshot rename on its fifth page, leaving a private temporary snapshot and safely failing acquisition. Re-running the same fixture in isolation succeeded, confirming a reader/writer sharing race rather than missing image data. The production Python writer now retries only `PermissionError`, at most 20 attempts separated by 10ms; it never deletes the old complete snapshot. A retained harness fault-injection test proves three transient failures then success and a permanent conflict failing after exactly 20 attempts while preserving the old JSON. The protocol fixture uses the corresponding bounded permission-only retry; the protocol verifier owns final end-to-end evidence.

## Remaining review and target boundaries

- The independent quality reviewer is finalizing new-record per-chapter metadata comparison for whole-album requests whose legacy total page count and chapter IDs remain unchanged. Their patch/tests are outside this implementer's final binary timestamp; rebuild after that patch.
- The protocol verifier and parent own the final advanced API/publication/import/restart/browser regressions and frontend integration. Their fixture evidence must remain distinct from this real-library bridge evidence.
- No live-site reachability, real account behavior, Docker deployment, native distribution packaging, physical NAS performance or long-running target download is asserted. Account/Cookie/favorites, arbitrary YAML/Python plugins and PDF/ZIP are outside this increment.
- Normal hard process cancellation and existing publication/import authority remain the original worker/app mechanisms. Restarted uncertain operations remain subject to the existing explicit reconciliation rules; page reuse does not authorize automatic re-execution or publication.
