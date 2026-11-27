# S03.2.3 主线验收与文档收尾清单

2026-11-23。状态：实施期间的主线验收准备，**不是实施报告或通过声明**。当前实施权威是 `../executor-manual-contract.md`，SHA256 `38d99a58c43ae58821fc24fc0b349d85236a4731646bae5fdea38a504f3b28b6`。本清单不另定产品契约，不取代完整技术附录或独立可写检查。

## 1. 交接与写入顺序

1. 实施者独占本片产品、临时测试、稳定专项脚本及实施报告；主线只写任务、规范和文档，不同步正在变化的共享 CodeGraph 索引。
2. 实施者报告完整读回、摘要核对且明确全面停写后，才进入 Phase 2.2。向独立检查者注入当前冻结稿、原技术附录、冻结前审查与本片最终实施报告。
3. 独立检查者可直接修复本片问题；最终新鲜产物、源码清单和回归必须属于其最后一次修改之后。修复前失败事件保留，最终轮次单列，不累加重跑测试数。
4. 检查者全面停写后主线读原始证据、重算源码/产物摘要、核确切进程与临时清源，才同步 CodeGraph 并进入 Phase 3.1/3.3。
5. S03.2.3 的本机关闭不等于整个 S03 完成；S03.2.4 仍须单独冻结，S03.3、S03.4 与真实 Linux/NAS 验收仍保留各自状态。

## 2. 产品事实与证据对应

| 契约接缝 | 最终证据须能够回答的问题 |
| --- | --- |
| 输入和 DTO | 逐 token 严格输入、UUIDv7 version/variant/时间边界、游标及毫秒时间是否实际断言；manifest 纳秒原始 bytes/hash 是否保持。 |
| 原 key 与高水位 | 过窗 active、terminal 到期等号、未来 F/F+1、回拨边界、拒绝提交 H、最终认证回滚、提交未知是否分别验证；结果未知是否沿原 ID 读回。 |
| 有界持久事实 | 201/65/2049 lookahead、active/ownership/guard/授权保护、压缩终态重放、不续期、计数饱和与缺 key 的旧 run 去重是否覆盖。 |
| 根准备与唯一执行器 | 未绑定普通启动和 GET 是否零持久根 I/O；合法首 POST 确认 H 后 503，是否无 run/key 且原 key 重试；mu 是否不包 SQL/Commit/I/O。 |
| 源和目标预算 | 同卷/可证明异卷/无法证明异卷、算术溢出、实际空间不足分别验证；旧下载 busy 是否只退 queued、无 fileId 且保留原截止。 |
| 原子登记和完成 | owner 原 bytes/hash 是否先提交；三类 Certainty 是否同时消费 error；completed、完整 tuple、终态 key、成功摘要与授权是否同事务，源 Close 错误是否只作收尾告警。 |
| 无 source 对账 | 原 running/pending 双名确缺与原 completed/retained 缺 final 是否分列；根整体失联是否不批量 missing；每一 pending/final/DB 组合是否保留原 ID、无 VACUUM。 |
| Windows final_seen | 标记提交确认/精确读回前是否绝不反向移动；回环两次移动之间是否用确切自有进程中断验证；已标记 pending 遇 durable 取消是否保留并确认原事实。 |
| 无删除授权结束 | 首次成功必发授权、阈值内清空、第八份或字节超额成功后阻新意图、各分量 max、token/policy 漂移及坏 guard 是否有证据；阻塞空闲 tick 是否避免重复 hash。 |
| 身份和生命周期 | 接纳/取消的实际会话事务重验、撤销原会话后后台继续、HTTP 断开后继续、旧 queued 不覆写 running 句柄、seal 单向、requests-drained 及共享 DB 收尾预算是否验证。 |

## 3. 工程与原始证据读回

- 读取最终行为事件而非只读汇总：顶层、叶子、嵌套容器、独立无子测试顶层、失败与 skip 分列；区分新设计测试与复用回归。
- 记录 Go 实际版本、gofmt/vet、临时行为测试、临时清源后的包编译、受影响 Python 注解/语法、前端类型检查和构建及 Windows/Linux amd64/arm64 新鲜产物。
- 所有本地构建、测试、临时工具经 `python -B scripts/project_env.py -- <命令>`，缓存/临时材料限仓库 `.runtime/`；不引入用户级环境设置。
- 复算最终产品源码清单与三平台产物 SHA256，核实新手动专项、storage、旧 backup、account `--offline`、core `--large-file` 使用同一 Windows 产物。
- 原始日志先作秘密检查，再生成脱敏展示。明确检查的原始文件、已知秘密集合、字节/编码边界，不把有限扫描描述成不存在任何泄露的证明。
- 精确核对新增临时测试源已清除及保留 `.go.txt` 的摘要；不删除既存测试或失败证据。
- 按产物绝对路径核对活进程；正常 Console 停服、超时后 Job 回收、进程中断夹具各自计数。根/数据库/端口释放及立即同根重启须有实际断言。
- Windows 本机运行、Linux 交编、真实 Linux/NAS、介质故障、进程中断、系统崩溃和物理断电单列，未测项保留待验。

## 4. 正式文档待同步位置

本节只是收尾定位，须待独立检查及主线证据通过后才将状态改为已实现。

- `.trellis/spec/backend/backup-manual-contract.md`：新增七节可执行规范，覆盖五路由、DTO、时钟/事务、唯一 executor、原 ID 对账、授权结束和停服；标本机完成、目标待验。
- `.trellis/spec/backend/index.md`、`runtime-contract.md`：加入手动执行契约入口，保持快照/存储/根发布各自边界。
- `.trellis/spec/backend/backup-contract.md`：将“未来消费者”说明接到唯一手动 executor，共用原 snapshot lease；旧 `GET /api/backup` 仍按同步临时下载处理。
- `.trellis/spec/backend/backup-publish-contract.md`：只补消费者链接及对账协调入口边界，不重写已验收平台协议或弱化 Certainty。
- `docs/api.md`：维护章节加入五路由、strict JSON/query、keyset、窗口和确认结果语义；更新早期切片的“没有持久入口”描述，保持历史分层准确。
- `README.md`：描述手动持久 API 已可用及首次准备/同 key 重试、keep7 阶段积压、database-only 和敏感材料；明确策略仍关闭，定时、下载、轮转及 UI/恢复仍待后片。
- `docs/verification.md`：追加最终证据定位、失败修复、行为计数、同产物回归、清源/进程及测试边界，不改写 S03.2.2 历史快照。
- 当前 `prd.md`、`info.md`、`task.json`：仅在主线核验后将 S03.2.3 加入本机完成；整个 S03 维持进行中。按下一片真实准备程度更新 active_slice/phase，不提前开放 S03.2.4。

## 5. 收尾状态

实施进行中，尚无 S03.2.3 产品门禁通过数。主线没有重跑 S03.2.2 的历史审计，也没有用旧源码摘要判断本片新改动是否漂移。
