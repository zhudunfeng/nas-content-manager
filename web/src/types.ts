/** 内容的五种分类。 */
export type ContentType = 'video' | 'image' | 'comic' | 'novel' | 'music'
/** 应用的导航页面。 */
export type ViewName = 'home' | ContentType | 'favorites' | 'libraries' | 'collection' | 'downloads' | 'jobs' | 'settings'
/** 当前登录的管理员。 */
export interface User {
  /** 用户编号。 */ id: number
  /** 账户名称。 */ username: string
}
/** 公开的有限会话元数据，不含 bearer 凭据或原始客户端信息。 */
export interface AccountSession {
  /** 独立随机公开编号。 */ id: string
  /** 是否为实际 Cookie 对应的当前会话。 */ current: boolean
  /** 浏览器类别，仅为提示。 */ clientBrowser: 'unknown' | 'chrome' | 'edge' | 'firefox' | 'safari' | 'opera' | 'other'
  /** 操作系统类别，仅为提示。 */ clientOS: 'unknown' | 'windows' | 'macos' | 'linux' | 'android' | 'ios' | 'other'
  /** 实际创建时间；历史未知值为空。 */ createdAt: string | null
  /** 精确到期时间。 */ expiresAt: string
}
/** 一批有界会话结果；空批次仍可能允许继续。 */
export interface AccountSessionPage {
  /** 单独固定的真实当前会话。 */ currentSession: AccountSession
  /** 仅本批其他有效会话。 */ items: AccountSession[]
  /** 最后消费原始行的游标，末批为空。 */ nextCursor: string | null
}
/** 媒体库的挂载与索引状态。 */
export interface Library {
  /** 媒体库编号。 */ id: number
  /** 显示名称。 */ name: string
  /** 内容分类。 */ type: ContentType
  /** 受控媒体根目录中的路径。 */ path: string
  /** 索引状态。 */ status: string
  /** 已索引的内容数量。 */ itemCount: number
  /** 最后扫描时间。 */ lastScannedAt: string
}
/** 按用户保存的播放或阅读进度。 */
export interface Progress {
  /** 秒数、页码或文本段落位置。 */ position: number
  /** 总时长、总页数或总段落数。 */ total: number
  /** EPUB 定位信息或阅读器辅助状态。 */ locator: string
  /** 进度更新时间。 */ updatedAt: string
}
/** 五类内容共享的索引模型。 */
export interface Item {
  /** 内容编号。 */ id: number
  /** 所属媒体库编号。 */ libraryId: number
  /** 内容标题。 */ title: string
  /** 内容分类。 */ type: ContentType
  /** 库内相对路径。 */ relativePath: string
  /** 原始文件字节数。 */ size: number
  /** 媒体类型。 */ mime: string
  /** 已鉴权的封面地址。 */ coverUrl: string
  /** 添加时间。 */ addedAt: string
  /** 更新时间。 */ updatedAt: string
  /** 源文件是否缺失。 */ missing: boolean
  /** 是否收藏。 */ favorite: boolean
  /** 用户标签。 */ tags: string[]
  /** 当前用户进度。 */ progress: Progress | null
  /** 视频或音乐时长，单位秒。 */ duration: number
  /** 图像或视频宽度。 */ width: number
  /** 图像或视频高度。 */ height: number
}
/** 持久化后台任务。 */
export interface Job {
  /** 任务编号。 */ id: number
  /** 所属媒体库编号。 */ libraryId: number
  /** 扫描或衍生处理类型。 */ kind: string
  /** 任务生命周期。 */ status: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled'
  /** 已处理内容数量。 */ processed: number
  /** 中文任务说明。 */ message: string
  /** 创建时间。 */ createdAt: string
  /** 最后更新时间。 */ updatedAt: string
}
/** 目录选择结果。 */
export interface BrowseResult {
  /** 当前规范化目录。 */ path: string
  /** 父目录，根节点为空。 */ parent: string
  /** 可进入的目录条目。 */ entries: { /** 文件名。 */ name: string; /** 完整目录。 */ path: string; /** 是否目录。 */ isDir: boolean }[]
  /** 允许访问的根目录。 */ roots: string[]
}
/** 分页内容结果。 */
export interface ItemPage {
  /** 本页内容。 */ items: Item[]
  /** 满足筛选条件的总数。 */ total: number
  /** 当前页码。 */ page: number
  /** 分页大小。 */ pageSize: number
}
/** 全库统计，不代表宿主机磁盘容量。 */
export interface Stats {
  /** 内容总数。 */ total: number
  /** 视频数。 */ video: number
  /** 图片数。 */ image: number
  /** 漫画数。 */ comic: number
  /** 小说数。 */ novel: number
  /** 音乐数。 */ music: number
  /** 已索引内容的字节总量。 */ totalSize: number
}
/** 服务端只读配置。 */
export interface Settings {
  /** 应用版本。 */ version: string
  /** 业务数据目录。 */ dataDir: string
  /** 媒体挂载目录。 */ mediaRoots: string[]
  /** 定期扫描间隔，单位分钟。 */ scanIntervalMinutes: number
  /** 衍生缓存容量上限。 */ cacheMaxBytes: number
  /** FFmpeg 是否可用。 */ ffmpegAvailable: boolean
  /** ffprobe 是否可用。 */ ffprobeAvailable: boolean
}
/** 漫画图片页面。 */
export interface ComicPage {
  /** 零起始页序。 */ index: number
  /** 自然排序的页面名称。 */ name: string
  /** 鉴权后的页面 URL。 */ url: string
}
/** TXT 解析结果。 */
export interface TextBook {
  /** 书名。 */ title: string
  /** 正文段落。 */ paragraphs: string[]
  /** 章节及起始段落。 */ chapters: { /** 章节名称。 */ title: string; /** 起始段落。 */ index: number }[]
  /** 是否因安全限额而截断。 */ truncated: boolean
}

