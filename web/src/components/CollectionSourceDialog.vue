<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { api, errorMessage, isAbort } from '../api'
import { collectionKindLabels } from '../collection'
import { collectionComponentLocalEntry } from '../collection-components'
import type { CollectionComponent } from '../collection-components'
import type { CollectionCapabilities, CollectionExternalService, CollectionKind, CollectionSource, CollectionSourceInput, CollectionWorkerSource } from '../types'
import DialogShell from './DialogShell.vue'
import Icon from './Icon.vue'
import PasswordInput from './PasswordInput.vue'
import CollectionComponentDeploy from './CollectionComponentDeploy.vue'

/** 来源配置只接收公开字段，秘密永不预填。 */
interface Props {
  /** 编辑的已有来源，空值表示新建。 */ source?: CollectionSource | null
  /** 服务端返回的目录与执行端能力。 */ capabilities: CollectionCapabilities | null
  /** 预选的新来源类型。 */ initialKind?: CollectionKind
}
const props = defineProps<Props>()
const emit = defineEmits<{ close: []; saved: [source: CollectionSource] }>()
const kind = ref<CollectionKind>(props.source?.kind || props.initialKind || 'rss')
const name = ref<string>(props.source?.name || (props.initialKind === 'jmcomic' ? 'JMComic 漫画' : ''))
const enabled = ref<boolean>(props.source?.enabled ?? true)
const url = ref<string>('')
const rootId = ref<string>(props.source?.rootId || '')
const path = ref<string>(props.source?.path || '')
const allowPrivate = ref<boolean>(props.source?.allowPrivate || false)
const refreshMinutes = ref<number>(props.source?.refreshMinutes || 30)
const outputProfile = ref<string>(props.source?.outputProfile || 'video')
const localMode = ref<'register' | 'copy'>(!props.source || props.source.rootId.startsWith('media-') ? 'register' : 'copy')
const externalUrl = ref<string>(props.source?.externalUrl || '')
/** 外部服务名称也是新建来源的建议名称。 */
const serviceNames: Record<CollectionExternalService, string> = { '': '仅入口与成品目录', moviepilot: 'MoviePilot', autobangumi: 'AutoBangumi', sonarr: 'Sonarr', radarr: 'Radarr' }
const externalService = ref<CollectionExternalService>(props.source ? props.source.externalService || '' : 'moviepilot')
const baseUrl = ref<string>('')
const apiKey = ref<string>('')
/** 已有连接保持原接入流程，新部署在同一弹窗中逐步完成。 */
const externalMode = ref<'connect' | 'deploy'>('connect')
/** 仅记录系统生成的入口，切换服务时不覆盖用户另填的入口。 */
const deployedEntrySuggestion = ref<string>('')
/** 仅记录系统生成的完成目录根，用于组件切换时移除过期建议。 */
const deployedRootSuggestion = ref<string>('')
/** 系统生成的完成子目录，与目录根共同判断建议是否仍未修改。 */
const deployedPathSuggestion = ref<string>('')
/** 部署完成切回连接时将焦点交给内部服务地址。 */
const baseUrlInput = ref<HTMLInputElement | null>(null)
const clearConnection = ref<boolean>(false)
const saving = ref<boolean>(false)
const error = ref<string>('')
const controller: AbortController = new AbortController()
const usesDirectory = computed<boolean>((): boolean => ['local', 'remote', 'external'].includes(kind.value))
const usesAPI = computed<boolean>((): boolean => kind.value === 'external' && externalService.value !== '')
const keepsConnection = computed<boolean>((): boolean => !!props.source?.apiConfigured && props.source.externalService === externalService.value && !clearConnection.value)
const register = computed<boolean>((): boolean => kind.value === 'external' || (kind.value === 'local' && localMode.value === 'register'))
const workerChoices = computed<CollectionWorkerSource[]>((): CollectionWorkerSource[] => (props.capabilities?.workerSources || []).filter((source: CollectionWorkerSource): boolean => kind.value === 'remote' ? source.kind === 'remote' : source.kind === 'local'))

