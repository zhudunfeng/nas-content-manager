# 密码显示切换独立质量复核

## Findings (fixed)

- File: `web/src/components/CollectionSourceDialog.vue`
- Issue: 凭据本来为空时，先点击显示、再切换外部服务，`changeService()` 将模型再次赋为空字符串，无法触发值监听；Vue 复用组件导致新服务仍以 `type=text` 开始，不符合新字段默认隐藏的契约。
- Fix: 独立复核添加 `:key="externalService"`，并增加中文注释解释重挂载理由。换服务时创建独立的默认隐藏输入框，不改变现有清空逻辑、标签、校验或接口数据。

- File: `web/src/App.vue`
- Issue: 主会话查看375px创建账户截图发现，多行密码规则末尾紧贴确认密码标签。
- Fix: 由实现者增加局部 `.auth-password-field + .field-hint { margin: -10px 0 22px; }` 和中文样式说明；独立复核确认仅影响相邻规则提示。最终375px浅色截图中的密码提示、确认标签和眼睛按钮均无重叠。

## Findings (not fixed)

无未修复问题。未发现其他具体行为或规范违例。

## Verification

- Lint: 项目 `web/package.json` 未配置独立lint命令；静态规范复核及最终 `git diff --check` 通过，仅有仓库既有行尾转换通知。
- TypeCheck: 通过。实现者在两项修复均落盘后执行 `python -B scripts/project_env.py -- npm --prefix web run build`，内含 `vue-tsc --noEmit`；生产Vite构建通过（175模块），证据见 `research/implementation.md`。
- Tests: 通过。主会话24组真实Vue界面场景、独立登录抽样、独立外部服务切换8组合和提示间距最终8组合均成功，详见下述证据。

### 静态及规范核对

- 已读取当前任务PRD、check.jsonl及其引用规范，检查共享组件、App认证字段、来源弹窗、原有认证策略与邻近样式的实际代码路径。
- `PasswordInput` 的字符串模型、独立可见状态、清空同步重置、原生属性/事件转发、44px按钮与输入预留区域符合契约；`inheritAttrs:false`、必填模型和默认配置均有中文说明。
- 登录与创建账户仍沿用原有验证及提交路径，未新增持久化、明文日志或业务请求。显式 `for/id` 将输入标签与按钮分开，原生 `type=button`、中文名称、标题、`aria-pressed`、disabled与焦点规则完整。
- 当前前端契约已记录组件签名、掩码/清空/重挂载语义、可访问性及浏览器检查要求；规范与最终实现一致。此前PowerShell和Vite修复保持原有改动。

### 浏览器证据

全部浏览器由仓库 `project_env.py` 包装启动，使用已安装的Playwright和Edge、独立浏览器上下文及API拦截；测试页面加载真实Vite/Vue组件。以下为界面与受控API夹具证据，不代表真实用户账户认证或外部服务写入成功。

- `.runtime/verify/password-visibility/result.json`：主会话24组合通过，涵盖登录、创建/确认及真实外部来源弹窗与共享组件，375/768/1024/1440宽度和明暗主题。覆盖独立反复切换、值/原生属性、标签与提示、44px、焦点/键盘、disabled、清空/重挂载、无溢出及保留原提交数据/创建密码策略；全部零意外写请求。有意提交仅由夹具拦截。
- `.runtime/verify/password-visibility/independent-login-result.json`：独立375px登录抽样通过，核对默认掩码、required/maxlength/autocomplete、精确标签、标签外按钮、44px、Space/Enter、值保持、清空恢复及无横向溢出；业务写请求为零。
- `.runtime/verify/password-visibility/service-reset-result.json`：独立8组合通过，涵盖四宽度和明暗主题。空值显示后切换到Sonarr、非空值显示后切换到AutoBangumi均恢复空值和掩码/`aria-pressed=false`，新服务精确标签及hint关联正确；无横向溢出，每组零意外写请求、零pageerror。
- `.runtime/verify/password-visibility/spacing-result.json`：最终创建账户提示8组合通过，提示至确认标签间距均为22px。独立查看 `setup-final-375-light.png` 与 `external-375-dark.png`，文字、图标及字段间距正常；关键布局截图由主会话保留在同目录。

### 清理与边界

- 本次独立服务切换验证通过内联Python执行，未创建临时测试源码；目录检索确认本轮证据目录无遗留 `.py/.js/.ts/.vue/.html` 测试源，仅保留JSON与截图。主会话已删除其临时夹具源码。
- 未向真实后端提交登录、setup或来源写请求；未停止用户服务、修改用户凭据或访问真实媒体。未提交或归档任务。
- 实际移动设备、其他浏览器及密码管理器自动填充未作本轮实机验证；现有证据覆盖Edge真实DOM行为与规定视口，不将其表述为跨设备认证成功。
