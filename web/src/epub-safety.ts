import DOMPurify from 'dompurify'
import type Section from 'epubjs/types/section'

/** 单个 EPUB 归档允许的最大条目数。 */
const maxEntries: number = 5000
/** 单项允许的最大实际解压字节数。 */
const maxEntryBytes: number = 32 * 1024 ** 2
/** 全部条目允许的最大实际解压字节数。 */
const maxTotalBytes: number = 256 * 1024 ** 2

/** 已通过结构校验的 EPUB 压缩数据边界。 */
export interface EpubArchiveEntry {
  /** 压缩数据的起始字节偏移。 */ dataStart: number
  /** 压缩数据的结束字节偏移。 */ dataEnd: number
  /** 压缩方法，零为直接存储，八为 DEFLATE。 */ method: number
  /** 中央目录声明的解压字节数。 */ expandedSize: number
}

/** 本地记录及其压缩数据的已验证边界。 */
interface ZipLocalBoundary {
  /** 本地文件头起点。 */ start: number
  /** 包含数据描述符的记录终点。 */ end: number
  /** 压缩数据起点。 */ dataStart: number
  /** 压缩数据终点。 */ dataEnd: number
}

/** 中央目录中用于核对本地文件头的字段。 */
interface ZipEntryBoundary {
  /** 本地文件头的绝对偏移。 */ localOffset: number
  /** 通用标志位。 */ flags: number
  /** 压缩方法编号。 */ method: number
  /** 内容校验值。 */ crc: number
  /** 压缩数据的字节数。 */ compressedSize: number
  /** 声明的解压字节数。 */ expandedSize: number
  /** 中央目录文件名的起始偏移。 */ nameOffset: number
  /** 文件名的字节数。 */ nameLength: number
}

/** 校验 ZIP 扩展字段的完整边界，并拒绝首版不支持的 ZIP64。 */
function validateZipExtra(view: DataView, start: number, length: number): void {
  const end: number = start + length
  let offset: number = start
  while (offset < end) {
    if (offset + 4 > end) throw new Error('EPUB 扩展字段不完整')
    const kind: number = view.getUint16(offset, true)
    const size: number = view.getUint16(offset + 2, true)
    if (kind === 0x0001) throw new Error('EPUB 暂不支持 ZIP64 归档')
    offset += 4 + size
    if (offset > end) throw new Error('EPUB 扩展字段越界')
  }
}

/** 核对可选数据描述符，返回其末尾，避免本地记录与目录使用不同长度。 */
function dataDescriptorEnd(view: DataView, start: number, limit: number, entry: ZipEntryBoundary): number {
  // 标准描述符允许有签名或无签名，两种形式都必须与中央目录一致。
  const candidates: number[] = [start]
  if (start + 4 <= limit && view.getUint32(start, true) === 0x08074b50) candidates.unshift(start + 4)
  for (const offset of candidates) {
    if (offset + 12 <= limit && view.getUint32(offset, true) === entry.crc && view.getUint32(offset + 4, true) === entry.compressedSize && view.getUint32(offset + 8, true) === entry.expandedSize) return offset + 12
  }
  throw new Error('EPUB 数据描述符损坏或已截断')
}