/** 下载器连接状态，不包含引擎凭据或原始下载来源。 */
export interface DownloadStatus {
  /** 是否已在部署配置中启用。 */ enabled: boolean
  /** 连接状态，与单个下载的状态相互独立。 */ state: string
  /** 实际下载引擎版本。 */ version: string
  /** 实际 WebAPI 版本。 */ webApiVersion: string
  /** 最近成功同步时间。 */ lastSyncedAt: string
  /** 已脱敏的连接错误。 */ error: string
  /** 当前引擎能否安全等待用户选择文件。 */ canSelectFiles: boolean
}

/** 一个由栖库管理的 BT 下载任务。 */
export interface Download {
  /** 本地任务编号，不是引擎哈希。 */ id: string
  /** 安全显示名称。 */ name: string
  /** 下载完成后的入库分类。 */ contentType: ContentType
  /** 目标媒体库编号，尚未创建时为空。 */ libraryId: number | null
  /** 归一化传输状态。 */ state: string
  /** 引擎原始状态，仅用于诊断。 */ engineState: string
  /** 独立的发布与入库状态。 */ importState: string
  /** 所选文件总字节数。 */ sizeBytes: number
  /** 已下载的字节数。 */ downloadedBytes: number
  /** 下载速度，单位字节每秒。 */ downloadSpeed: number
  /** 上传速度，单位字节每秒。 */ uploadSpeed: number
  /** 累计上传字节数。 */ uploadedBytes: number
  /** 无单位分享率。 */ ratio: number
  /** 当前连接的节点数。 */ peers: number
  /** 预计剩余秒数，无有效估计时为空。 */ etaSeconds: number | null
  /** 后端文件清单版本。 */ manifestVersion: number
  /** 正式开始后锁定选集。 */ selectionLocked: boolean
  /** 并发操作修订号。 */ revision: number
  /** 文件已经过安全发布验证。 */ ready: boolean
  /** 是否已移除下载管理，原文件保留。 */ archived: boolean
  /** 最近成功同步时间。 */ lastSyncedAt: string
  /** 已脱敏的业务错误。 */ lastError: string
  /** 本地任务创建时间。 */ createdAt: string
  /** 最近更新时间。 */ updatedAt: string
}

