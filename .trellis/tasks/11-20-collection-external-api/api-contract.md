# External automation integration contract

## User flow and scope

Select a service, enter its base address and existing API credential, save and test. No API-version selector. Advanced completion-directory mapping is optional at connection time; registration remains a separate explicit preview/scan. Manage subscriptions, queue and history through one service panel. Show only detected capabilities. Never install services, forward arbitrary HTTP, delete files or take over downloaders.

## Source configuration

Existing source DTO gains `externalService: ''|moviepilot|autobangumi|sonarr|radarr` and `apiConfigured:boolean`. Write-only source input gains optional `connection:{baseUrl,apiKey}` and `clearConnection:boolean`. Blank fields retain existing values for the same service; changing service requires both values. Secrets and private endpoint live in a new random private file per config revision. `collection_external_connections(source_id,revision,private_ref)` points to it. A short transaction saves source + reference together. Source edits never erase credentials implicitly. Generic external sources remain supported. Completion `rootId` may be empty for API-enabled external sources; import requires a valid authorized media root. `allowPrivate` remains explicit.

## Browser endpoints (authenticated /api/collection)

- `POST /sources/{id}/external/test {revision}` → `{info:Info}`. No mutations.
- `GET /sources/{id}/external/items?section=resources|queue|history&page=1` → `Page` (includes info). Separate per-section errors; never disguise failure as empty data.
- `GET /sources/{id}/external/options` → `Options`. Existing roots/profiles only.
- `GET /sources/{id}/external/lookup?q=...` → `{items:Item[]}`. Metadata lookup only, not download search. Only Servarr supports it initially.
- `GET /sources/{id}/external/actions` → `{actions:Action[]}`, latest 30 records, no private input.
- `POST /sources/{id}/external/actions` with `{requestId,revision,kind,targetId?,externalId?,title?,mediaType?,season?,rootId?,profileId?,enabled?,rssUrl?}` → `{action:Action}`. Kinds `add|monitor|search`. `rssUrl` is write-only and only AB add accepts it. MP media identity is TMDB, Sonarr TVDB, Radarr TMDB. Target IDs are server IDs from current lists; AB uses `bangumi:N|rss:N`.
- `POST /sources/{id}/external/actions/{actionId}/reconcile {}` → `{action:Action}`. Read-only reconciliation, never resends the mutation.

Action `{id,requestId,sourceId,sourceRevision,kind,targetId,state,message,createdAt,updatedAt}`; state `dispatching|succeeded|failed|uncertain|unconfirmed`. Same UUID + same normalized immutable intent returns original action; differing intent is 409. Config revision mismatch is 409 for new intent. Existing accepted UUID remains retrievable after config changes, but reconciliation requires the original matching connection revision. An unresolved action blocks new writes for that source until reconciled or explicitly acknowledged as unconfirmed. Before transport, persist the action and its private immutable intent. After restart dispatching becomes uncertain. Never auto-retry an uncertain write. A confirmed HTTP success is a receipt, not evidence of local import. Search without a durable known remote receipt remains uncertain after transport failure. No broad search commands.

## Adapter package contract

`internal/externalapi`: root owns `types.go`, `client.go` (HTTP budgets, config validation, fixed service dispatch); adapter agents own `servarr.go`, `moviepilot.go`, `autobangumi.go`. Client `config Config`, `http *http.Client`, `info Info`, `prefix string`. HTTP helper `call(ctx,method,path string,query url.Values,input any,output any) error`; path must be fixed by adapter. Max body 4 MiB, max upstream arrays 1000. No raw error/response text. `Fault{Message string,Rejected bool}` indicates whether mutation rejection is definite; unknown is conservative uncertain. `callMP` unwraps MP envelope in its own file. Every method/field has Chinese comments; all boundaries explicit typed.

Exported methods: `New(config Config,httpClient *http.Client) (*Client,error)`, `Probe(ctx) (Info,error)`, `List(ctx,section string,page int) (Page,error)`, `Options(ctx) (Options,error)`, `Lookup(ctx,query string) ([]Item,error)`, `Execute(ctx,mutation Mutation) (Outcome,error)`, `Reconcile(ctx,mutation Mutation) (Outcome,bool,error)`. Probe before any other method. Adapter methods are `probeServarr/listServarr/optionsServarr/lookupServarr/executeServarr/reconcileServarr`, MP suffix `MoviePilot`, AB suffix `AutoBangumi`. Reconcile bool true only if desired state/identity uniquely confirmed. It must never dispatch a write.

## Security and storage

