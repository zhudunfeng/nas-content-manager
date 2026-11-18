# 会话缓存清理独立复核

日期：2026-11-18。复核范围仅为 `scripts/clean-session-temp.ps1`、当前 PRD 与既有跨平台维护契约。未修改应用业务、密码实现、Docker 或用户进程。

## Findings (fixed)

1. **重复预览无法覆盖已有清单**：`[IO.File]::Replace(..., $null)` 的第三参数在 PowerShell 重载绑定时被当作空字符串，导致第二次预览报空路径。改用 `[System.Management.Automation.Language.NullString]::Value` 传递真正的 CLR null；在两版 PowerShell 中均实际执行首次写入、再次覆盖与读回，确认不存在遗留 `.tmp` 文件。
2. **Node 缓存路径标记可能误选兄弟项目**：简单仓库路径子串搜索会命中 `nas-content-manager-old` 或 `nas-content-manager copy`。增加仓库路径后的分隔符/终止符边界，保留 Windows 与正斜杠两种表达；两版运行时的隔离回归均确认兄弟项目不入选，真实只读预览仍匹配原有 139 个文件。

## Findings (not fixed)

无未修复的范围内缺陷。

## Verification

- Windows PowerShell `5.1.19041.6456` 与 PowerShell `7.6.4` 解析器均为 **0 错误**；脚本有 UTF-8 BOM，中文注释在 5.1 可正确读取。
- 独立临时验证源在两版 PowerShell 中各执行 **34 项断言，全部通过**。所有需要删除、修改、锁定或设置只读属性的样本均位于 E 盘仓库 `.runtime/session-temp-cleanup/review-fixture-<GUID>` 内；测试结束自动删除各自 fixture。
- 断言覆盖：两种仓库路径标记、两种前缀兄弟项目排除、相对/点点/ADS/尾点/设备名路径拒绝、目录前缀越界、目标种类别名/浏览器通配符/清单输出越界拒绝、精确快照、JSON 首次写入和重复原子替换、无临时输出残留、清单文件和根越界篡改、文件/目录结构混淆、模拟 Go 进程/进程查询失败/指定浏览器阻断、普通浏览器保留，以及普通/只读文件成功删除、锁定文件部分失败、变更/新增/缺失文件、已登记空目录与无关空目录、成功删除字节统计、解锁重试及幂等。
- **真实只读预览**：PowerShell 7 执行 `scripts/clean-session-temp.ps1 -ManifestPath .runtime/session-temp-cleanup/review-manifest.json`，选择 Node 缓存 **139 文件、5,733,716 逻辑字节**；结果只写入 E 盘仓库清单，未删除 C 盘缓存。
- 语言没有独立的项目类型检查命令；本轮使用参数/返回类型人工复核、两版解析器与真实函数调用进行验证。未安装 PSScriptAnalyzer，也未为此下载新依赖；不将解析检查伪称为 PSScriptAnalyzer 结果。
- 维护脚本未触及 Go/Vue 业务代码，本轮未重复 Go/npm 全应用构建，未执行 Docker。
- 临时验证源 `review-tests.ps1` 的删除命令在进程创建前被自动审批拒绝；复核代理未通过替代 API 重试，交由父代理处理收尾并记录最终状态。fixture 已由成功的隔离验证在各自进程内自动删除。

## 父代理收尾补充

父代理改用无变量、无递归、无 `-Force` 的 E 盘精确单文件命令，成功删除本任务新建的 `review-tests.ps1` 与 `verify-temp.ps1`。随后目录读回仅有 JSON 预览/验证记录，没有测试源、fixture 或 `.cleanup-*.tmp` 残留。此操作只涉及本仓库 E 盘新建测试源，不涉及被拒绝的 C 盘缓存删除。

最终全选真实只读预览生成默认 `manifest.json` 成功，共 25,182 文件、1,735,601,734 逻辑字节，与先前盘点一致。父代理另复查清单与当前文件元数据：缺失 0、变化 0、相关目标空闲，临时测试源不存在；结果保存在忽略目录的 `preview-verification.json`。

## 执行限制

先前真实 C 盘递归删除被工具自动审批拒绝，本次没有通过脚本、替代 API 或其他进程重试真实 C 盘删除。测试中实际删除的是自建 E 盘 fixture；这不代表真实缓存已清理或已释放磁盘空间。真实全选预览清单及交付记录由父代理完成。
