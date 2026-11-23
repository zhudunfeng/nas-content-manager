<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, ApiError, errorMessage, isAbort } from '../api'
import { newPasswordHint, validNewPassword } from '../password'
import type { AccountSession, AccountSessionPage } from '../types'
import PasswordInput from './PasswordInput.vue'
import DialogShell from './DialogShell.vue'

const emit = defineEmits<{ requireLogin: [message: string] }>()
const currentPassword = ref<string>('')
const newPassword = ref<string>('')
const confirmPassword = ref<string>('')
const revokePassword = ref<string>('')
// 同值空字符串不会触发输入组件watch；敏感操作后用局部键恢复全部默认掩码。
const secretReset = ref<number>(0)
const passwordBusy = ref<boolean>(false)
const revokeOthersBusy = ref<boolean>(false)
const revokingID = ref<string>('')
const passwordError = ref<string>('')
const revokeOthersError = ref<string>('')
const sessionError = ref<string>('')
const notice = ref<string>('')
const listLoading = ref<boolean>(false)
const listError = ref<string>('')
const sessionPage = ref<AccountSessionPage | null>(null)
const confirmCurrent = ref<boolean>(false)
const mutationBusy = computed<boolean>((): boolean => passwordBusy.value || revokeOthersBusy.value || revokingID.value !== '')
let active: boolean = true
let readGeneration: number = 0
let readController: AbortController | null = null
let mutationController: AbortController | null = null

/** 清除所有短期密码，同时让复用输入框恢复默认掩码。 */
function clearSecrets(): void {
  currentPassword.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
  revokePassword.value = ''
  secretReset.value++
}

/** 取消旧批次读取，阻止撤销后的过时结果重新覆盖当前列表。 */
function cancelRead(): void {
  readGeneration++
  readController?.abort()
  readController = null
  listLoading.value = false
}

/** 只读取一批有界结果；继续查看替换当前批次，不自动追页或无限累积。 */
async function loadSessions(cursor: string = ''): Promise<void> {
  cancelRead()
  const generation: number = readGeneration
  const controller: AbortController = new AbortController()
  readController = controller
  listLoading.value = true
  listError.value = ''
  const query: URLSearchParams = new URLSearchParams({ limit: '20' })
  if (cursor) query.set('cursor', cursor)
  try {
    const result: AccountSessionPage = await api<AccountSessionPage>(`/account/sessions?${query}`, 'GET', undefined, controller.signal)
    if (active && generation === readGeneration) sessionPage.value = result
  } catch (error: unknown) {
    if (active && generation === readGeneration && !isAbort(error)) listError.value = errorMessage(error)
  } finally {
    if (active && generation === readGeneration) { listLoading.value = false; readController = null }
  }
}

/** 安全呈现失败，冲突时仅刷新只读状态，不自动重发含密码的请求。 */
function mutationFailure(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.status === 409) { clearSecrets(); void loadSessions() }
    return error.retryAfterSeconds ? `${error.message}，请至少等待 ${error.retryAfterSeconds} 秒后重试。` : error.message
  }
  return '操作结果尚未确认，请先刷新会话状态；如已退出，请使用新密码登录。请勿自动重复提交。'
}

/** 校验原密码仅限 UTF-8 字节上限，保留旧弱密码和中文密码，不 trim。 */
function validCurrentPassword(password: string): boolean {
  return new TextEncoder().encode(password).byteLength <= 72
}

/** 提交新密码，成功后先清秘密再交由原有会话清理逻辑退出。 */
async function changePassword(): Promise<void> {
  if (mutationBusy.value) return
  passwordError.value = ''
  notice.value = ''
  if (!validCurrentPassword(currentPassword.value)) { passwordError.value = '当前密码不能超过 72 个 UTF-8 字节'; return }
  if (!validNewPassword(newPassword.value)) { passwordError.value = newPasswordHint; return }
  if (newPassword.value !== confirmPassword.value) { passwordError.value = '两次输入的新密码不一致'; return }
  cancelRead()
  passwordBusy.value = true
  const controller: AbortController = new AbortController()
  mutationController = controller
  try {
    const result: { requiresLogin: boolean } = await api('/account/password', 'POST', { currentPassword: currentPassword.value, newPassword: newPassword.value }, controller.signal)
    if (active && result.requiresLogin) { clearSecrets(); emit('requireLogin', '密码已更新，全部旧会话已退出，请使用新密码登录') }
  } catch (error: unknown) {
    if (active && !isAbort(error)) passwordError.value = mutationFailure(error)
  } finally {
    clearSecrets()
    if (active) { passwordBusy.value = false; mutationController = null }
  }
}