/** 分页下载列表。 */
export interface DownloadPage {
  /** 本页受管任务。 */ downloads: Download[]
  /** 任务总数。 */ total: number
  /** 当前页码。 */ page: number
  /** 每页数量。 */ pageSize: number
}

/** 受控文件清单的一项。 */
export interface DownloadFile {
  /** 引擎返回的稳定文件编号，不能用数组下标代替。 */ index: number
  /** 相对于任务目录的安全路径。 */ path: string
  /** 文件字节数。 */ sizeBytes: number
  /** 是否包含在当前选集中。 */ selected: boolean
  /** 完成比例，范围为零至一。 */ progress: number
}

/** 分页文件清单及并发选择凭据。 */
export interface DownloadFilePage {
  /** 本页文件。 */ files: DownloadFile[]
  /** 当前清单文件总数。 */ total: number
  /** 当前页码。 */ page: number
  /** 每页数量。 */ pageSize: number
  /** 所有分页必须保持相同的清单版本。 */ manifestVersion: number
  /** 确认下载后不可再次改变选集。 */ selectionLocked: boolean
}

/** 允许通过栖库提交的受管任务操作。 */
export type DownloadAction = 'start' | 'pause' | 'reconcile' | 'remove'

/** 搜种时可选择的标准内容范围，与下载后的五类媒体类型相互独立。 */
export type TorrentSearchCategory = 'all' | 'video' | 'music' | 'book'

/** 可公开展示的索引来源，永不包含服务地址或 API 密钥。 */
export interface TorrentSearchSource {
  /** 服务端配置的稳定来源编号。 */ id: string
  /** 管理员设置的来源显示名。 */ name: string
}

/** 搜种模块的公开能力和受控来源列表。 */
export interface TorrentSearchStatus {
  /** 是否已在服务端启用搜种。 */ enabled: boolean
  /** 当前配置的可搜索来源。 */ sources: TorrentSearchSource[]
  /** 单次返回的结果上限。 */ maxResults: number
  /** 结果令牌在服务端保留的秒数。 */ resultTtlSeconds: number
}

/** 一条安全的搜索结果，不承载磁力、私密下载地址或来源凭据。 */
export interface TorrentSearchResult {
  /** 由服务端随机生成的短期结果编号。 */ id: string
  /** 只作为文本显示的资源标题。 */ title: string
  /** 选中结果所属的来源编号。 */ sourceId: string
  /** 来源的中文或站点显示名。 */ sourceName: string
  /** 资源字节数，未知时为空。 */ sizeBytes: number | null
  /** 来源报告的做种数，未知时为空。 */ seeders: number | null
  /** 来源报告的下载数，未知时为空。 */ leechers: number | null
  /** 来源报告的发布时间，未知时为空字符串。 */ publishedAt: string
  /** 来源是否提供可由后端安全解析的下载输入。 */ downloadable: boolean
}

/** 本次查询中单个来源的可用性，错误已由后端脱敏。 */
export interface TorrentSearchSourceStatus extends TorrentSearchSource {
  /** 当前来源是否返回了有效结果。 */ state: 'ready' | 'error'
  /** 不包含 URL 或凭据的来源错误。 */ error: string
}

/** 单次有界搜索快照，由前端在本地排序和分页。 */
export interface TorrentSearchResponse {
  /** 已去重且限制数量的安全结果。 */ results: TorrentSearchResult[]
  /** 本次实际搜索的来源状态。 */ sources: TorrentSearchSourceStatus[]
  /** 服务端结果令牌的 UTC 到期时间。 */ expiresAt: string
  /** 是否受到单源或聚合结果上限限制。 */ truncated: boolean
}

