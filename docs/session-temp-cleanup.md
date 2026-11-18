# Windows 开发会话缓存清理

固定脚本为 `scripts/clean-session-temp.ps1`，在 Windows PowerShell 5.1 或 PowerShell 7 中运行。默认只预览，将格式版本为 1 的精确文件清单写入仓库的 `.runtime/session-temp-cleanup/manifest.json`；加 `-Execute` 才消费该清单并执行清理。再次预览会更新同一清单，需要保留多份时使用 `-ManifestPath` 指定不同文件名。

## 先预览，再执行

在仓库根目录运行。以下示例对应 2026-11-17 至 18 日的本次会话；后续会话应替换开始时间与实际测试 profile 名称。

```powershell
# 预览本会话缓存，记录具体文件与预计逻辑大小。
.\scripts\clean-session-temp.ps1 -IncludeGoBuildCache -IncludeGoModuleCache -BrowserProfileName 'mcp-chrome-c3d9699' -Since '2026-11-17T17:30:00+08:00'

# 查看预览清单并关闭相关构建/测试浏览器后，再执行同一清单。
.\scripts\clean-session-temp.ps1 -Execute
```

不传选择开关时，只查找包含当前仓库绝对路径的 Node 编译缓存；不会自动选择 Go 缓存或浏览器资料。清理 Go 缓存后，下次构建会重新编译；清理模块缓存后，还需要网络重新下载依赖。

## 参数

| 参数 | 说明 |
| --- | --- |
| `-Execute` | 消费已有预览清单执行清理；省略时仅预览 |
| `-ManifestPath` | 限仓库 `.runtime/session-temp-cleanup/` 内的 `.json` 文件，可用绝对路径或仓库相对路径；预览与执行必须使用同一路径 |
| `-IncludeGoBuildCache` | 显式选择当前用户默认位置的 Go 编译/测试缓存；这是共享可重建缓存 |
| `-IncludeGoModuleCache` | 显式选择当前用户默认位置的 Go 依赖及校验缓存；这是共享可重建缓存 |
| `-BrowserProfileName` | 精确的 `mcp-chrome-<hex>` 测试 profile 名称；不支持整个 profile 父目录或通配符 |
| `-Since` | 只选择在指定时间后创建的缓存文件；建议带时区，省略时不按时间过滤 |

不要把整盘、用户根或通用 Temp 作为目标。脚本只处理已知缓存位置；预览中的共享缓存范围应由使用者核对。

## 执行行为

- 执行不重新扫描或扩大文件范围；新文件保留，元数据与清单不符的文件跳过。
- 拒绝越界、未知目标、链接/重解析点和仍被相关进程使用的目标；不会结束进程或提权。
- 只按清单删除文件并清除已空的清单目录，保留目录内后来出现或未选择的文件。
- 对已不存在的文件按幂等处理；部分失败会保留错误和实际完成结果，不能把计划大小当作已释放空间。
- 结果保存在清单相邻的 `<清单名>.<UTC时间>.result.json` 文件，供后续复查。清单和结果包含本机路径，请保留在已忽略的 `.runtime/` 下。

本功能是 Windows 开发缓存维护工具，与 NAS 业务数据、媒体库或 Docker 清理无关。

## 后续避免共享缓存

项目现有开发、构建与验证入口已使用[项目构建环境配置](project-build-environment.md)，将新产生的可控缓存放入仓库 `.runtime/`。此清理脚本仍只处理上述原有 C 盘默认缓存，不会扩大到项目缓存或业务数据。若当前终端已加载项目环境，带 Go 选择开关的旧缓存预览会因 Go 路径已重定向而拒绝；需要维护旧缓存时，在未加载项目环境的独立终端运行本脚本，仍按先预览、再执行操作。
