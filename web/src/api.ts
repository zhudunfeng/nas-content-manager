/** 保留中文服务端错误与状态码的异常。 */
export class ApiError extends Error {
  /** HTTP 状态码。 */ status: number

  /** 创建请求错误，保留可供会话过期处理的状态码。 */
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

/** 发送同源 JSON 请求；媒体流始终交由浏览器直接请求。 */
export async function api<T>(path: string, method: string = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(`/api${path}`, {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
  if (!response.ok) {
    const failure = await response.json().catch((): { error: string } => ({ error: '服务暂时不可用，请稍后重试' })) as { error?: string }
    if (response.status === 401) window.dispatchEvent(new Event('session-expired'))
    throw new ApiError(failure.error || `请求失败（${response.status}）`, response.status)
  }
  return await response.json() as T
}

/** 上传有界表单，让浏览器自行生成 multipart 分隔符。 */
export async function apiMultipart<T>(path: string, body: FormData, signal?: AbortSignal): Promise<T> {
  const response: Response = await fetch(`/api${path}`, { method: 'POST', credentials: 'same-origin', body, signal })
  if (!response.ok) {
    const failure = await response.json().catch((): { error: string } => ({ error: '上传暂时未能确认，请保留当前表单重试' })) as { error?: string }
    if (response.status === 401) window.dispatchEvent(new Event('session-expired'))
    throw new ApiError(failure.error || `上传失败（${response.status}）`, response.status)
  }
  return await response.json() as T
}

/** 在 HTTP 局域网与 HTTPS 环境均可生成随机幂等键。 */
export function createRequestId(): string {
  const bytes: Uint8Array = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  bytes[6] = ((bytes[6] ?? 0) % 16) + 64
  bytes[8] = ((bytes[8] ?? 0) % 64) + 128
  const hex: string = Array.from(bytes, (value: number): string => value.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/** 将未知异常转换为安全的用户提示。 */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '发生未知错误，请重试'
}

/** 判断请求是否被主动取消。 */
export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}