/** 再认证后退出其他记录，保留实际当前会话；删除数不称为活跃设备数。 */
async function revokeOthers(): Promise<void> {
  if (mutationBusy.value) return
  revokeOthersError.value = ''
  notice.value = ''
  if (!validCurrentPassword(revokePassword.value)) { revokeOthersError.value = '当前密码不能超过 72 个 UTF-8 字节'; return }
  cancelRead()
  revokeOthersBusy.value = true
  const controller: AbortController = new AbortController()
  mutationController = controller
  try {
    const result: { requiresLogin: false; revokedCount: number } = await api('/account/sessions/revoke-others', 'POST', { currentPassword: revokePassword.value }, controller.signal)
    if (active) { notice.value = `已退出其他会话，删除 ${result.revokedCount} 条会话记录；当前会话保留。`; void loadSessions() }
  } catch (error: unknown) {
    if (active && !isAbort(error)) revokeOthersError.value = mutationFailure(error)
  } finally {
    clearSecrets()
    if (active) { revokeOthersBusy.value = false; mutationController = null }
  }
}

/** 撤销一条本人会话；真实当前会话只能从确认对话框调用。 */
async function revokeSession(session: AccountSession): Promise<void> {
  if (mutationBusy.value) return
  cancelRead()
  sessionError.value = ''
  notice.value = ''
  revokingID.value = session.id
  const controller: AbortController = new AbortController()
  mutationController = controller
  try {
    const result: { requiresLogin: boolean } = await api(`/account/sessions/${session.id}`, 'DELETE', undefined, controller.signal)
    if (!active) return
    confirmCurrent.value = false
    if (result.requiresLogin) { clearSecrets(); emit('requireLogin', '当前会话已退出，请重新登录') }
    else { notice.value = '所选会话已撤销。'; void loadSessions() }
  } catch (error: unknown) {
    if (active && !isAbort(error)) sessionError.value = mutationFailure(error)
  } finally {
    clearSecrets()
    if (active) { revokingID.value = ''; mutationController = null }
  }
}

/** 将有限客户端类别翻译为可读提示，不声称能识别可信设备。 */
function clientLabel(session: AccountSession): string {
  const browsers: Record<AccountSession['clientBrowser'], string> = { unknown: '未知浏览器', chrome: 'Chrome', edge: 'Edge', firefox: 'Firefox', safari: 'Safari', opera: 'Opera', other: '其他浏览器' }
  const systems: Record<AccountSession['clientOS'], string> = { unknown: '未知系统', windows: 'Windows', macos: 'macOS', linux: 'Linux', android: 'Android', ios: 'iOS', other: '其他系统' }
  return `${browsers[session.clientBrowser]} · ${systems[session.clientOS]}`
}

/** 本地化时间；历史创建时间明确显示未知。 */
function sessionTime(value: string | null): string {
  if (value === null) return '未知（历史会话）'
  const date: Date = new Date(value)
  return Number.isNaN(date.getTime()) ? '未知时间' : date.toLocaleString('zh-CN', { hour12: false })
}

onMounted((): void => { void loadSessions() })
onBeforeUnmount((): void => {
  active = false
  clearSecrets()
  cancelRead()
  mutationController?.abort()
})
</script>