/** 校验本地文件头与数据区，返回不可与其他条目重叠的字节区间。 */
function validateLocalEntry(view: DataView, centralOffset: number, entry: ZipEntryBoundary): ZipLocalBoundary {
  const offset: number = entry.localOffset
  if (offset + 30 > centralOffset || view.getUint32(offset, true) !== 0x04034b50) throw new Error('EPUB 本地文件头偏移无效')
  const flags: number = view.getUint16(offset + 6, true)
  const method: number = view.getUint16(offset + 8, true)
  const crc: number = view.getUint32(offset + 14, true)
  const compressedSize: number = view.getUint32(offset + 18, true)
  const expandedSize: number = view.getUint32(offset + 22, true)
  const nameLength: number = view.getUint16(offset + 26, true)
  const extraLength: number = view.getUint16(offset + 28, true)
  const dataStart: number = offset + 30 + nameLength + extraLength
  if (dataStart > centralOffset || nameLength !== entry.nameLength || flags !== entry.flags || method !== entry.method) throw new Error('EPUB 本地文件头与中央目录不一致')
  if (compressedSize === 0xffffffff || expandedSize === 0xffffffff) throw new Error('EPUB 暂不支持 ZIP64 归档')
  for (let index: number = 0; index < nameLength; index++) {
    if (view.getUint8(offset + 30 + index) !== view.getUint8(entry.nameOffset + index)) throw new Error('EPUB 本地文件名与中央目录不一致')
  }
  validateZipExtra(view, offset + 30 + nameLength, extraLength)
  const usesDescriptor: boolean = (flags & 0x0008) !== 0
  // 流式 ZIP 可暂以零填充本地大小；最终数值必须由描述符和中央目录共同确认。
  if (usesDescriptor ? (crc !== 0 && crc !== entry.crc) || (compressedSize !== 0 && compressedSize !== entry.compressedSize) || (expandedSize !== 0 && expandedSize !== entry.expandedSize) : crc !== entry.crc || compressedSize !== entry.compressedSize || expandedSize !== entry.expandedSize) throw new Error('EPUB 本地文件大小或校验值不一致')
  const dataEnd: number = dataStart + entry.compressedSize
  if (dataEnd > centralOffset) throw new Error('EPUB 压缩数据越过中央目录边界')
  return { start: offset, end: usesDescriptor ? dataDescriptorEnd(view, dataEnd, centralOffset, entry) : dataEnd, dataStart, dataEnd }
}

/** EPUB 解压前按真实目录边界计数，限制条目数与声明解压大小。 */
export function validateEpubArchive(buffer: ArrayBuffer): EpubArchiveEntry[] {
  const view: DataView = new DataView(buffer)
  if (view.byteLength < 22) throw new Error('EPUB 压缩目录无效，无法安全读取')
  let eocd: number = -1
  // 与 JSZip 一样选择最后一个结束签名，避免双方解析不同的伪造目录。
  for (let position: number = view.byteLength - 4; position >= Math.max(0, view.byteLength - 65557); position--) {
    if (view.getUint32(position, true) === 0x06054b50) { eocd = position; break }
  }
  if (eocd < 0 || eocd + 22 > view.byteLength || eocd + 22 + view.getUint16(eocd + 20, true) !== view.byteLength) throw new Error('EPUB 结束记录或归档注释长度无效')
  const disk: number = view.getUint16(eocd + 4, true)
  const centralDisk: number = view.getUint16(eocd + 6, true)
  const diskCount: number = view.getUint16(eocd + 8, true)
  const count: number = view.getUint16(eocd + 10, true)
  const centralSize: number = view.getUint32(eocd + 12, true)
  const centralOffset: number = view.getUint32(eocd + 16, true)
  if (disk !== 0 || centralDisk !== 0 || diskCount !== count || count === 0xffff || count > maxEntries || centralSize === 0xffffffff || centralOffset === 0xffffffff) throw new Error('EPUB 超出首版安全限制（最多 5000 个条目，不支持分卷或 ZIP64）')
  const centralEnd: number = centralOffset + centralSize
  if (centralEnd !== eocd) throw new Error('EPUB 中央目录范围无效，不支持附加前后缀或截断归档')
  let offset: number = centralOffset
  let actualCount: number = 0
  let total: number = 0
  const localRanges: [number, number][] = []
  const entries: EpubArchiveEntry[] = []
  // 不信任 EOCD 的条目计数，遍历完整中央目录后再核对实际条数。
  while (offset < centralEnd) {
    if (offset + 46 > centralEnd || view.getUint32(offset, true) !== 0x02014b50) throw new Error('EPUB 中央目录损坏或已截断')
    actualCount++
    if (actualCount > maxEntries) throw new Error('EPUB 实际条目超过 5000 个，无法安全读取')
    const size: number = view.getUint32(offset + 24, true)
    const flags: number = view.getUint16(offset + 8, true)
    const compressedSize: number = view.getUint32(offset + 20, true)
    const method: number = view.getUint16(offset + 10, true)
    const nameLength: number = view.getUint16(offset + 28, true)
    const extraLength: number = view.getUint16(offset + 30, true)
    const commentLength: number = view.getUint16(offset + 32, true)
    const localOffset: number = view.getUint32(offset + 42, true)
    const entryEnd: number = offset + 46 + nameLength + extraLength + commentLength
    if (!nameLength || entryEnd > centralEnd) throw new Error('EPUB 中央目录变长字段越界')
    if (compressedSize === 0xffffffff || size === 0xffffffff || localOffset === 0xffffffff || view.getUint16(offset + 34, true) !== 0) throw new Error('EPUB 暂不支持 ZIP64 或分卷条目')
    total += size
    if (size > maxEntryBytes || total > maxTotalBytes || (flags & 0x2041) !== 0) throw new Error('EPUB 内容过大或已加密，暂不支持读取')
    if (method !== 0 && method !== 8) throw new Error('EPUB 使用了首版不支持的压缩方式')
    if (method === 0 && compressedSize !== size) throw new Error('EPUB 未压缩条目的大小不一致')
    validateZipExtra(view, offset + 46 + nameLength, extraLength)
    const local: ZipLocalBoundary = validateLocalEntry(view, centralOffset, { localOffset, flags, method, crc: view.getUint32(offset + 16, true), compressedSize, expandedSize: size, nameOffset: offset + 46, nameLength })
    localRanges.push([local.start, local.end])
    entries.push({ dataStart: local.dataStart, dataEnd: local.dataEnd, method, expandedSize: size })
    offset = entryEnd
  }
  if (offset !== centralEnd || actualCount !== count) throw new Error('EPUB 实际目录条目数与声明不一致')
  localRanges.sort((left: [number, number], right: [number, number]): number => left[0] - right[0])
  let previous: [number, number] | undefined
  for (const range of localRanges) {
    if (!previous && range[0] !== 0) throw new Error('EPUB 不支持自解压前缀')
    if (previous && range[0] < previous[1]) throw new Error('EPUB 本地文件条目相互重叠')
    previous = range
  }
  return entries
}

