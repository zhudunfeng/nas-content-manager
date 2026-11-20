# 密码输入框浏览器验收

## 执行环境与隔离

以 `python -B scripts/project_env.py -- python -B .runtime/verify/password-visibility/check.py` 启动本机Python Playwright/Edge。浏览器启动前继承项目.runtime环境，所有页面使用独立context；复用既有5173 Vite的真实Vue模块，不启停用户服务。

全部API在浏览器route中拦截：状态样本分别提供login/setup模式，主动提交仅比对内存中的合成输入后返回样本错误，没有访问用户后端或创建账户、登录、保存外部来源。共享组件及外部弹窗样本直接挂载真实PasswordInput和CollectionSourceDialog，不是复制实现。

## 已通过矩阵

`.runtime/verify/password-visibility/result.json`：passed=true，共24场景（login、setup含确认密码、外部/共享控件 × 375/768/1024/1440 × light/dark）。

- 默认隐藏、反复显示/隐藏保持输入，setup中主密码显示时确认密码仍隐藏，操作确认字段不改变主密码状态。
- 实际键盘Tab聚焦按钮，焦点轮廓可见，Enter/Space切换；切换没有任何提交请求。
- 清空模型立即恢复隐藏，重挂载后保留既有模型但显示状态恢复隐藏，disabled同时禁用输入与按钮。
- 原生required、maxlength、autocomplete和精确label/hint关联保持。setup弱密码被原策略拦截；有效setup样本和原中文login样本原样到达浏览器拦截层。
- 各宽度/主题无横向溢出，按钮44×44px，输入右侧预留56px，按钮名称、title、aria-pressed完整；无pageerror。
- 375和1440的两主题认证/外部弹窗截图均保存并抽样目视检查。发现setup hint末尾与确认label过近，已修正相邻提示间距并完成下述8组合复验。

## 最终小范围复验

- `.runtime/verify/password-visibility/spacing-result.json`：创建账户4宽度×2主题共8组合通过，密码规则至确认密码标签间距全部为22px，默认掩码且无横向溢出。最终375浅色、1440深色截图经目视检查，眼睛按钮和字段间距正常；最终截图命名为 `setup-final-*.png`。
- `.runtime/verify/password-visibility/service-reset-result.json`：真实外部来源弹窗4宽度×2主题共8组合通过；空值显示后切换Sonarr、非空值显示后切换AutoBangumi，均清空并恢复掩码和 `aria-pressed=false`，标签及提示关联正确；无横向溢出、业务写请求或页面脚本错误。
- 包含最终间距与外部服务重挂载修正的 wrapped `npm --prefix web run build` 通过，含严格Vue类型检查和Vite生产构建（175模块）。

## 独立核验

复核者以独立浏览器完成375px真实login DOM抽样：输入属性、唯一label、按钮在label外、44px、键盘切换、值保持、清空重置与无溢出通过；0写请求。结果为 `.runtime/verify/password-visibility/independent-login-result.json`。

## 样本修正记录

前两轮运行的login/setup页面验证通过，共享组件HTML样本先后暴露验证搭建问题：未声明UTF-8导致中文样本label乱码，以及使用不带Vite版本query的Vue导入造成独立runtime。已声明charset，并从实际转换后的PasswordInput模块读取相同Vue依赖URL，最终24项全通过。未因此修改产品逻辑，不把样本问题记为产品故障。

## 清理

完整矩阵成功后临时check.py已自动删除；最终间距复验的spacing.py同样成功后自动删除，外部服务复验使用inline Python没有落盘测试源码。浏览器/context在finally关闭。保留脱敏JSON和截图，不保存样本凭据内容或新建长期单元测试类。