/** 新建时切换类型并丢弃不适用的目录及私密输入。 */
function changeKind(): void {
  clearDeployedSuggestions()
  rootId.value = ''
  url.value = ''
  path.value = ''
  outputProfile.value = 'video'
  localMode.value = 'register'
  baseUrl.value = ''
  apiKey.value = ''
  clearConnection.value = false
  externalMode.value = 'connect'
  if (kind.value === 'external' && !name.value.trim()) name.value = serviceNames[externalService.value]
  if (kind.value === 'jmcomic' && !name.value.trim()) name.value = 'JMComic 漫画'
}
/** 本地模式改变时重新选择真实配置的目录。 */
function changeMode(): void { rootId.value = '' }
/** 清理仍未被用户修改的部署建议，防止跨组件连接到旧入口或目录。 */
function clearDeployedSuggestions(): void {
  if (deployedEntrySuggestion.value && externalUrl.value === deployedEntrySuggestion.value) externalUrl.value = ''
  if (deployedRootSuggestion.value && rootId.value === deployedRootSuggestion.value && path.value === deployedPathSuggestion.value) {
    rootId.value = ''
    path.value = ''
  }
  deployedEntrySuggestion.value = ''
  deployedRootSuggestion.value = ''
  deployedPathSuggestion.value = ''
}
/** 服务切换保留用户填写的公开设置，清理旧组件建议并重新填写秘密。 */
function changeService(): void {
  clearDeployedSuggestions()
  if (!name.value.trim() || Object.values(serviceNames).includes(name.value)) name.value = externalService.value ? serviceNames[externalService.value] : ''
  baseUrl.value = ''
  apiKey.value = ''
  clearConnection.value = false
  error.value = ''
  externalMode.value = 'connect'
}
/** 只预填已核验组件的公开连接信息，API 凭据仍由用户输入。 */
async function connectDeployed(component: CollectionComponent): Promise<void> {
  if (component.id !== externalService.value || !component.serviceUrl) return
  baseUrl.value = component.serviceUrl
  apiKey.value = ''
  clearConnection.value = false
  allowPrivate.value = true
  if (!externalUrl.value) {
    externalUrl.value = collectionComponentLocalEntry(component.setupPort)
    deployedEntrySuggestion.value = externalUrl.value
  }
  if (!rootId.value) {
    const root: CollectionCapabilities['roots'][number] | undefined = props.capabilities?.roots.find((entry: CollectionCapabilities['roots'][number]): boolean => entry.path.replace(/\\/g, '/').replace(/\/$/, '') === '/collection-media/library')
    if (root) {
      rootId.value = root.id
      path.value = component.id
      deployedRootSuggestion.value = root.id
      deployedPathSuggestion.value = component.id
    }
  }
  externalMode.value = 'connect'
  error.value = ''
  await nextTick()
  baseUrlInput.value?.focus()
}
/** 创建或乐观修订来源，地址仅通过提交写入。 */
async function save(): Promise<void> {
  if (saving.value) return
  if (usesAPI.value && externalMode.value === 'deploy') return
  if (usesDirectory.value && !usesAPI.value && !rootId.value) { error.value = '请先选择已配置的受控目录或连接。'; return }
  if (usesAPI.value && !clearConnection.value && !keepsConnection.value && (!baseUrl.value.trim() || !apiKey.value.trim())) { error.value = '请填写服务地址和已有 API 凭据。'; return }
  if (kind.value === 'rss' && !props.source && !url.value.trim()) { error.value = '请输入 RSS 来源地址。'; return }
  if (path.value.split(/[\\/]/).some((segment: string): boolean => segment === '..') || /^[\\/]|^[a-zA-Z]:/.test(path.value)) { error.value = '子目录必须使用根目录内的相对路径。'; return }
  saving.value = true
  error.value = ''
  const input: CollectionSourceInput = { kind: kind.value, name: name.value.trim(), enabled: enabled.value, rootId: rootId.value, path: path.value.trim(), allowPrivate: allowPrivate.value, refreshMinutes: refreshMinutes.value, outputProfile: outputProfile.value, externalUrl: externalUrl.value.trim(), externalService: kind.value === 'external' ? externalService.value : '' }
  // 固定漫画来源不继承其他类型表单中已隐藏的目录、链接或私网授权。
  if (kind.value === 'jmcomic') Object.assign(input, { rootId: '', path: '', allowPrivate: false, outputProfile: '', externalUrl: '' })
  if (usesAPI.value && (baseUrl.value.trim() || apiKey.value.trim()) && !clearConnection.value) input.connection = { baseUrl: baseUrl.value.trim(), apiKey: apiKey.value.trim() }
  if (kind.value === 'external' && props.source?.apiConfigured && (clearConnection.value || !externalService.value)) input.clearConnection = true
  if (url.value.trim()) input.url = url.value.trim()
  if (props.source) input.revision = props.source.revision
  try {
    const result = await api<{ source: CollectionSource }>(`/collection/sources${props.source ? `/${encodeURIComponent(props.source.id)}` : ''}`, props.source ? 'PATCH' : 'POST', input, controller.signal)
    if (controller.signal.aborted) return
    url.value = ''
    baseUrl.value = ''
    apiKey.value = ''
    console.info('资源来源配置已保存')
    emit('saved', result.source)
  } catch (failure: unknown) { if (!isAbort(failure)) error.value = errorMessage(failure) }
  finally { saving.value = false }
}
onBeforeUnmount((): void => { controller.abort(); url.value = ''; baseUrl.value = ''; apiKey.value = '' })
</script>