/** 分批让出主线程，并在解压流程继续之前确认用户尚未取消。 */
async function yieldEpubParsing(signal: AbortSignal): Promise<void> {
  await new Promise<void>((resolve: () => void): void => { setTimeout(resolve, 0) })
  if (signal.aborted) throw new DOMException('读取已取消', 'AbortError')
}

/** 流式计数真实解压输出，拒绝伪造大小，不保留解压后的内容。 */
export async function validateEpubInflation(buffer: ArrayBuffer, entries: EpubArchiveEntry[], signal: AbortSignal): Promise<void> {
  /** 压缩输入每块最多一 KiB，限制高压缩比数据单次产生的输出。 */
  const inputChunkBytes: number = 1024
  /** 每处理十六个解压块主动让出主线程一次。 */
  const yieldEveryChunks: number = 16
  let total: number = 0
  let chunks: number = 0
  for (const entry of entries) {
    if (signal.aborted) throw new DOMException('读取已取消', 'AbortError')
    if (entry.method === 0) {
      const actual: number = entry.dataEnd - entry.dataStart
      total += actual
      if (actual !== entry.expandedSize || actual > maxEntryBytes || total > maxTotalBytes) throw new Error('EPUB 实际内容超过大小限制或与声明不一致')
      if (++chunks % yieldEveryChunks === 0) await yieldEpubParsing(signal)
      continue
    }
    let decompressor: DecompressionStream
    try { decompressor = new DecompressionStream('deflate-raw') }
    catch { throw new Error('当前浏览器不支持安全的 EPUB 解压，请升级浏览器或使用本地阅读器') }
    let offset: number = entry.dataStart
    const compressed: ReadableStream<Uint8Array> = new ReadableStream<Uint8Array>({
      /** 按需投递小块压缩数据，避免解压器预先消费整个条目。 */
      pull(controller: ReadableStreamDefaultController<Uint8Array>): void {
        if (signal.aborted) { controller.error(new DOMException('读取已取消', 'AbortError')); return }
        if (offset >= entry.dataEnd) { controller.close(); return }
        const length: number = Math.min(inputChunkBytes, entry.dataEnd - offset)
        controller.enqueue(new Uint8Array(buffer, offset, length))
        offset += length
      },
    })
    const reader: ReadableStreamDefaultReader<Uint8Array> = compressed.pipeThrough(decompressor).getReader()
    /** 用户关闭阅读器时立即终止当前解压流，取消异常不覆盖原始退出原因。 */
    function cancelInflation(): void { void reader.cancel().catch((): void => { /* 清理已结束的原生流时无需替换业务错误。 */ }) }
    signal.addEventListener('abort', cancelInflation, { once: true })
    let expanded: number = 0
    try {
      while (true) {
        if (signal.aborted) throw new DOMException('读取已取消', 'AbortError')
        const part: ReadableStreamReadResult<Uint8Array> = await reader.read()
        if (signal.aborted) throw new DOMException('读取已取消', 'AbortError')
        if (part.done) break
        expanded += part.value.byteLength
        total += part.value.byteLength
        if (expanded > entry.expandedSize || expanded > maxEntryBytes || total > maxTotalBytes) throw new Error('EPUB 实际解压内容超过声明或安全限额')
        if (++chunks % yieldEveryChunks === 0) await yieldEpubParsing(signal)
      }
      if (expanded !== entry.expandedSize) throw new Error('EPUB 实际解压大小与声明不一致')
    } catch (error: unknown) {
      await reader.cancel().catch((): void => { /* 原生流可能已经失败，保留首次校验错误。 */ })
      if (signal.aborted) throw new DOMException('读取已取消', 'AbortError')
      throw error instanceof Error && error.message.startsWith('EPUB') ? error : new Error('EPUB 压缩数据无效，无法安全读取')
    } finally { signal.removeEventListener('abort', cancelInflation); reader.releaseLock() }
  }
}

