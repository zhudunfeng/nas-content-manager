# Bug Analysis: qB 认证探针与生产适配器不一致

## 1. Root Cause Category

- **Category**：B（跨层契约）、D（覆盖缺口）、E（隐含假设）。
- **Specific Cause**：最初只按旧版登录习惯检查 HTTP 200、`Ok.` 和固定 `SID`。真实 5.2.4 本机返回 HTTP 204、空正文与 `QBT_SID_<WebUI端口>`。独立 Python 协议探针使用 cookie jar 自动保存／回送，生产 Go 适配器手动处理名称，两条实现没有共享同一验证闭环。

## 2. Why Fixes Failed

1. 增加 HTTP 204／空正文兼容只修复了状态判定，尚未检查返回 Cookie 的完整名称；有效会话仍无法提取。
2. 仅识别新名称也不充分：后续请求若继续写死 `SID`，登录看似成功，实际鉴权调用仍失败。因此临测必须同时验证下一次请求的名称与值回送。

这些差异来自真实联调，而非放宽密码或允许无会话访问。错误密码仍拒绝；最终适配器继续要求有效且受限的会话名称。

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | 架构 | 将会话名称与值成对保存、回送；限定旧 `SID` 或合法端口后缀的新名称 | DONE |
| P0 | 行为回归 | 14 个认证子项覆盖状态／正文／空值／任意名称／非法端口／过期值与后续实际请求 | 执行通过，临测按用户要求清理 |
| P0 | 集成 | 保留真实 app→qB→本机 Peer→发布→读取的探针，不只运行独立客户端 | 真实闭环通过，新 Cookie 假引擎最终 36 项通过，脚本保留 |
| P1 | 规范 | backend download-contract 记录版本、会话名称、反代端口与脱敏边界 | DONE |
| P1 | 流程 | cross-layer-thinking-guide 要求真实生产客户端的完整认证往返与恢复验证 | DONE |

## 4. Systematic Expansion

- **Similar Issues**：元数据停止、完整 v2 身份、异步移动及分片边界同样不能只按 HTTP 成功推断行为，均已纳入分层证据。
- **Design Improvement**：保留受控适配器边界；浏览器不获知上游会话，失效认证不影响栖库管理员会话。反代端口可能与引擎端口不同，不能错误绑定两者。
- **Process Improvement**：可控假实现用于确定性故障与竞态，真实探针用于协议观察，真实应用联调用于两者交汇；三者不能互相替代。

## 5. Knowledge Capture

- [x] 更新 `.trellis/spec/backend/download-contract.md` 的认证往返与拒绝矩阵。
- [x] 更新 `.trellis/spec/guides/cross-layer-thinking-guide.md` 的外部服务边界检查。
- [x] 保留认证脱敏元信息与真实闭环脚本；不记录密码或会话值。
- [x] 检查模板同步位置：本业务仓库不存在 `src/templates/markdown/spec/`，无需创建 Trellis 工具自身的模板树。
- [x] 保留当前任务修复，无需另开根因工单。用户要求不自动提交，规范变更随功能一起留在工作区供审阅。
