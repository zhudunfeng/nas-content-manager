/**
 * 在已启动的隔离栖库页面上验证真实界面；引擎路由由受控 fixture 代替，不代表真实 Docker 验收。
 * @param {import('playwright').Page} page 独立本地验证页面。
 * @returns {Promise<object>} 不含账号密码的断言结果。
 */
async (page) => {
  const checks = []
  const failures = []
  /** 记录断言并在首次行为错误时停止，避免继续产生误导证据。
   * @param {boolean} condition 断言条件。
   * @param {string} label 中文证据说明。
   * @returns {void} 无返回值。
   */
  function check(condition, label) { if (!condition) { failures.push(label); throw new Error(label) } checks.push(label) }
  const base = new URL(page.url()).origin
  check(/^http:\/\/127\.0\.0\.1:\d+$/.test(base), '仅允许本地隔离应用')
  if (await page.getByRole('dialog').count()) await page.keyboard.press('Escape')
  const stamp = new Date().toISOString()
  // 只拦截引擎控制面，其余认证、页面、下载历史、导航继续使用真实应用。
  const fixture = {
    scenario: 'missing', // 可切换的受控部署状态。
    activeTaskId: '', // 服务端持久活动任务编号。
    initializations: [], // 用于核对相同请求重试的安全参数。
    retries: [], // 用于核对乐观修订和幂等键。
    rejectFirstInitialization: true, // 第一次返回受控服务故障以验证幂等重试。
    rejectFirstRetry: true, // 第一次重试返回受控服务故障。
    posts: 0, // 页面读取不应触发部署操作。
    tasks: {}, // 有限任务状态，只存受控事件。
  }
  /** 创建不含私密字段的组件状态。
   * @param {string} id 固定组件编号。
   * @returns {object} 安全组件 DTO。
   */
  function component(id) {
    const result = { id, mode: 'managed', presence: 'missing', runtime: 'unknown', readiness: 'not_configured', message: '已确认本安装不存在此组件，可按需初始化。', revision: 1, canInitialize: true, canStart: false }
    if (fixture.scenario === 'unavailable') Object.assign(result, { presence: 'unknown', readiness: 'error', message: '部署控制面暂不可用，请重试检测。', canInitialize: false })
    if (fixture.scenario === 'conflict') Object.assign(result, { presence: 'conflict', readiness: 'error', message: '发现归属不符的配置，未修改现有内容。', canInitialize: false })
    if (fixture.scenario === 'external') Object.assign(result, { mode: 'external', presence: 'unknown', runtime: 'offline', readiness: 'auth_failed', message: '外部实例认证失败，请修正服务端配置。', canInitialize: false })
    if (fixture.scenario === 'running') Object.assign(result, { presence: 'present', runtime: 'running', readiness: id === 'jackett' ? 'configuration_required' : 'ready', message: id === 'jackett' ? '已部署，等待配置来源。' : '生产适配器连接已确认。', canInitialize: false })
    if (fixture.scenario === 'stopped') Object.assign(result, { presence: 'present', runtime: 'stopped', readiness: 'not_configured', canInitialize: false, canStart: true })
    if (fixture.activeTaskId) Object.assign(result, { readiness: 'initializing', canInitialize: false })
    return result
  }
  /** 构造只读安全检测快照。
   * @returns {object} 状态 DTO。
   */
  function status() { return { helper: fixture.scenario === 'unavailable' ? 'unavailable' : 'ready', message: '受控界面验收状态，不执行 Docker 操作。', components: [component('qbittorrent'), component('jackett')], activeTaskId: fixture.activeTaskId, noticeVersion: 'qbittorrent-5.2.4-v1', noticeText: 'qBittorrent 首次使用告知。请确认已获得下载和分享所需授权。', setupPort: 19117 } }
  /** 构造有限任务和真实分子分母的模拟镜像层，不伪装成实际部署结果。
   * @param {string} id 任务编号。
   * @param {string} state 任务状态。
   * @param {number} sequence 单任务事件编号。
   * @param {string} message 受控中文事件。
   * @returns {object} 任务响应。
   */
  function makeTask(id, state, sequence, message) {
    return { task: { id, action: 'initialize', state, revision: 2, components: ['qbittorrent', 'jackett'], steps: [{ component: 'qbittorrent', phase: state === 'completed' ? 'completed' : 'pulling', state: state === 'completed' ? 'completed' : state, attempt: 1, message: '受控镜像拉取进度。', layers: [{ id: 'abcd1234', status: '下载中', current: 5242880, total: 10485760 }, { id: 'ffff0000', status: '等待处理', current: 0, total: 0 }] }, { component: 'jackett', phase: 'completed', state: 'completed', attempt: 1, message: '搜索引擎部署步骤已完成。', layers: [] }], createdAt: stamp, updatedAt: stamp, message: '此为受控 UI 状态机。' }, events: [{ sequence, component: 'qbittorrent', level: 'info', message, createdAt: stamp }], nextSequence: sequence, reloadRequired: false }
  }
  fixture.tasks['ui-old-task'] = makeTask('ui-old-task', 'completed', 42, '旧任务独占事件，不应出现在新任务')
  await page.route('**/api/engines**', async (route) => {
    const request = route.request()
    const url = new URL(request.url())
    if (request.method() === 'POST') fixture.posts++
    if (url.pathname === '/api/engines' || url.pathname === '/api/engines/probe') return route.fulfill({ json: status() })
    if (url.pathname === '/api/engines/initializations' && request.method() === 'POST') {
      fixture.initializations.push(request.postDataJSON())
      if (fixture.rejectFirstInitialization) { fixture.rejectFirstInitialization = false; return route.fulfill({ status: 503, json: { error: '受控网络故障，请保留选择重试。' } }) }
      fixture.tasks['ui-new-task'] = makeTask('ui-new-task', 'running', 1, '新任务独占事件')
      fixture.activeTaskId = 'ui-new-task'
      return route.fulfill({ status: 202, json: fixture.tasks['ui-new-task'] })
    }
    if (url.pathname.endsWith('/retry')) {
      fixture.retries.push(request.postDataJSON())
      if (fixture.rejectFirstRetry) { fixture.rejectFirstRetry = false; return route.fulfill({ status: 503, json: { error: '受控重试等待故障，请再次重试。' } }) }
      fixture.tasks['ui-new-task'].task.state = 'running'
      fixture.tasks['ui-new-task'].task.revision = 3
      fixture.activeTaskId = 'ui-new-task'
      return route.fulfill({ status: 202, json: fixture.tasks['ui-new-task'] })
    }
    if (url.pathname === '/api/engines/search/sync-sources') {
      const body = request.postDataJSON()
      const candidates = Array.from({ length: 10 }, (_, index) => ({ id: `source${index + 1}`, name: `界面验收来源 ${index + 1}` }))
      return route.fulfill({ json: { candidates, selectedIds: body.sourceIds || [], selectionRequired: !body.sourceIds, state: body.sourceIds ? 'ready' : 'configuration_required', message: body.sourceIds ? '已接入选择的来源。' : '请明确选择最多八个来源。' } })
    }
    if (request.method() === 'GET' && url.pathname.startsWith('/api/engines/initializations/')) {
      const id = url.pathname.split('/').pop()
      const response = structuredClone(fixture.tasks[id])
      if (!response) return route.fulfill({ status: 404, json: { error: '任务不存在' } })
      response.events = response.events.filter((event) => event.sequence > Number(url.searchParams.get('afterSequence') || 0))
      return route.fulfill({ json: response })
    }
    return route.fulfill({ status: 400, json: { error: '受控路由没有此操作' } })
  })
  page.__engineFixture = fixture
  page.__engineUiChecks = checks
  await page.goto(`${base}/#downloads`)
  await page.evaluate(() => { sessionStorage.setItem('nas-engine-observed-task', 'ui-old-task'); sessionStorage.removeItem('nas-engine-pending-command') })
  await page.getByRole('button', { name: '引擎管理', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: '引擎管理', exact: true })
  await dialog.getByText('旧任务独占事件，不应出现在新任务', { exact: true }).waitFor({ state: 'attached' })
  check(fixture.posts === 0, 'GET 状态与打开面板不触发任何 POST 部署')
  const initialize = dialog.getByRole('button', { name: '初始化缺失引擎', exact: true })
  const notice = dialog.getByRole('checkbox', { name: '我已阅读并接受上述首次使用告知', exact: true })
  check(!await notice.isChecked() && !await initialize.isEnabled(), '首次告知未预选且阻止 qB 初始化')
  await dialog.getByRole('checkbox', { name: /^下载引擎/ }).uncheck()
  check(await dialog.getByRole('button', { name: '初始化所选引擎', exact: true }).isEnabled(), '只选 Jackett 时无需 qB 告知')
  await dialog.getByRole('checkbox', { name: /^下载引擎/ }).check()
  await notice.check()
  // 使用生产构建渲染八组尺寸与主题，并观察实际 DOM 是否横向溢出。
  const matrix = []
  page.__engineUiMatrix = matrix
  for (const theme of ['light', 'dark']) {
    for (const width of [375, 768, 1024, 1440]) {
      await page.setViewportSize({ width, height: 900 })
      await page.evaluate((value) => { document.documentElement.dataset.theme = value }, theme)
      const dimensions = await page.evaluate(() => {
        const modal = document.querySelector('dialog[open]')
        return { body: document.documentElement.scrollWidth, viewport: innerWidth, modal: modal.scrollWidth, modalClient: modal.clientWidth, mobileEntries: document.querySelectorAll('.bottom-nav button').length }
      })
      check(dimensions.body <= width + 1 && dimensions.modal <= dimensions.modalClient + 1, `${theme} ${width}px 页面与模态没有横向溢出`)
      matrix.push({ theme, width, ...dimensions })
      await dialog.screenshot({ path: `E:/vscodeProjects/nas-content-manager/.runtime/engine-ui/${theme}-${width}.png` })
    }
  }
  // 原生对话框允许 Tab 访问浏览器工具栏（DOM 表示为 body），但不能访问被 inert 的背景按钮。
  for (let index = 0; index < 14; index++) { await page.keyboard.press('Tab'); check(await page.evaluate(() => document.activeElement === document.body || document.activeElement?.closest('dialog[open]') !== null), 'Tab 不会落到原生模态背后的应用控件') }
  await initialize.click()
  await dialog.getByText('受控网络故障，请保留选择重试。', { exact: false }).waitFor()
  check(await notice.isChecked(), '提交失败后保留用户确认与选项')
  await initialize.click()
  await dialog.getByText('新任务独占事件', { exact: true }).waitFor({ state: 'attached' })
  check(fixture.initializations.length === 2 && fixture.initializations[0].requestId === fixture.initializations[1].requestId, '初始化网络失败重试沿用 requestId')
  check(fixture.initializations[1].acceptNotice === true && fixture.initializations[1].noticeVersion === 'qbittorrent-5.2.4-v1', '仅提交用户确认过的固定版本告知')
  check(await dialog.getByText('旧任务独占事件，不应出现在新任务', { exact: true }).count() === 0, '新任务原子清除旧任务事件与游标')
  check(await dialog.locator('progress').count() === 1 && await dialog.locator('progress').getAttribute('value') === '50', '仅可信分母的镜像层显示真实进度，无整体百分比')
  check(await dialog.locator('.status-pill').first().innerText() === '正在初始化', '初始化状态优先于尚未安装提示')
  await page.keyboard.press('Escape')
  check(await page.getByRole('dialog').count() === 0 && await page.evaluate(() => document.activeElement?.textContent?.trim()) === '引擎管理', 'Esc 关闭后恢复入口焦点')
  await page.reload()
  await page.getByRole('button', { name: '引擎管理', exact: true }).click()
  await dialog.getByText('新任务独占事件', { exact: true }).waitFor({ state: 'attached' })
  check(await dialog.getByRole('heading', { name: '初始化进行中', exact: true }).count() === 1, '页面刷新后恢复同一个活动持久任务')
  fixture.activeTaskId = ''
  fixture.tasks['ui-new-task'].task.state = 'failed'
  fixture.tasks['ui-new-task'].task.steps[0].state = 'failed'
  await dialog.getByRole('button', { name: '检测连接', exact: true }).click()
  const retry = dialog.getByRole('button', { name: '重试失败部分', exact: true })
  await retry.waitFor()
  await retry.click()
  await dialog.getByText('受控重试等待故障，请再次重试。', { exact: false }).waitFor()
  await retry.click()
  await dialog.getByRole('heading', { name: '初始化进行中', exact: true }).waitFor()
  check(fixture.retries.length === 2 && fixture.retries[0].requestId === fixture.retries[1].requestId && fixture.retries[0].revision === fixture.retries[1].revision, '失败部分重试保留原 requestId 与 revision')
  fixture.activeTaskId = ''
  fixture.tasks['ui-new-task'].task.state = 'completed'
  fixture.scenario = 'running'
  await dialog.getByRole('button', { name: '检测连接', exact: true }).click()
  await dialog.getByRole('button', { name: '同步来源', exact: true }).click()
  await dialog.getByRole('heading', { name: '已发现 10 个来源，请选择最多 8 个', exact: true }).waitFor()
  for (let index = 1; index <= 8; index++) await dialog.getByRole('checkbox', { name: `界面验收来源 ${index}`, exact: true }).check()
  check(!await dialog.getByRole('checkbox', { name: '界面验收来源 9', exact: true }).isEnabled(), '超过八个候选时明确选择并限制第九个')
  await dialog.getByRole('button', { name: '接入 8 个来源', exact: true }).click()
  await dialog.getByText('已接入选择的来源。', { exact: false }).waitFor()
  check(await dialog.getByRole('heading', { name: '已发现 10 个来源，请选择最多 8 个', exact: true }).count() === 0, '来源确认成功后退出选择面板')
  for (const scenario of ['unavailable', 'conflict', 'external', 'stopped']) {
    fixture.scenario = scenario
    await dialog.getByRole('button', { name: '检测连接', exact: true }).click()
    check(await dialog.getByRole('button', { name: /初始化缺失引擎|初始化所选引擎/ }).count() === 0, `${scenario} 不提供缺失初始化按钮`)
  }
  check(await dialog.getByRole('button', { name: '启动原引擎', exact: true }).count() === 2, '已停止的受管组件只提供启动原实例')
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: '搜种', exact: true }).click()
  await page.getByRole('dialog', { name: '搜种', exact: true }).getByRole('button', { name: '管理搜索引擎', exact: true }).click()
  await dialog.waitFor()
  check(await page.getByRole('dialog').count() === 1, '搜种未配置入口先关闭旧模态再打开引擎管理')
  await page.keyboard.press('Escape')
  await page.setViewportSize({ width: 375, height: 900 })
  const mobileCount = await page.locator('.bottom-nav button').count()
  check(mobileCount === 5, '375px 移动底栏保持五项')
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.getByRole('button', { name: '引擎管理', exact: true }).click()
  const reduced = await dialog.evaluate((element) => getComputedStyle(element.querySelector('button')).transitionDuration)
  check(reduced === '1e-05s' || reduced === '0.00001s', '遵循减少动态效果偏好')
  const result = { verifiedAt: new Date().toISOString(), scope: '真实本地应用认证与生产前端；引擎状态和操作为受控fixture，未执行Docker', checks, matrix, failures }
  page.__engineUiResult = result
  await page.keyboard.press('Escape')
  return result
}