<template>
  <DialogShell :title="source ? '编辑收集来源' : '添加收集来源'" @close="emit('close')">
    <form class="collection-form" @submit.prevent="save">
      <p v-if="kind !== 'external'" class="muted">来源是你已经拥有的地址或连接。执行工具是否可用会单独检测。</p>
      <p v-else class="section-note">选择服务 → 连接或部署 → 保存并检测。下载仍由外部服务负责，完成目录可以稍后设置。</p>
      <div class="collection-field-grid">
        <label>来源类型<select v-model="kind" :disabled="!!source || saving" @change="changeKind"><option v-for="(label, key) in collectionKindLabels" :key="key" :value="key">{{ label }}</option></select><span class="field-hint">来源创建后类型固定；不同入口可各建一个来源。</span></label>
        <label>来源名称<input v-model="name" autofocus required maxlength="80" :disabled="saving" placeholder="例如：个人音乐目录" /><span class="field-hint">在候选和任务记录中显示的名称。</span></label>
      </div>
      <template v-if="kind === 'external'">
        <label>选择服务<select v-model="externalService" :disabled="saving" @change="changeService"><option v-for="(label, value) in serviceNames" :key="value" :value="value">{{ label }}</option></select><span class="field-hint">自动检测兼容版本，无需选择 API 版本。</span></label>
        <template v-if="usesAPI">
          <div class="collection-tabs source-external-mode" role="group" aria-label="组件接入方式"><button type="button" :class="{ active: externalMode === 'connect' }" :aria-pressed="externalMode === 'connect'" :disabled="saving" @click="externalMode = 'connect'">连接已有组件</button><button type="button" :class="{ active: externalMode === 'deploy' }" :aria-pressed="externalMode === 'deploy'" :disabled="saving" @click="externalMode = 'deploy'">在此部署组件</button></div>
          <CollectionComponentDeploy v-if="externalMode === 'deploy'" :key="externalService" :service="externalService" :disabled="saving" @connect="connectDeployed" />
        </template>
        <template v-if="usesAPI && externalMode === 'connect'">
          <label>服务地址<input ref="baseUrlInput" v-model="baseUrl" type="url" :required="!keepsConnection && !clearConnection" :disabled="saving || clearConnection" autocomplete="off" spellcheck="false" :placeholder="keepsConnection ? '留空保留已保存地址' : '例如 http://192.168.1.10:8989'" /><span class="field-hint">填写实例首页基址，可保留反向代理子路径，不需要追加 /api。地址保密，不会回显。</span></label>
          <!-- 切换服务时重建凭据框，即使旧值为空也恢复默认掩码。 -->
          <div class="collection-secret-field"><label for="source-api-key">{{ externalService === 'autobangumi' ? 'API Token' : 'API Key / Token' }}</label><PasswordInput :key="externalService" id="source-api-key" v-model="apiKey" secret-label="API 凭据" :required="!keepsConnection && !clearConnection" :disabled="saving || clearConnection" autocomplete="new-password" spellcheck="false" aria-describedby="source-api-key-hint" :placeholder="keepsConnection ? '已保存凭据，留空保留' : '粘贴外部服务中已生成的 API 凭据'" /><span id="source-api-key-hint" class="field-hint">{{ externalService === 'autobangumi' ? '使用 AutoBangumi 中 scope 为 api 的 Token。' : '在外部应用设置中获取 API 凭据。' }} 凭据仅写入，不在列表和编辑中显示。</span></div>
          <label class="collection-check"><input v-model="allowPrivate" type="checkbox" :disabled="saving" /><span>允许连接我的 NAS 私网服务<span class="field-hint">内网或本机地址需要明确勾选。保留地址、元数据地址和越界重定向仍会拒绝。</span></span></label>
        </template>
      </template>
      <label v-if="kind === 'rss'">RSS 地址<input v-model="url" type="url" :required="!source" :disabled="saving" autocomplete="off" spellcheck="false" :placeholder="source ? '留空保留已保存地址' : 'https://…'" /><span class="field-hint">地址只写入，不在读取和编辑中回显。{{ source?.hasSecret ? '已保存私密配置。' : '' }}</span></label>
      <template v-if="kind === 'rss'">
        <label>刷新间隔（分钟）<input v-model.number="refreshMinutes" type="number" min="5" max="1440" required :disabled="saving" /><span class="field-hint">控制启用订阅的轮询频率；首次收取建立基线，历史回填需单独操作。</span></label>
        <label class="collection-check"><input v-model="allowPrivate" type="checkbox" :disabled="saving" /><span>允许此来源访问私网<span class="field-hint">仅用于你明确管理的 NAS 私网 RSS 服务，其他来源仍受独立网络策略限制。</span></span></label>
      </template>
      <p v-if="kind === 'web'" class="section-note">网页来源使用固定获取工具。具体链接在“网页与文件”中逐条预览；首期不递归播放列表、不上传 Cookie。</p>
      <p v-if="kind === 'jmcomic'" class="section-note">使用 JMComic-Crawler-Python 固定插件搜索与下载漫画。保存后可按关键词或编号预览，再确认导入漫画库；不需要填写站点地址或账号。插件依赖和网络能力由独立获取执行端检测。</p>
      <label v-if="kind === 'web'">默认输出<select v-model="outputProfile" :disabled="saving"><option value="video">保存视频</option><option value="audio">提取音频</option></select><span class="field-hint">每次预览时仍可明确选择输出类型。</span></label>
      <label v-if="kind === 'local'">导入方式<select v-model="localMode" :disabled="saving" @change="changeMode"><option value="register">只读登记已有文件（默认）</option><option value="copy">归档副本至受管目录</option></select><span class="field-hint">只读登记直接扫描原目录；归档复制需要执行端的预配置本地目录。</span></label>
      <template v-if="usesDirectory && kind !== 'external'">
        <label>{{ register ? '已授权媒体根目录' : '执行端目录或连接' }}<select v-model="rootId" required :disabled="saving"><option value="" disabled>请选择预配置来源</option><template v-if="register"><option v-for="root in capabilities?.roots || []" :key="root.id" :value="root.id">{{ root.name }} · {{ root.path }}</option></template><template v-else><option v-for="entry in workerChoices" :key="entry.id" :value="entry.id">{{ entry.name }}</option></template></select><span class="field-hint">{{ register ? '仅可访问服务端已授权的媒体挂载。' : '连接凭据和实际根路径由执行端配置，不在网页输入。' }}</span></label>
        <p v-if="!capabilities || (!register && !capabilities.workerReady)" class="error-box">{{ capabilities?.workerReason || '目录能力尚未读取成功，请关闭后刷新能力。' }}</p>
        <label>来源子目录<input v-model="path" :disabled="saving" maxlength="512" placeholder="可留空，例如 Music/2026" /><span class="field-hint">相对于所选根目录的路径；留空使用根目录，不允许绝对路径或上级跳转。</span></label>
      </template>
      <template v-if="kind === 'external' && (!usesAPI || externalMode === 'connect')">
        <details class="collection-advanced" :open="!usesAPI">
          <summary>{{ usesAPI ? '高级设置：完成目录与外部入口（可稍后）' : '外部入口与成品目录' }}</summary>
          <div class="collection-advanced-fields">
            <label>完成目录在本系统的挂载<select v-model="rootId" :required="!usesAPI" :disabled="saving"><option value="">{{ usesAPI ? '稍后设置完成目录' : '请选择已授权媒体根目录' }}</option><option v-for="root in capabilities?.roots || []" :key="root.id" :value="root.id">{{ root.name }} · {{ root.path }}</option></select><span class="field-hint">只选择外部服务已整理完毕的专用目录。本系统只读登记，不读取下载中的临时目录。</span></label>
            <label>完成子目录<input v-model="path" :disabled="saving || !rootId" maxlength="512" placeholder="可留空，例如 Movies/Complete" /><span class="field-hint">相对于所选挂载的目录；配置后仍需预览并扫描入库。</span></label>
            <label>浏览器中的外部应用入口<input v-model="externalUrl" type="url" :required="!usesAPI" :disabled="saving" placeholder="可选，https://你的应用入口/" /><span class="field-hint">供“进入外部应用”使用，与私密服务地址分开保存；不能含账号、查询参数或片段。</span></label>
            <label v-if="source?.apiConfigured && usesAPI" class="collection-check"><input v-model="clearConnection" type="checkbox" :disabled="saving" /><span>断开已保存的 API 连接<span class="field-hint">保留公开入口、完成目录和历史记录；再次控制需要重新配置。</span></span></label>
            <label class="collection-check"><input v-model="enabled" type="checkbox" :disabled="saving" /><span>允许此来源创建新操作<span class="field-hint">停用仅限制本系统的新操作，不会停止外部服务或删除媒体。</span></span></label>
          </div>
        </details>
      </template>
      <label v-if="kind !== 'external'" class="collection-check"><input v-model="enabled" type="checkbox" :disabled="saving" /><span>启用来源<span class="field-hint">停用后不产生新的收集；历史任务与媒体保留。</span></span></label>
      <p v-if="error" class="error-box" role="alert">{{ error }}</p>
      <div class="dialog-actions"><button class="secondary-button" type="button" @click="emit('close')">{{ usesAPI && externalMode === 'deploy' ? '关闭' : '取消' }}</button><button v-if="!usesAPI || externalMode === 'connect'" class="primary-button" type="submit" :disabled="saving || !name.trim()"><Icon name="check" />{{ saving ? '保存中…' : usesAPI && !clearConnection ? '保存并检测' : '保存来源' }}</button></div>
    </form>
  </DialogShell>
</template>

<style scoped>
.collection-secret-field { display: flex; flex-direction: column; gap: 9px; min-width: 0; font-weight: 500; }
.source-external-mode { width: fit-content; max-width: 100%; }
.source-external-mode button { white-space: normal; }
@media (max-width: 600px) { .source-external-mode { width: 100%; } .source-external-mode button { flex: 1; padding: 10px; } }
</style>