/** 可托管的两个固定组件，浏览器不能指定任意镜像。 */
export type EngineComponentId = 'qbittorrent' | 'jackett'

/** 引擎组件的部署归属与应用连接状态。 */
export interface EngineComponent {
  /** 固定组件标识。 */ id: EngineComponentId
  /** 部署配置指定的管理模式。 */ mode: 'disabled' | 'external' | 'managed'
  /** Docker 元数据确认的存在情况。 */ presence: string
  /** 容器运行状态，和应用连接分开判断。 */ runtime: string
  /** 应用实际适配器的连接就绪情况。 */ readiness: string
  /** 不包含地址或凭据的中文诊断。 */ message: string
  /** 启动操作所需的乐观修订号。 */ revision: number
  /** 后端是否已确认允许初始化。 */ canInitialize: boolean
  /** 后端是否已确认允许启动原受管实例。 */ canStart: boolean
}

/** 初始化控制面的只读安全快照。 */
export interface EngineStatus {
  /** 私有助手的可用性。 */ helper: string
  /** 已脱敏的控制面说明。 */ message: string
  /** 固定组件的独立状态。 */ components: EngineComponent[]
  /** 重新打开后可继续观察的持久任务编号。 */ activeTaskId: string
  /** qBittorrent 首次使用告知版本。 */ noticeVersion: string
  /** qBittorrent 首次使用告知文本。 */ noticeText: string
  /** Jackett 仅限宿主回环的配置端口。 */ setupPort: number
}

/** 一层镜像的真实处理进度，不代表整体完成百分比。 */
export interface EngineLayer {
  /** 可公开显示的镜像层短标识。 */ id: string
  /** 受控中文层状态。 */ status: string
  /** 实际处理的字节数。 */ current: number
  /** 实际总字节数，零表示未知。 */ total: number
}

/** 单组件的持久安装步骤。 */
export interface EngineStep {
  /** 固定组件标识。 */ component: EngineComponentId
  /** 当前阶段。 */ phase: string
  /** 阶段的执行状态。 */ state: string
  /** 尝试次数。 */ attempt: number
  /** 已脱敏的中文进度或错误。 */ message: string
  /** 最新且有界的镜像层状态。 */ layers: EngineLayer[]
}

/** 助手独立持久化的初始化任务。 */
export interface EngineTask {
  /** 服务端任务编号。 */ id: string
  /** 首次授权的初始化或启动操作。 */ action: 'initialize' | 'start'
  /** 任务执行状态。 */ state: string
  /** 重试时必须提交的乐观修订号。 */ revision: number
  /** 本任务确认过的固定组件集合。 */ components: EngineComponentId[]
  /** 每个组件的独立安装结果。 */ steps: EngineStep[]
  /** 创建时间。 */ createdAt: string
  /** 最近更新时间。 */ updatedAt: string
  /** 不含原始引擎响应的中文说明。 */ message: string
}

/** 初始化任务的安全中文事件。 */
export interface EngineEvent {
  /** 单任务递增事件序号。 */ sequence: number
  /** 关联的固定组件。 */ component: EngineComponentId | ''
  /** 信息或错误级别。 */ level: string
  /** 已脱敏并有长度上限的中文事件。 */ message: string
  /** 事件时间。 */ createdAt: string
}

/** 可恢复轮询的任务快照及增量事件。 */
export interface EngineTaskResponse {
  /** 最新完整任务。 */ task: EngineTask
  /** 本次新增的有界事件。 */ events: EngineEvent[]
  /** 后续读取使用的事件游标。 */ nextSequence: number
  /** 事件历史已过期时要求重载快照。 */ reloadRequired: boolean
}

/** 来源同步只返回可公开的候选名称和稳定编号。 */
export interface EngineSourceSync {
  /** 已配置的安全来源候选。 */ candidates: TorrentSearchSource[]
  /** 当前实际接入的来源编号。 */ selectedIds: string[]
  /** 超过八个来源时必须由用户明确选择。 */ selectionRequired: boolean
  /** 搜索就绪或等待配置的状态。 */ state: string
  /** 不包含站点地址或密钥的说明。 */ message: string
}

