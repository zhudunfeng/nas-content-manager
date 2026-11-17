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

/** 将未知异常转换为安全的用户提示。 */
export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : '发生未知错误，请重试'
}

/** 判断请求是否被主动取消。 */
export function isAbort(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}
