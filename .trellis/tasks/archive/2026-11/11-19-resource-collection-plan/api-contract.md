# Resource collection API — implementation contract

Base `/api/collection`, same-origin session auth. Mutations strict JSON. Every collection GET lists arrays as `[]`. IDs are UUID strings except library/item IDs. Every edit/action carries `revision`; create-task `requestId` is a UUID. Errors use `{error:string}`.

## Types (camelCase wire fields)

```ts
type CollectionKind = 'rss'|'web'|'local'|'remote'|'external'
interface CollectionCapability {kind:string;available:boolean;reason:string;version:string}
interface CollectionWorkerSource {id:string;kind:string;name:string}
interface CollectionCapabilities {capabilities:CollectionCapability[];workerSources:CollectionWorkerSource[];roots:{id:string;name:string;path:string}[];workerReady:boolean;workerReason:string}
interface CollectionSource {id:string;kind:CollectionKind;name:string;enabled:boolean;revision:number;hasSecret:boolean;rootId:string;path:string;allowPrivate:boolean;refreshMinutes:number;outputProfile:string;lastSuccessAt:string;lastError:string;externalUrl:string;baselineAt:string;createdAt:string;updatedAt:string}
interface CollectionSourceInput {kind:CollectionKind;name:string;enabled:boolean;url?:string;rootId:string;path:string;allowPrivate:boolean;refreshMinutes:number;outputProfile:string;externalUrl:string;revision?:number}
interface CollectionRule {include:string;exclude:string;quality:string;minBytes:number;maxBytes:number;episodePattern:string}
interface CollectionSubscription {id:string;sourceId:string;name:string;enabled:boolean;revision:number;contentType:string;mode:'review'|'automatic';rule:CollectionRule;createLibrary:boolean;libraryId:number;selectionPattern:string;nextRunAt:string;createdAt:string;updatedAt:string}
interface CollectionSubscriptionInput {sourceId:string;name:string;enabled:boolean;contentType:string;mode:'review'|'automatic';rule:CollectionRule;createLibrary:boolean;libraryId:number;selectionPattern:string;revision?:number}
interface CollectionCandidate {id:string;sourceId:string;title:string;sizeBytes:number;publishedAt:string;decision:string;reason:string;revision:number;createdAt:string;taskId:string}
interface CollectionFile {path:string;size:number;modTime:number;sha256:string}
interface CollectionFormat {id:string;label:string;extension:string}
interface CollectionPreview {id:string;sourceId:string;title:string;kind:string;profile:string;files:CollectionFile[];formats:CollectionFormat[];totalBytes:number;expiresAt:string;warnings:string[]}
interface CollectionTask {id:string;requestId:string;sourceId:string;candidateId:string;kind:string;title:string;contentType:string;state:string;importState:string;message:string;revision:number;downloadId:string;libraryId:number;jobId:number;itemIds:number[];bytes:number;createdAt:string;updatedAt:string}
interface CollectionEvent {id:number;taskId:string;stage:string;message:string;createdAt:string}
```

## Routes

- `GET /capabilities` → CollectionCapabilities. `roots[].id` is local source root ID; `workerSources[]` are worker-configured roots/remotes. local read-only registration uses roots, local-copy uses worker source IDs.
- `GET /sources` → `{sources:CollectionSource[]}`; `POST /sources` (CollectionSourceInput) → `{source}` 201; `PATCH /sources/{id}` (full CollectionSourceInput with revision) → `{source}`. URL is write-only, omit or empty on edit to retain. All source credentials/URLs stay outside ordinary DB/DTO. externalUrl may only be a public no-query HTTP(S) external application entry.
- `POST /sources/{id}/test` `{}` → `{ok:boolean,message:string}`; no discovery or download.
- `POST /sources/{id}/refresh` `{backfill:boolean}` → `{added:number,message:string}`; RSS only. First refresh stores baseline candidates as `baseline`; backfill explicitly makes historical candidates reviewable. Later refresh only processes unseen entries.
- `GET /subscriptions` → `{subscriptions:CollectionSubscription[]}`; POST collection / PATCH `/{id}` (full input + revision on edit) → `{subscription}`.
- `POST /subscriptions/{id}/preview` `{}` → `{candidates:CollectionCandidate[],matched:number}`; no acquiring.
- `POST /subscriptions/{id}/run` `{}` → `{added:number,message:string}`; enabled source required, bounded refresh then evaluate rules.
- `GET /candidates?sourceId=&page=1&pageSize=30` → `{candidates,total,page,pageSize}`.
- `POST /candidates/{id}/ignore` `{revision:number}` → `{ok:true}`.
- `POST /candidates/{id}/acquire` `{requestId:string,revision:number,contentType:string,createLibrary:boolean,libraryId:number}` → `{task:CollectionTask}` 202. BT may require explicit file selection in existing download center.
- `POST /previews` `{sourceId:string,url:string,path:string,profile:'video'|'audio',mode:'register'|'copy'}` → `{preview:CollectionPreview}`. source.kind web uses submitted URL; local mode register uses local configured root and path; local copy/remote uses worker. path is relative, empty means selected source path. Single URL only; batch UI sends bounded sequential calls independently.
- `POST /tasks` `{requestId:string,previewId:string,contentType:string,createLibrary:boolean,libraryId:number}` → `{task:CollectionTask}` 202. immutable preview-bound plan, retains same requestId on uncertain retry.
- `GET /tasks?page=&pageSize=&state=` → `{tasks,total,page,pageSize}`; `GET /tasks/{id}` → `{task}`.
- `GET /tasks/{id}/events` → `{events:CollectionEvent[]}`.
- `POST /tasks/{id}/actions` `{action:'cancel'|'retry'|'retry_import',revision:number}` → `{task}`. Fetch/import phases remain separate; BT actions/selection use existing DownloadCenter.

Source disable and subscription disable are PATCH enabled:false with revision; no deletion of data. UI must distinguish failed capability requests, unavailable worker and empty data. UI may link `downloadId` into existing download details and `itemIds` into media items via existing lifecycle.

Idempotency is bound only after acceptance: the original requestId with the same immutable plan returns the same task. A different requestId for an already collected fingerprint returns409 with an explicit duplicate explanation; it is not silently accepted as an alias. Reusing an accepted requestId for another target/content returns409. Deleting a library invalidates associated collection item IDs immediately and retains media files; published non-BT tasks permit explicit retry_import, unpublished tasks permit acquisition recovery, and BT recovery stays in DownloadCenter. Historical create-library intent never silently recreates the deleted target. Completed BT collection records continue bounded periodic reconciliation with original download publication/coverage facts.
