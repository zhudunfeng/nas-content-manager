# Bug Analysis: HLS 传输成功与媒体完整性的边界

## 1. Root Cause Category

- **Category**: B / D / E — 跨层契约、测试覆盖缺口与隐含假设。
- **Specific Cause**: relay 的传输错误、HTTP 响应完成、yt-dlp 成功退出、FFmpeg 可解码分别代表不同条件。最初只核验其中一层，不能证明所请求的范围或清单时长实际完整到达。

## 2. Why Fixes Failed

1. 只看 HTTP 成功状态会接受忽略 Range 的 200，下载器将整文件当作单个分片。
2. 增加 206、Content-Range 和声明长度仍不覆盖无 Content-Length 的 chunked 过长正文。
3. 发现超长后再中止连接，若下游已收到完整声明长度，真实 yt-dlp 仍可成功退出。最终保留最后一字节，确认上游恰好 EOF 后才发送。
4. 全文件解码可接受在有效 TS 包边界截断的短视频；实际 8 秒样例截至约 1.96 秒仍可重封装和解码。最终依据 HLS EXTINF 总时长核验解码时长下界。

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Runtime | 单个闭合 Range，精确 206 / Content-Range / 实际长度，末字节在 EOF 验证后发送 | DONE |
| P0 | Runtime | HLS 解码时长至少达到 EXTINF 总和减 max(0.5 秒, 2%)；保留帧与 progress=end 检查 | DONE |
| P0 | Test Coverage | 实际锁定 yt-dlp 穿过生产 relay，覆盖正确、忽略、错误、短、长及 chunked Range | 独立复核报告记录最终结果 |
| P0 | Test Coverage | 真实 FFmpeg 生成 8 秒 TS 与包边界截尾反例，验证可解码与时长下界分别成立 | 独立复核报告记录最终结果 |
| P1 | Documentation | 在规范及用户文档解释容差和直链 MP4 无可信预期时长的限制 | DONE |

## 4. Systematic Expansion

- **Similar Issues**: AES 密钥也不能只依赖下载器宽松接受，当前资源代理严格限制为恰好 16 字节；上游缺片、GAP 和不支持的标签在清单阶段拒绝。
- **Design Improvement**: 所有 HLS 资源统一经过有限登记的操作作用域代理，下载与校验完成后才进入已有发布/扫描流程。
- **Process Improvement**: 协议替身集成验证与真实依赖验证分别记账；handler 中止、下载器失败和最终任务不发布要按各自证据表述。

## 5. Knowledge Capture

- [x] `.trellis/spec/backend/uav-contract.md` 已记录精确 Range、末字节门禁、时长下界及真实下载器反例要求。
- [x] `docs/uav.md` 已解释 HLS 容差与直链 MP4 完整性验证边界。
- [x] 本仓库不存在 `src/templates/markdown/spec/`，无对应生成模板可同步。
- [x] 不自动提交；按项目收尾流程由用户审阅并提交。

本报告分析已复现缺陷及修复机制；最终质量门禁与实际通过矩阵以 `quality-check.md` 为准。
