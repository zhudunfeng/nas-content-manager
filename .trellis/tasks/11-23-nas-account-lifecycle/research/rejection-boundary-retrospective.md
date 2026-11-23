# S02.3 账号拒绝路径复盘

日期：2026-11-23。依据最终独立复核与主线baseline的实际结果；失败/中间产物不覆盖，完整证据索引见`quality-review-s02-3.md`。

## 1. Root Cause Category

- **E 隐含假设 / D 覆盖缺口**：`bcrypt.Cost`能解析成本不等于完整摘要格式有效；最初预检遗漏尾缀、编码字符、未知minor和带符号成本。
- **B 跨层契约 / E 隐含假设 / D 覆盖缺口**：HTTP handler写413不等于客户端能读到413。本机Go1.27.4下`Connection: close`已使写头前drain跳过，提前拒绝未读取MaxBytesReader，共享小体defer按约定跳过大体。native body.Close在closing且无trailer时不读取，也没有limit-hit/early-close；直接关闭带未读输入的连接可能RST。应用health200说明并非崩溃。
- **D 证据缺口**：链接验证未经本次实际创建就推断特权不可用；ACL恢复只比较字节前缀，未核对原始长度。

## 2. Why Fixes Failed

没有将旧小体修复无界扩展到大体。早期单次超限请求碰巧通过，未暴露低频RST；独立完整回归实际失败后，旧产物30次对照29/1复现，再追踪实际工具链标准库。不能归咎随机网络后重跑到绿，也不能用旧SHA的core结果替代最终回归。Cost的宽松解析假设则由确定性损坏摘要反例推翻。

## 3. Prevention Mechanisms

| 优先级 | 机制 | 已执行动作 | 状态 |
| --- | --- | --- | --- |
| P0 | 运行时 | 显式bcrypt版本/长度/成本/字符检查后再用Cost，预检与事务重读一致 | DONE |
| P0 | 有界拒绝 | 账号已知超限局部65,537字节/250ms，Expect及不支持deadline零读取，异常保留expired | DONE |
| P0 | 稳定回归 | 100次真实独立短连接精确413/health200及分片/停滞/Expect/chunked/EOF/边界，无重试 | DONE |
| P1 | 故障断言 | 临时Go实际读取计数/deadline矩阵与损坏hash反例，执行后清源并保留日志 | DONE |
| P1 | 真实平台证据 | 文件symlink真实尝试；ACL恢复比较完整长度与字节 | DONE |
| P1 | 规范 | account/runtime正式契约及测试点同步，明确500ms原生收尾与送达边界 | DONE |

## 4. Systematic Expansion

- 共享小体收尾继续只适用于已知小体；媒体/未知体/Expect不自动套用账号局部路径。其它接口如未来引入早拒绝，应先证明各自读取/关闭生命周期，再作有界设计。
- 日后更换Go版本需在真实短连接回归中验证行为；本次结论绑定实际Go1.27.4，不把语言版本声明当运行时版本。
- 合成reader、吞TCP异常、自动重发和重复执行到成功都削弱真实证据。最终产物摘要绑定验证结果，目标环境证据另列。

## 5. Knowledge Capture

- [x] `.trellis/spec/backend/account-contract.md`补签名、精确边界、正反例、错误矩阵和防复发断言。
- [x] `.trellis/spec/backend/runtime-contract.md`明确账号局部例外及共享规则不变。
- [x] 稳定验证器、账号/API说明与`docs/verification.md`同步实际结果。
- `src/templates/markdown/spec/`在此仓库不存在，不新建虚构模板树。按会话约束未自动提交；规范已落盘可审阅。
