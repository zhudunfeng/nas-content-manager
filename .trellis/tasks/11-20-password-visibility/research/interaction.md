# 密码显示交互决策

## 已核实上下文

- Vue3、严格TypeScript、现有本地Icon.vue，页面使用语义CSS变量和统一44px控件，不需要新依赖。
- App.vue有登录/初次创建的密码和确认字段；CollectionSourceDialog.vue有API Key/Token掩码字段，可共用PasswordInput。
- 检索现有组件与密码逻辑未发现可复用的显示切换控件；无可用CodeGraph工具/CLI，回退到rg及直接读取明确文件。

## 交互规范依据

已使用 ui-ux-pro-max 的定向本地检索：

`python -B scripts/project_env.py -- python -B D:/soft_workspace/.codex/skills/ui-ux-pro-max/scripts/search.py "aria labels" --domain ux -n 1`

命中Accessibility / ARIA Labels：图标按钮必须具备可访问名称，避免无名称的纯图标交互。结合技能触摸目标44px、可见键盘焦点及项目原有控件规范实现。

## 选择

新增一个小型共享Vue组件，只管理本地显示状态并转发原生input属性。默认隐藏，按钮切换不提交。保持模型及验证原样，凭据清空与卸载后重置状态；disabled同时作用于按钮。分别提供密码、确认密码、API凭据的清晰中文名称，避免按钮文字改变输入框label含义。

## 验证方式

在真实Vite提供的源码上运行本机已安装Python Playwright及Edge，启动前经project_env注入仓库.runtime环境。浏览器为独立上下文，route fixture覆盖setup/login状态及拦截所有API，避免用户数据变更；外部凭据使用真实CollectionSourceDialog挂载。覆盖四种宽度和两种主题，额外验证默认隐藏、独立切换、输入值、清空重置、键盘、disabled、hint关联与零意外提交。

browser-automation技能引用的tool-index.md在技能根不存在；已直接验证Python playwright模块及Edge可执行文件存在，无需安装或更改全局配置。仅使用已有工具，测试源码执行后删除，截图与JSON证据保留在.runtime/verify/password-visibility/。