<template>
  <section class="settings-panel account-security" aria-labelledby="account-security-title">
    <h2 id="account-security-title">账号安全</h2>
    <p class="muted account-intro">修改管理员密码，管理保存在本机的登录会话。</p>
    <form aria-label="修改密码" class="account-form" @submit.prevent="changePassword">
      <h3>修改密码</h3>
      <label for="account-current-password">当前密码</label><PasswordInput :key="`current-${secretReset}`" id="account-current-password" v-model="currentPassword" autocomplete="current-password" secret-label="当前密码" :disabled="mutationBusy" :aria-describedby="passwordError ? 'account-password-error' : undefined" />
      <label for="account-new-password">新密码</label><PasswordInput :key="`new-${secretReset}`" id="account-new-password" v-model="newPassword" autocomplete="new-password" secret-label="新密码" :disabled="mutationBusy" required :aria-describedby="`account-new-password-hint${passwordError ? ' account-password-error' : ''}`" />
      <p id="account-new-password-hint" class="field-hint">{{ newPasswordHint }}</p>
      <label for="account-confirm-password">确认新密码</label><PasswordInput :key="`confirm-${secretReset}`" id="account-confirm-password" v-model="confirmPassword" autocomplete="new-password" secret-label="确认新密码" :disabled="mutationBusy" required :aria-describedby="passwordError ? 'account-password-error' : undefined" />
      <p id="account-password-error" class="account-error" role="alert">{{ passwordError }}</p>
      <p class="field-hint">更新后所有会话都会退出，需使用新密码重新登录。</p>
      <button type="submit" class="primary-button" :disabled="mutationBusy">{{ passwordBusy ? '正在修改…' : '修改密码' }}</button>
    </form>
    <div class="account-sessions" :aria-busy="listLoading">
      <div class="account-heading"><h3>登录会话</h3><button type="button" class="secondary-button" :disabled="listLoading || mutationBusy" @click="loadSessions()">刷新会话</button></div>
      <p class="field-hint">客户端类别仅作提示。其他会话按随机编号分批展示，非登录时间排序；刷新从头读取。</p>
      <p v-if="listLoading" class="muted" role="status">正在读取会话…</p>
      <p v-if="listError" class="account-error" role="alert">{{ listError }}</p>
      <template v-if="sessionPage">
        <article class="account-session" :data-session-id="sessionPage.currentSession.id" data-current="true">
          <div><strong>{{ clientLabel(sessionPage.currentSession) }}</strong><span class="status-pill">当前会话</span></div>
          <dl><div><dt>创建</dt><dd>{{ sessionTime(sessionPage.currentSession.createdAt) }}</dd></div><div><dt>到期</dt><dd>{{ sessionTime(sessionPage.currentSession.expiresAt) }}</dd></div></dl>
          <button type="button" class="secondary-button danger-text" :disabled="mutationBusy" @click="confirmCurrent = true">撤销当前会话</button>
        </article>
        <article v-for="session in sessionPage.items" :key="session.id" class="account-session" :data-session-id="session.id" data-current="false">
          <strong>{{ clientLabel(session) }}</strong>
          <dl><div><dt>创建</dt><dd>{{ sessionTime(session.createdAt) }}</dd></div><div><dt>到期</dt><dd>{{ sessionTime(session.expiresAt) }}</dd></div></dl>
          <button type="button" class="secondary-button" :disabled="mutationBusy" :aria-label="`撤销会话：${clientLabel(session)}，创建于${sessionTime(session.createdAt)}`" @click="revokeSession(session)">{{ revokingID === session.id ? '正在撤销…' : '撤销会话' }}</button>
        </article>
        <p v-if="!sessionPage.items.length" class="muted" role="status">{{ sessionPage.nextCursor ? '本批无其他有效会话，可继续查看' : '本批无其他有效会话' }}</p>
        <button v-if="sessionPage.nextCursor" type="button" class="secondary-button" :disabled="listLoading || mutationBusy" @click="loadSessions(sessionPage.nextCursor)">继续查看</button>
      </template>
      <p v-if="sessionError" class="account-error" role="alert">{{ sessionError }}</p>
    </div>
    <form aria-label="退出其他会话" class="account-form" @submit.prevent="revokeOthers">
      <h3>退出其他会话</h3>
      <p class="field-hint">保留当前会话，删除其他全部会话记录（含过期记录）。此操作需要再次验证当前密码。</p>
      <label for="account-revoke-password">验证当前密码</label><PasswordInput :key="`revoke-${secretReset}`" id="account-revoke-password" v-model="revokePassword" autocomplete="current-password" secret-label="验证当前密码" :disabled="mutationBusy" :aria-describedby="revokeOthersError ? 'account-revoke-error' : undefined" />
      <p id="account-revoke-error" class="account-error" role="alert">{{ revokeOthersError }}</p>
      <button type="submit" class="secondary-button" :disabled="mutationBusy">{{ revokeOthersBusy ? '正在退出…' : '退出其他会话' }}</button>
    </form>
    <p v-if="notice" class="account-notice" role="status">{{ notice }}</p>
  </section>
  <DialogShell v-if="confirmCurrent && sessionPage" title="撤销当前会话" @close="confirmCurrent = false">
    <div class="account-confirm"><p>确认后此浏览器将立即退出，需要重新登录。其他已准入的媒体传输可能继续完成。</p><p v-if="sessionError" class="account-error" role="alert">{{ sessionError }}</p><div class="account-heading"><button type="button" class="secondary-button" :disabled="mutationBusy" @click="confirmCurrent = false">取消</button><button type="button" class="primary-button" :disabled="mutationBusy" @click="revokeSession(sessionPage.currentSession)">{{ mutationBusy ? '正在退出…' : '确认退出当前会话' }}</button></div></div>
  </DialogShell>
</template>

<style scoped>
.account-intro { margin-top: 12px; line-height: 1.7; }
.account-security h3 { font-size: 15px; }
.account-form, .account-sessions { display: grid; gap: 14px; margin-top: 24px; padding-top: 24px; border-top: 1px solid var(--line); min-width: 0; }
.account-form label { font-size: 13px; }
.account-form > button, .account-sessions > button { justify-self: start; }
.account-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 12px; }
.account-session { display: grid; gap: 12px; padding: 15px; border: 1px solid var(--line); border-radius: 8px; min-width: 0; }
.account-session strong { font-size: 13px; }
.account-session .status-pill { margin-inline-start: 8px; }
.account-session dl { display: grid; gap: 8px; margin: 0; font-size: 12px; }
.account-session dl > div { display: flex; flex-wrap: wrap; gap: 8px; }
.account-session dt { color: var(--muted); }
.account-session dd { margin: 0; overflow-wrap: anywhere; }
.account-session button { justify-self: start; }
.account-error { color: var(--danger); font-size: 13px; line-height: 1.7; overflow-wrap: anywhere; }
.account-error:empty { display: none; }
.account-notice { margin-top: 18px; color: var(--accent); font-size: 13px; line-height: 1.7; }
.account-confirm { display: grid; gap: 22px; padding: 24px; line-height: 1.8; }
.field-hint { line-height: 1.7; overflow-wrap: anywhere; }
</style>
