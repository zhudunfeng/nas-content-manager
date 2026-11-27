# S02.2 独立浏览器复核

日期：2026-11-23。平台：Windows AMD64，真实 Edge `158.0.4370.33`。本记录只覆盖在线账号安全 UI、其真实 HTTP 链路和浏览器 verifier 的证据边界，不声明 S02.3、Linux 或目标 NAS 已通过。

## 结论与已修复问题

独立复核发现并修复一个可稳定复现的密码显示状态问题：在“退出其他会话”表单的**空**密码框点击显示，再从另一表单提交错误原密码，原 `clearSecrets()` 仅重复赋值空字符串，不能触发 `PasswordInput` 的值变化 watcher；该空框保持 `type=text`，下次输入会继续明文。

- 复现证据：`.runtime/verify/s02-2-review-browser-empty-mask/result.json`，41 项已完成断言后在“错误原密码”阶段按预期失败，`failureKind=AssertionError`，`cleanupFailed=false`。真实请求返回 400；没有把预期失败改记为通过。
- 最小修复：只改 `web/src/components/AccountSecurity.vue`，`clearSecrets()` 推进局部 `secretReset`，四个 `PasswordInput` 各自使用带字段前缀的局部 key，确保每次敏感操作清理都恢复默认掩码。不改共享 `PasswordInput`、`DialogShell`、全站导航或样式。
- 稳定回归：`scripts/verify_account_browser.py:wrong_password()` 在真实 400 前主动显示另一表单的空框，之后核对四字段均为空且 `type=password`，并再次核对四个 label 的精确 ID 与新密码提示的 `aria-describedby`。

## 本轮 verifier 增强

- 对源构建与隔离服务目录的 `index.html`、全部 assets 记录相对路径、字节和 SHA256，并在复制后、浏览器结束后核对。不穿越 symlink/junction/reparse point；空资产不能通过。
- 资产断言使用独立 `artifact-identity` 分类，不增加真实浏览器或受控界面计数。结果记录 `frontend`、`frontendUnchanged`、`browserVersion` 和 `artifactAssertions`。
- 分页以实际 Cookie 摘要从隔离 SQLite 查出独立的当前公开 ID，再将数据库其他有效会话集合与两批 UI ID 的并集精确比较；当前固定项、每批上限、第二批真实 HTTP 200、批间无重复分别验证。
- 初始 setup、每次真实浏览器登录、分页夹具的 API 登录都立即记住 token 及摘要；改密前后 bcrypt hash 也分别采集。撤销会话后仍保留旧秘密，只在内存执行原值扫描。
- 任一 context 关闭错误后继续关闭其余自有 context；browser 关闭错误不会阻止回收隔离应用，且 `cleanupFailed=true` 必须使总结果失败。原始 Playwright 异常可能含输入值，只保存异常类型和固定阶段。

## 最终真实回归

执行命令：

```powershell
python -B scripts/project_env.py -- python -B scripts/verify_account_browser.py --binary .runtime/bin/s02-2-review/nas.exe --root .runtime/verify/s02-2-review-browser-final
```

结果根：`.runtime/verify/s02-2-review-browser-final/`。`result.json`：

| 项目 | 实际结果 |
| --- | --- |
| passed / cleanupFailed | true / false |
| 总断言 | 138 |
| realBrowserAssertions | 122 |
| presentationAssertions | 14 |
| artifactAssertions | 2 |
| frontendUnchanged | true |
| 二进制字节 | 21,334,016 |
| 二进制 SHA256 | `2d8c2ed03b85e9b4c1ddbcca87cda119df44f2f17cab726d509abcf056d3873a` |
| 前端清单 SHA256 | `3194b81659ed5e2a4bb045cea3460c243a6c362688c55387a14e2eee9759111a` |
| 截图 | 10 张，保存前已清空密码 |

真实覆盖包括两个隔离 context 分别登录、四字段独立显示、键盘切换不提交、真实分页、真实错误原密码 400、不误触全局退出、真实指定会话撤销、再认证退出其他、真实改密/全部旧 Cookie 失效、旧密码拒绝、新密码登录、当前会话撤销及 Cookie 清除。字段重建后标签/提示仍关联，离开再进入无遗留秘密，迟到读请求不能重写卸载后的页面，未知敏感写结果不自动重发。

受控 route 的空批继续、错误/限流/冲突/未知网络呈现单独记为 14 项，不冒充后端实际执行。375/768/1024/1440 与明暗、当前撤销 modal 正反 Tab、背景不能获焦、Escape 和焦点恢复均由实际浏览器执行。浏览器原生 Tab 可进入浏览器工具栏；仅在页面持有焦点时判断焦点是否位于 modal，不把 `document.hasFocus=false` 与 BODY 误判成背景页面可交互。

## 故障注入与清理

用 Python stdin 临时 patch 生产 verifier，未新增永久测试入口或保留临时测试源码。三种情况均实际启动隔离应用及 Edge；stdout/stderr 仅在内存收集，断言后只写安全结果。

| 根目录后缀（前缀 `.runtime/verify/s02-2-review-browser-`） | 注入 | 已验证结果 |
| --- | --- | --- |
| `failure-secret` | 两个真实页面登录后抛含随机密码的 Playwright Error | exit1、passed=false、failureKind 仅 Error；两 context 均关闭；app 已停止；原始异常密码不在输出及文本证据 |
| `cleanup-failure` | 页面真实登录完成，实际 browser.close 后抛含密码 Error | exit1、passed=false、cleanupFailed=true；app 仍回收；异常正文未持久化 |
| `context-cleanup-failure` | 首 context 实际关闭后抛含密码 Error | 第二 context 仍尝试并成功关闭；exit1、passed=false、cleanupFailed=true、failureKind=RuntimeError；app 已回收 |

各根保存 `fault-injection-review.json` 和 verifier 原始安全结果。故障根的 failed 是预期结果，不计入最终 138 项成功断言。关闭注入先执行真实关闭再抛错，用于确定性检查错误传播和后续清理，不声称模拟操作系统完全拒绝关闭进程。

## 截图目视与边界

已目视最终根的 375 light/dark、1440 dark 全页及账号区域原尺寸裁切，以及 375 light/1440 dark 两个当前撤销 modal。输入框、中文规则提示、按钮和会话卡在容器内；窄屏确认文案换行、取消/确认与关闭控件可读，没有账号区域横向溢出。键盘测试后可见的“跳转到主要内容”是既有 skip link 获焦状态，不是新增账号页面遮挡缺陷。裁切仅作目视辅助，文件名后缀 `-review-crop.png`，不作为额外 UI 断言计数。

浏览器原始控制台、local/sessionStorage、可见 Cookie 与 DTO 在持久脱敏前做已知密码/token/hash/原始 UA 检查；控制台超出采样预算会失败。保存的服务日志由公共夹具脱敏，不能据此倒推全部原始服务 stdout 无秘密。服务端原始业务日志另有本轮六个临时 Go 测试之一覆盖，详见 `quality-review-s02-2.md`。本轮没有操作用户实例、外部账号或 Docker，没有提交或部署。