/** 在章节进入 sandbox 之前去除活动内容，并阻止外网与同源敏感资源请求。 */
export function sanitizeEpubSection(_output: string, section: Section): void {
  // 保留前序资源替换钩子生成的 blob URL，随后再统一施加内容安全边界。
  const safe: string = DOMPurify.sanitize(section.output, {
    WHOLE_DOCUMENT: true,
    FORBID_TAGS: ['script', 'iframe', 'object', 'embed', 'form', 'input', 'button', 'textarea', 'base', 'audio', 'video'],
    FORBID_ATTR: ['srcdoc', 'autofocus', 'formaction', 'action', 'ping'],
    ADD_ATTR: ['xmlns', 'epub:type'],
    ALLOWED_URI_REGEXP: /^(?:(?:blob|data):|[^a-z]|[a-z+.-]+(?:[^a-z+.-:]|$))/i,
  })
  const doc: Document = new DOMParser().parseFromString(safe, 'text/html')
  doc.querySelectorAll('meta[http-equiv]').forEach((node: Element): void => node.remove())
  const policy: HTMLMetaElement = doc.createElement('meta')
  policy.httpEquiv = 'Content-Security-Policy'
  policy.content = "default-src 'none'; script-src 'none'; img-src blob: data:; style-src 'unsafe-inline' blob:; font-src blob: data:; connect-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'"
  doc.head.prepend(policy)
  doc.querySelectorAll('a').forEach((link: HTMLAnchorElement): void => {
    const href: string = link.getAttribute('href') || ''
    if (/^(?:[a-z][\w+.-]*:|\/\/)/i.test(href)) link.removeAttribute('href')
    link.removeAttribute('target')
  })
  section.output = '<!doctype html>\n' + doc.documentElement.outerHTML
}

/** 有界下载 EPUB；视频绝不使用此路径。 */
export async function fetchEpub(url: string, signal: AbortSignal): Promise<ArrayBuffer> {
  const maxBytes: number = 64 * 1024 ** 2
  const response: Response = await fetch(url, { credentials: 'same-origin', signal })
  if (response.status === 401) window.dispatchEvent(new Event('session-expired'))
  if (!response.ok) throw new Error('电子书读取失败，请检查文件与会话')
  if (Number(response.headers.get('Content-Length')) > maxBytes) throw new Error('此 EPUB 大于 64 MiB，超出首版浏览器阅读限额')
  const reader: ReadableStreamDefaultReader<Uint8Array> | undefined = response.body?.getReader()
  if (!reader) throw new Error('浏览器不支持安全读取电子书')
  const chunks: Uint8Array[] = []
  let size: number = 0
  try {
    while (true) {
      const part: ReadableStreamReadResult<Uint8Array> = await reader.read()
      if (part.done) break
      size += part.value.byteLength
      if (size > maxBytes) throw new Error('此 EPUB 超过 64 MiB，请使用本地阅读器打开')
      chunks.push(part.value)
    }
  } catch (error: unknown) { await reader.cancel(); throw error }
  const output: Uint8Array = new Uint8Array(size)
  let offset: number = 0
  for (const chunk of chunks) { output.set(chunk, offset); offset += chunk.byteLength }
  const entries: EpubArchiveEntry[] = validateEpubArchive(output.buffer)
  await validateEpubInflation(output.buffer, entries, signal)
  return output.buffer
}
