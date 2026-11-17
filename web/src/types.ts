/** 内容的五种分类。 */
export type ContentType = 'video' | 'image' | 'comic' | 'novel' | 'music'
/** 应用的导航页面。 */
export type ViewName = 'home' | ContentType | 'favorites' | 'libraries' | 'jobs' | 'settings'
/** 当前登录的管理员。 */
export interface User {
  /** 用户编号。 */ id: number
  /** 账户名称。 */ username: string
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
