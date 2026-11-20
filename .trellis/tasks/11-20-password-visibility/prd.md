# 密码输入框显示与隐藏切换

## Goal

为前端密码及已有掩码凭据输入框增加小眼睛按钮，默认隐藏，点击可显示或再次隐藏，保持现有界面风格和表单行为。

## Requirements

- 覆盖登录密码、初次创建管理员密码、确认密码，以及现有外部服务 API Key / Token 掩码输入框，采用共享的轻量组件。
- 每个字段初始 type=password，独立切换为 text，再次点击恢复隐藏；切换不修改内容、不触发表单提交、不发送接口请求。
- 保留现有 v-model、required、长度、autocomplete、placeholder、aria-describedby、禁用状态、密码策略和中文提示。
- 切换按钮使用项目本地 SVG 眼睛图标，明确的中文可访问名称/提示与状态；原生 button type=button、键盘可用、可见焦点，触摸目标至少44px。
- 保持桌面/移动端、明暗主题的输入框和错误提示布局，预留按钮区域避免遮挡文本；禁用输入框时切换按钮也禁用。
- 新建字段、清空凭据及切换外部服务后恢复隐藏，包括原值为空时切换服务；不记录或持久化密码明文/可见状态。
- 沿用中文方法/字段/配置注释与显式类型；不为纯本地显示切换增加业务日志，避免暴露凭据。

## Acceptance Criteria

- [x] 登录、创建账户、确认及外部凭据默认隐藏，显示/隐藏可反复独立切换，输入值保持一致。
- [x] 点击/键盘切换不提交表单或发出业务写请求，disabled 和可访问名称、焦点正确。
- [x] 原有自动填充属性、密码校验和提示关联保持；清空、重新打开及切换外部服务后恢复隐藏。
- [x] 375/768/1024/1440宽度、明暗主题无横向溢出或遮挡，图标及按钮尺寸符合现有样式。
- [x] 前端类型检查、构建和独立质量复核通过，临时测试源码清除，规范记录组件契约。

## Technical Approach

复用现有 Icon.vue 与语义 CSS 变量，新增 PasswordInput.vue 封装本地可见状态及输入属性转发，替换现有 type=password 字段。不引入依赖。用独立浏览器上下文/隔离样本检查真实 Vue 组件，不向用户数据库提交 setup/login 或外部服务写请求。

## Definition of Done

完成 Trellis implement → check → spec update，记录实际浏览器检查和构建证据；保留先前修复、用户服务与业务数据，不提交或归档。

## Out of Scope

不增加密码强度、密码生成/找回、认证接口、密码策略或凭据存储改动。

## Technical Notes

- 已定位：web/src/App.vue（登录/创建/确认），web/src/components/CollectionSourceDialog.vue（API凭据），web/src/components/Icon.vue，web/src/style.css。
- 适用规范：.trellis/spec/frontend/app-contract.md、.trellis/spec/frontend/index.md、docs/api.md、.trellis/spec/guides/code-reuse-thinking-guide.md。
- 构建与临时工具通过 python -B scripts/project_env.py -- 执行，浏览器进程启动前继承同一环境，所有缓存和生成的验证材料限定在.runtime/。
- 需求由本轮用户明确授权，交互细节沿用项目规范，无阻塞问题。

## Completion Evidence

- 真实Vue界面24组合、独立登录抽样、服务切换8组合及最终间距8组合通过；最终类型检查、Vite构建、差异检查与Trellis上下文校验通过。
- 实现、浏览器与独立质量记录位于research/；规范已更新，验收概要见docs/verification.md。临时测试源码已删除，仅保留.runtime中的JSON/截图证据。
- API全部由隔离浏览器夹具拦截，没有向用户后端提交认证或来源写请求；未实测其他浏览器、物理移动设备或密码管理器自动填充。