Use existing real DNS/IP validation via injected HTTP transport, no environment proxy, no cookie jar, reject every redirect. Default TLS verification. URL base rejects credentials/query/fragment/control chars/escaped separators/traversal. Private/loopback only with allowPrivate; reserved/metadata always denied. Only allowlisted fields returned; discard raw health/error messages, provider URLs, RSS URLs, private paths. Options expose remote directory labels only as explicit authenticated service choices; never treat them as local directory authorization.

Schema4 adds connections, external_actions and external_choices with source/UUID/intent hashes and public fields. No API secrets or private endpoint in DB/backup. Immutable mutation and choice private_ref reference private files. No DB transaction or shared collection lock crosses network. A bounded external operation semaphore blocks source edits while a mutation is in flight; it is distinct from the collection coordinator lock. Shutdown cancels outbound calls through app lifecycle.

## Manual uncertainty acknowledgement

`POST /sources/{id}/external/actions/{actionId}/acknowledge {checkedExternally:true}` only accepts uncertain actions with no in-flight writer. It records state `unconfirmed`, explicitly says the user checked externally and the system did not prove success, and unblocks new explicit writes. It never retries an upstream request. Reconcile uses the immutable original config only while it still equals the currently consented connection; name-only revisions do not redirect the original operation.

## MoviePilot six-stage workflow extension

Authenticated base `/sources/{id}/external/moviepilot`. All choices use server-generated opaque UUIDs bound to source revision and a 15-minute private snapshot. `Choice={id,title,kind,status,externalId,mediaType,year,season,organizeAllowed}`. No raw contexts, torrent links/cookies/passkeys, downloader names, storage paths or scheduler payloads go to browser/DB. Responses `{choices:Choice[],message:string}`. Unknown/expired snapshots are409, wrong kind is400, changed source revision is409. At most100 choices per result,600 active choices per source,60KiB private payload per choice. Only local storage directories already configured in MP can be browsed; files stay within the selected configured root.

- `POST /search {revision,query,mediaType:'movie'|'series'}`: media metadata choices (`kind=media`).
- `POST /releases {revision,selectionId}`: release choices (`kind=release`), based on the selected private metadata context.
- `GET /downloads`: current queue with private download identity choices (`kind=download`).
- `GET /roots`: configured local download/library choices (`download_root|library_root`).
- `POST /files {revision,rootId,parentId?}`: one bounded directory page (`dir|file`). No client filesystem paths.
- `POST /organize/preview {revision,fileId,targetId,externalId,mediaType}`: fixed copy preview; returns one `organize_plan`. No filesystem mutation at this stage. The actual configured target must have `overwrite_mode=never` and `organizeAllowed=true`; reject ambiguous overlapping roots. `ManualTransferItem` has no per-request overwrite_mode field. Re-read configured policy and observed single-file size/mtime immediately before execution; organization never automatically scrapes.
- `POST /scrape/preview {revision,fileId,externalId,mediaType}`: validates a library file and metadata identity without starting scrape, returns one `scrape_plan`.
- `GET /library`: configured fixed `mediaserver_sync` scheduler choice (`library_job`) and its current state. This refreshes **MoviePilot library index from configured media servers**, not an Emby/Plex filesystem rescan; display that exact distinction.

Generic action input extends `Mutation` with `selectionId` and `confirmOverwrite`. Kinds `download|download_start|download_stop|organize|scrape|refresh_library` accept only the matching opaque choice, never raw upstream payloads. No other mutation fields accepted for these kinds. `confirmOverwrite=true` is mandatory only for scrape. Private adapter data is hydrated server-side from the snapshot and independently revalidated by fixed API methods. A choice may be consumed by only one action UUID (download pause/resume use separate fresh list snapshots). Repeating the accepted UUID returns its original action. Side effects use the same durable action journal, uncertainty and acknowledgement flow. Search/preview reads never become automatic downloads. UI keeps native modal ownership and progressive disclosure.

## Supported capabilities

Sonarr4 / Radarr7: resources, queue, history, options, lookup, add, monitor, search. Unknown product versions detected read-only/unsupported, no inferred writes. Sonarr5 only if separately verified, otherwise disable. MP3 basic: resources, queue, history, add exact TMDB/type/season, monitor active R/P versus paused S, explicit one-subscription search. The six-stage MoviePilot extension is enabled only for verified exact version3.2.8 (optional leading v), not all MP3 versions. AB4: combined bangumi/RSS resources, partial downloader queue, add RSS, monitor; no global history or generic search. Unknown versions do not enable writes. Read budgets fail explicitly rather than silently truncate. Side-effecting scheduler GET uses a separate no-keepalive transport to prevent Go automatic GET replay; its receipt confirms submission, not a unique completed run.
