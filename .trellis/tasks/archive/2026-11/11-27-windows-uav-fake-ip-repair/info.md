# Windows UAV Fake-IP 修复结果

2026-11-27，Asia/Shanghai。

已修复当前机器的 Clash/Mihomo Fake-IP 与执行端公网地址校验冲突。没有修改生产源码、主程序、`.env`、执行端安全规则或资源预算。

## 实际变更

- 原 Clash 用户配置根：`C:/Users/Administrator/AppData/Roaming/io.github.clash-verge-rev.clash-verge-rev`。
- `profiles/Script.js`：原文件是默认 identity 脚本；加入中文说明和类型注释，以追加去重方式补充 `+.jable.tv`、`+.huangguoai.com`、`+.fs1.app`。保留已有和未来订阅中的其他过滤项。遇到非黑名单模式或不兼容数组明确停止。
- `clash-verge.yaml`：当前运行配置只新增上述三个 Fake-IP 过滤项，其他配置语义与备份完全一致。
- 使用原命名管道重载，返回 204；Windows DNS 缓存清理成功。没有切换全局 DNS 模式，也没有关闭 TUN。
- 私密备份和候选：原配置根下 `uav-fake-ip-repair-20261127/`。含节点/凭据的原配置没有复制到项目目录。回滚步骤见 `research/clash-dns.md`。

## 验证

1. `live-before.json`：两站系统 DNS 为 198.18 段，原 worker 搜索均 502。
2. `live-after.json`：两域名均已返回公网地址；Jable 200，24 条。
3. `live-focused.json`：黄果另一组关键词经原 worker 搜索 200，40 条。
4. `final-verification.json`：精确配置差异、持久脚本回读、Node 隔离 9 项检查、核心接口/TUN、UAV 就绪、7 个原文件及 6 张业务表保全全部通过。
5. 原进程保持不变：Clash Verge 58932、Mihomo 27984、栖库 59668、worker 37416。热重载过程中未杀进程或重启。

## 需要区分的限制

黄果 `test` 搜索在网络修复后仍返回 502。固定上游诊断共观察到 12 个 HTTP 200 响应，脱敏异常栈落在 `huangguo.py:695`，对应“expanded page exceeds the episode safety limit”；原桥接将该上限设为每页 100 集。没有将此特定关键词查询声称为成功，也没有移除预算控制。另一关键词查询已通过原 worker，证明该站搜索链路可以完成。

两站成功搜索的 worker 日志位置：原 profile 的 `logs/collection-worker.log` 第 27 行（Jable）与第 29 行（黄果）。没有执行媒体下载、发布或入库。

## 工具与检查范围

临时工具仅位于 `.runtime/verify/windows-uav-fake-ip-repair/`，无新增单元测试类。没有生产码变化，因此未以全仓 Go/前端测试代替现场配置和搜索验证。未提交 Git；任务材料可供审阅。