/** 收集来源的固定类型，不增加媒体分类。 */
export type CollectionKind = 'rss' | 'web' | 'local' | 'remote' | 'external' | 'jmcomic' | 'uav'
/** UAV 首版支持的固定视频站点。 */
export type CollectionUAVSite = 'jable' | 'missav' | 'hanime1' | 'huangguo' | 'supjav'
/** UAV 只向浏览器公开选择所需的有界元数据。 */
export interface CollectionUAVItem {
  /** 当前站点内的视频编号。 */ videoId: string
  /** 纯文本视频标题。 */ title: string
  /** 上游提供的时长文本，空值表示未知。 */ duration: string
}
/** UAV 只返回当前页，不伪造上游未知总数。 */
export interface CollectionUAVPage {
  /** 本次实际选择的固定站点。 */ site: CollectionUAVSite
  /** 本次实际关键词。 */ query: string
  /** 从一开始的页码。 */ page: number
  /** 是否可继续翻页。 */ hasNext: boolean
  /** 当前页视频元数据。 */ items: CollectionUAVItem[]
}
/** JMComic 搜索结果仅包含可展示的漫画信息。 */
export interface CollectionJMComicItem {
  /** 正整数字符串漫画编号。 */ albumId: string
  /** 纯文本标题。 */ title: string
  /** 有界纯文本标签。 */ tags: string[]
}
/** JMComic 每次只返回当前页，不在浏览器无限积累结果。 */
export interface CollectionJMComicPage {
  /** 本次实际查询。 */ query: string
  /** 一开始的页码。 */ page: number
  /** 上游已知结果总数。 */ total: number
  /** 是否可继续翻页。 */ hasNext: boolean
  /** 当前页漫画清单。 */ items: CollectionJMComicItem[]
}
/** 固定外部服务；空值保留仅入口和成品目录模式。 */
export type CollectionExternalService = '' | 'moviepilot' | 'autobangumi' | 'sonarr' | 'radarr'
/** 一个可检测的收集能力。 */
export interface CollectionCapability {
  /** 固定能力标识。 */ kind: string
  /** 当前是否实际可用。 */ available: boolean
  /** 中文能力诊断。 */ reason: string
  /** 实际工具版本。 */ version: string
}
/** 执行端预配置的受控目录或连接。 */
export interface CollectionWorkerSource {
  /** 受控来源编号。 */ id: string
  /** 本地目录或远程连接类型。 */ kind: string
  /** 安全显示名称。 */ name: string
}
/** 收集模块的平台与目录能力。 */
export interface CollectionCapabilities {
  /** 各固定适配器的实际能力。 */ capabilities: CollectionCapability[]
  /** 执行端已经配置的来源。 */ workerSources: CollectionWorkerSource[]
  /** 可只读登记的授权根目录。 */ roots: { /** 根目录编号。 */ id: string; /** 显示名称。 */ name: string; /** 已授权根路径。 */ path: string }[]
  /** 执行端是否已连接。 */ workerReady: boolean
  /** 执行端中文诊断。 */ workerReason: string
}
/** 不包含私密来源地址的收集来源。 */
export interface CollectionSource {
  /** 来源编号。 */ id: string
  /** 固定来源类型。 */ kind: CollectionKind
  /** 显示名称。 */ name: string
  /** 是否允许新收集。 */ enabled: boolean
  /** 并发修改版本。 */ revision: number
  /** 是否保存了私密配置。 */ hasSecret: boolean
  /** 预配置目录或连接编号。 */ rootId: string
  /** 根目录以内的相对路径。 */ path: string
  /** 是否明确允许此来源的私网访问。 */ allowPrivate: boolean
  /** RSS 刷新间隔，单位分钟。 */ refreshMinutes: number
  /** 网页输出规格或本地导入模式。 */ outputProfile: string
  /** 最近成功时间。 */ lastSuccessAt: string
  /** 已脱敏的最近错误。 */ lastError: string
  /** 允许公开展示的外部应用入口。 */ externalUrl: string
  /** 已选择的固定外部服务。 */ externalService: CollectionExternalService
  /** 是否存在仅写入的 API 连接配置。 */ apiConfigured: boolean
  /** RSS 首次基线时间。 */ baselineAt: string
  /** 创建时间。 */ createdAt: string
  /** 更新时间。 */ updatedAt: string
}
/** 创建或完整修订来源的提交体。 */
export interface CollectionSourceInput {
  /** 固定来源类型。 */ kind: CollectionKind
  /** 显示名称。 */ name: string
  /** 是否启用来源。 */ enabled: boolean
  /** 仅写入的来源地址，编辑留空保留。 */ url?: string
  /** 预配置目录或连接编号。 */ rootId: string
  /** 根目录以内的相对路径。 */ path: string
  /** 此来源是否允许私网。 */ allowPrivate: boolean
  /** RSS 刷新间隔分钟数。 */ refreshMinutes: number
  /** 输出规格或本地导入模式。 */ outputProfile: string
  /** 无凭据、无查询参数的公开应用入口。 */ externalUrl: string
  /** 固定外部服务，普通来源不设置。 */ externalService?: CollectionExternalService
  /** 仅写入的服务地址和凭据；字段留空保留同一服务的已有值。 */ connection?: { /** 私密实例基址。 */ baseUrl: string; /** 已有 API 凭据。 */ apiKey: string }
  /** 明确断开连接，普通编辑不能隐式清除。 */ clearConnection?: boolean
  /** 编辑所需的乐观修订号。 */ revision?: number
}
/** 外部服务连接与真实能力快照。 */
export interface CollectionExternalInfo {
  /** 固定服务标识。 */ service: CollectionExternalService
  /** 实际检测的产品版本。 */ version: string
  /** 是否为已确认支持的版本。 */ supported: boolean
  /** 服务端确认的固定能力。 */ capabilities: string[]
  /** 脱敏中文说明。 */ message: string
}
/** 外部订阅、任务或历史的公开白名单字段。 */
export interface CollectionExternalItem {
  /** 明确的上游目标编号。 */ id: string
  /** TVDB 或 TMDB 元数据编号。 */ externalId: string
  /** 安全标题。 */ title: string
  /** movie/series/rss/bangumi 类型。 */ mediaType: string
  /** 年份，零表示未知。 */ year: number
  /** 季号，零表示未指定。 */ season: number
  /** 启用状态，未知时为空。 */ enabled: boolean | null
  /** 上游状态，不等于本系统入库状态。 */ status: string
  /** 百分比进度，未知时为空。 */ progress: number | null
  /** 历史记录时间。 */ date: string
}
/** 单个外部业务分区的有界列表。 */
export interface CollectionExternalPage {
  /** 当前连接能力。 */ info: CollectionExternalInfo
  /** 公开条目。 */ items: CollectionExternalItem[]
  /** 已知总数。 */ total: number
  /** 页码。 */ page: number
  /** 每页上限。 */ pageSize: number
  /** 部分视图等限制说明。 */ message: string
}
/** 已有外部目录或画质选项。 */
export interface CollectionExternalOption {
  /** 外部选项编号。 */ id: string
  /** 安全显示名称。 */ name: string
}
/** 新增受监控资源时的只读选择，不构成本地目录授权。 */
export interface CollectionExternalOptions {
  /** 实例中已有的外部目录。 */ roots: CollectionExternalOption[]
  /** 实例中已有的画质配置。 */ profiles: CollectionExternalOption[]
}
/** 固定外部业务操作，不能传任意路径或原始 JSON。 */
export interface CollectionExternalMutation {
  /** 单目标动作。 */ kind: 'add' | 'monitor' | 'search' | 'download' | 'download_start' | 'download_stop' | 'organize' | 'scrape' | 'refresh_library'
  /** 绑定版本和有效期的私密工作流选择编号。 */ selectionId?: string
  /** 仅刮削时明确确认覆盖已有元数据。 */ confirmOverwrite?: boolean
  /** 上游目标编号。 */ targetId?: string
  /** 明确的元数据身份。 */ externalId?: string
  /** 用户确认的资源或 RSS 名称。 */ title?: string
  /** movie/series/rss。 */ mediaType?: string
  /** MP 电视剧季号。 */ season?: number
  /** 已有外部目录编号。 */ rootId?: string
  /** 已有外部画质编号。 */ profileId?: string
  /** 目标启用状态，不使用相对切换。 */ enabled?: boolean
  /** AB 仅写入的 RSS 地址。 */ rssUrl?: string
}
/** 持久动作的安全公开结果。 */
export interface CollectionExternalAction {
  /** 本地动作编号。 */ id: string
  /** 幂等请求编号。 */ requestId: string
  /** 来源编号。 */ sourceId: string
  /** 接受时的来源版本。 */ sourceRevision: number
  /** 动作种类。 */ kind: string
  /** 已确认的上游目标。 */ targetId: string
  /** 已持久化的执行事实；人工解除阻塞不等于成功。 */ state: 'dispatching' | 'succeeded' | 'failed' | 'uncertain' | 'unconfirmed'
  /** 脱敏中文结果。 */ message: string
  /** 创建时间。 */ createdAt: string
  /** 更新时间。 */ updatedAt: string
}
/** MoviePilot 受控选择，不包含原始资源、路径或凭据。 */
export interface CollectionMoviePilotChoice {
  /** 服务端不透明选择编号。 */ id: string
  /** 可安全展示的标题。 */ title: string
  /** 服务端确认的选择种类。 */ kind: 'media' | 'release' | 'download' | 'download_root' | 'library_root' | 'file' | 'dir' | 'organize_plan' | 'scrape_plan' | 'library_job'
  /** 实际外部状态，仅作说明。 */ status: string
  /** 已确认的 TMDB 身份。 */ externalId: string
  /** 电影或电视剧类型。 */ mediaType: string
  /** 年份，零表示未知。 */ year: number
  /** 季号，零表示未知。 */ season: number
  /** 仅已配置为不覆盖的媒体目录允许作为整理目标。 */ organizeAllowed?: boolean
}
/** MoviePilot 每一步的有界受控选项。 */
export interface CollectionMoviePilotChoices {
  /** 当前版本可执行的选择。 */ choices: CollectionMoviePilotChoice[]
  /** 中文状态、范围或限制说明。 */ message: string
}
/** RSS 的确定性匹配规则。 */
export interface CollectionRule {
  /** 必须包含的关键词。 */ include: string
  /** 排除关键词。 */ exclude: string
  /** 质量关键词。 */ quality: string
  /** 最小字节数，零表示不限制。 */ minBytes: number
  /** 最大字节数，零表示不限制。 */ maxBytes: number
  /** 明确的剧集匹配表达式。 */ episodePattern: string
}
/** 已保存的订阅与授权规则。 */
export interface CollectionSubscription {
  /** 订阅编号。 */ id: string
  /** 所属 RSS 来源。 */ sourceId: string
  /** 显示名称。 */ name: string
  /** 是否运行此规则。 */ enabled: boolean
  /** 并发修改版本。 */ revision: number
  /** 目标媒体分类。 */ contentType: ContentType
  /** 待确认或明确授权的自动模式。 */ mode: 'review' | 'automatic'
  /** 匹配与排除规则。 */ rule: CollectionRule
  /** 是否独立创建目标库。 */ createLibrary: boolean
  /** 现有目标库编号。 */ libraryId: number
  /** 明确的文件选择表达式。 */ selectionPattern: string
  /** 下次计划运行时间。 */ nextRunAt: string
  /** 创建时间。 */ createdAt: string
  /** 更新时间。 */ updatedAt: string
}
/** 保存订阅的完整提交体。 */
export type CollectionSubscriptionInput = Omit<CollectionSubscription, 'id' | 'revision' | 'nextRunAt' | 'createdAt' | 'updatedAt'> & { /** 编辑所需的乐观修订号。 */ revision?: number }
/** 持久保存的脱敏候选资源。 */
export interface CollectionCandidate {
  /** 候选编号。 */ id: string
  /** 所属来源。 */ sourceId: string
  /** 来源标题，仅按文本展示。 */ title: string
  /** 已知字节数，零表示未知。 */ sizeBytes: number
  /** 发布时间。 */ publishedAt: string
  /** 匹配和人工决策状态。 */ decision: string
  /** 脱敏中文决策说明。 */ reason: string
  /** 并发决策版本。 */ revision: number
  /** 发现时间。 */ createdAt: string
  /** 已关联的任务编号。 */ taskId: string
}
/** 经过受控预览的文件。 */
export interface CollectionFile {
  /** 安全相对路径。 */ path: string
  /** 字节数。 */ size: number
  /** 源文件修改时间。 */ modTime: number
  /** 文件完整性摘要。 */ sha256: string
}
/** 网页预览的格式说明。 */
export interface CollectionFormat {
  /** 固定适配器格式编号。 */ id: string
  /** 可显示的格式说明。 */ label: string
  /** 输出扩展名。 */ extension: string
}
/** 有时效且绑定来源计划的预览。 */
export interface CollectionPreview {
  /** 服务端预览编号。 */ id: string
  /** 受控来源编号。 */ sourceId: string
  /** 预览标题。 */ title: string
  /** 获取方式。 */ kind: string
  /** 输出规格或导入模式。 */ profile: string
  /** 预览文件清单。 */ files: CollectionFile[]
  /** 可展示的格式信息。 */ formats: CollectionFormat[]
  /** 已知总字节数。 */ totalBytes: number
  /** 预览失效时间。 */ expiresAt: string
  /** 有界中文预览说明。 */ warnings: string[]
}
/** 获取与入库分别保存状态的统一任务。 */
export interface CollectionTask {
  /** 任务编号。 */ id: string
  /** 首次提交时的幂等键。 */ requestId: string
  /** 关联来源编号。 */ sourceId: string
  /** 关联候选编号。 */ candidateId: string
  /** 获取方式。 */ kind: string
  /** 安全显示标题。 */ title: string
  /** 目标媒体分类。 */ contentType: ContentType
  /** 获取阶段状态。 */ state: string
  /** 独立入库状态。 */ importState: string
  /** 已脱敏的业务说明。 */ message: string
  /** 操作修订号。 */ revision: number
  /** 原 BT 下载编号。 */ downloadId: string
  /** 目标媒体库编号。 */ libraryId: number
  /** 当前扫描任务编号。 */ jobId: number
  /** 实际入库媒体编号。 */ itemIds: number[]
  /** 已知总字节数。 */ bytes: number
  /** 创建时间。 */ createdAt: string
  /** 最近更新时间。 */ updatedAt: string
}
/** 脱敏的中文任务事件。 */
export interface CollectionEvent {
  /** 事件编号。 */ id: number
  /** 所属任务编号。 */ taskId: string
  /** 实际执行阶段。 */ stage: string
  /** 中文事件内容。 */ message: string
  /** 发生时间。 */ createdAt: string
}
/** 统一任务列表的分页结果。 */
export interface CollectionTaskPage {
  /** 当前页任务。 */ tasks: CollectionTask[]
  /** 满足筛选的总数。 */ total: number
  /** 当前页码。 */ page: number
  /** 每页大小。 */ pageSize: number
}
/** 候选列表的分页结果。 */
export interface CollectionCandidatePage {
  /** 当前页候选。 */ candidates: CollectionCandidate[]
  /** 满足筛选的总数。 */ total: number
  /** 当前页码。 */ page: number
  /** 每页大小。 */ pageSize: number
}
