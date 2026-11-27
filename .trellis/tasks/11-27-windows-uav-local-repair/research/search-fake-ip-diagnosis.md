# Windows 搜索失败：Fake-IP 与公网校验冲突

检查日期：2026-11-27（Asia/Shanghai）。这是运行时修复完成后对新错误的只读排查，没有调整用户网络或生产代码。

## 现场证据

- 原桌面控制接口仍报告 `collectionState=ready`，UAV 3.8.4 `available=true`。
- 执行端日志在 08:04:41 记录视频解析依赖本地探测通过；08:08:16 的 jable 搜索和 08:08:27 的 huangguo 搜索记录“视频搜索未完成，请检查站点与网络”。
- 当前 Windows 系统解析结果：`jable.tv -> 198.18.1.3`；`huangguoai.com -> 198.18.1.5`，均为非公网地址。
- 当前运行 `clash-verge-service.exe`、`clash-verge.exe` 和 `verge-mihomo.exe`。
- `internal/collectionworker/proxy.go` 的 `publicDial` 在连接前核对所有 DNS 答案，`publicIP` 明确拒绝 `198.18.0.0/15`。上述两个地址会命中拒绝规则。
- `uavSearchHTTP` 的上述警告来自 `callUAV` 失败分支，不是共享并发槽繁忙分支。
- `internal/app/collection_worker.go` 对未单列处理的非成功响应统一显示“资源获取执行端拒绝操作，请检查连接、工具与限额”，因此界面文案不能证明限额耗尽。

## 判断与处理方向

已识别当前确定的阻塞点：系统返回的 Fake-IP 与执行端的公网安全校验冲突。原有私有运行时缺失已修复，与本次 DNS 拒绝是不同阶段的问题。

优先在 Clash/Mihomo 侧让相关站点返回真实公网 IP，例如按其现有配置模式设置 Fake-IP 过滤，或使用 `redir-host` DNS 模式；重载后清理 DNS 缓存并重新验证。站点、镜像及 CDN 可能另有域名，仅处理两个搜索域名不能保证后续全部下载链路通过。

不要删除执行端对保留网段的安全校验。当前执行端自有代理禁用环境代理；仅打开 Windows 系统代理不保证执行端出口可用，还需保留可用的透明路由或 TUN 出口。

未修改 Clash 配置、Windows DNS 或运行中的服务。未将站点搜索或下载标记为验证通过。

参考：Mihomo 官方 DNS 文档 https://wiki.metacubex.one/config/dns/ ，说明 `enhanced-mode` 的 `fake-ip` / `redir-host` 选择以及 `fake-ip-filter` 的真实地址过滤语义。
