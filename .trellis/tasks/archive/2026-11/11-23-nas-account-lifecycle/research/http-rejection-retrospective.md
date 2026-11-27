# S02 集成回归：Windows 未读小正文导致响应被连接重置

日期：2026-11-23。此记录解释本机真实回归发现的存量问题；不能据此推断 Linux/NAS 结果。

## 1. 根因分类

- B 跨层契约 / E 隐含假设：业务在读取正文前返回403/401/202，假设客户端一定能收到该响应。Python urllib 的关闭连接请求把头和正文分开发送；Windows 上未读正文与连接关闭竞态可使客户端只收到10054。机制同时涉及成功与拒绝响应。
- 本机 Go 官方源码 `net/http/server.go` 的关闭连接分支不会使用普通keep-alive的正文消费路径；`transfer.go:body.Close` 也明确跳过即将关闭且无trailer的未读正文。`server.go` 的RST规避说明提到，客户端写入失败可能连同已到达的响应一起丢失。
- 这是HTTP传输边界，不是账号CAS/迁移失败，也不是应用进程崩溃。原core失败后manifest服务exit1来自验证器finally回收，不能据此宣称服务自退。

## 2. 原始证据与对照

- `s02-1-core-1` 保留原失败：前三断言后跨站POST发生10054，不能算403通过。
- `s02-1-origin-diagnostic-1`：新片20次相同跨站POST，12次10054、8次403；每次进程仍存活且health200。
- `s02-1-origin-old-artifact-1`：S01 schema4旧产物20次，1次10054、19次403；进程和health正常，确认存量存在。
- `s02-1-origin-fixed-1`：局部修复产物20次全部403，进程和health正常。
- `s02-1-unauth-diagnostic-1`：同源匿名POST在尚未补auth早拒绝时20次中17次401、3次10054，health正常；据此把同一小正文处理复用于auth分支。
- `s02-1-core-2`：仅覆盖错误分支的中间产物曾通过52条；不能据此证明同类问题完整解决。最新后续`s02-1-core-3`仍在音乐重扫`POST /libraries/{id}/scan {}`成功202处10054，服务端扫描已开始且完成，确认成功但忽略正文也会触发。保留该失败。
- 上述目录均在 `.runtime/verify/`，属于本地证据，不上传含私密夹具的整个根。最终完整回归由implementation及quality-review另记，不能用定向诊断代替。

## 3. 修复与防回归

- 最终改为security中统一defer一次`discardUnreadSmallBody`，同时覆盖成功和拒绝。它只丢弃已知非空且不超过64KiB正文的未读部分，使用一秒局部读取deadline，不解析、不保留或记录正文。
- 不需要计数ReadCloser或ResponseWriter包装：HTTP原始正文已经保留消费位置，`io.CopyN`以上次原ContentLength为最大读取量，普通EOF表示原正文已经读完。若把已部分/全部消费后的EOF当成损坏，会误关闭正常连接；UnexpectedEOF等实际传输错误才标记close。保留原生Close的并发中断语义。
- 大正文、未知长度、Expect请求不在helper内等待，不触发100-continue；小正文不完整时关闭连接。deadline不可用则不执行可能无界的读取。
- 不放松同源鉴定、不把TCP错误转换成预期状态、不新增全局媒体写超时、不无限制drain攻击者输入。
- 中间方案5个临时Go测试实际通过，日志保留`.runtime/verify/s02-1-http-rejection-go.log`，只证明中间错误分支边界。
- 最终方案8个顶层临时Go测试实际通过：未读/部分/全部消费；JSON原值保留且不误关连接；大小/未知/Expect/deadline不支持跳过；0与64KiB边界；真实TCP大/分块/Expect不等待且无100；截断关闭；分开发送的202/403/200各20次；一秒慢发送仍交付响应。日志`.runtime/verify/s02-1-http-finish-go.log`。新临时单测源运行后已删除。
- 补充原生keep-alive生命周期2个临时测试通过：失败时不清零deadline而设为now，避免net/http后续Body.Close重新等待；实际慢体在约1.51秒收到202并观察到Connection关闭，包含Go原生约0.5秒关闭保护。正常JSON在同一连接连续3次200，原值保持且无误关。日志`.runtime/verify/s02-1-http-keepalive-go.log`，新增测试源已删除。只有成功收尾才恢复读取预算。
- 稳定HTTP回归保留精确成功/错误状态与服务存活断言；最终全链路结果等待最后一轮集成和独立复核，不把中间产物通过值冒充最终值。维护规范已同步到backend/runtime-contract。

## 4. 调试过程与扩展范围

先保留失败并判断进程是否仍活着，再用旧产物对照，最后在新产物重复相同请求验证；没有采用重复重跑直至碰巧通过的做法。第一次只修跨站、第二次扩至匿名及限流都遗漏成功分支，说明以状态码枚举修复不足以覆盖同一传输契约，最终收敛为统一收尾。临时测试补齐原值/已读完、慢发送与Expect边界，避免为修复小请求引入错误连接关闭或新的默认30秒等待。

无需修改脚手架模板：仓库没有 `src/templates/markdown/spec/` 镜像。按本任务约定不自动提交或部署。
