# 本机 UAV 修复记录

## 已确认根因

- 目标：`E:\test\qiku-0.1.0-windows-amd64-development`。
- `release.json` 为 schema 3、`developmentWithoutTools=true`，UAV、下载器、媒体工具材料字段为 null。
- 用户 `.env` 已启用 managed 和 UAV，但目标缺少 `uav-runtime.json`、`runtimes/uav/`、`licenses/uav/`、`tools/yt-dlp.exe`、`tools/ffmpeg.exe`。
- 真实目标 worker 的修复前 `--check-uav` 退出 2，输出用户报告的运行时清单/架构/版本/完整性合并错误。
- 2026-11-27 修复开始时，精确进程盘点已无目标桌面与 worker 存活，无需强杀。

## 本机材料与约束

- UAV：`.runtime/tools/uav-private/uav-runtime.json` 及清单声明目录，按仓库既有验证器静态校验与真实离线 probe。
- 下载器：`.runtime/tools/uav-windows/yt-dlp.exe`，必须核对固定 2026.10.26 及既有真实 EXE 摘要。
- 媒体工具：本机 `D:\adunsoftware\ffmpeg\ffmpeg-master-latest-win64-gpl-shared\bin` 的 FFmpeg、ffprobe 与对应 DLL，复制后在不依赖原安装目录的环境中真实执行。
- 这是用户机器本地依赖修复，保留原开发包身份；不是新增完整正式再分发发行物，不生成或伪造人工材料审核声明。

## 数据保护

- `protected-files-before.json` 记录原 `.env`、三份 EXE、`release.json`、checksums 与 desktop 配置的 SHA-256/大小。
- `database-before.json` 只记录数据库摘要与关键表行数/内容摘要，不记录账户、会话或来源原值。
- 修复前关键表行数：users=1、sessions=1、libraries=0、items=0、collection_sources=2、collection_tasks=0。
- 不修改原 EXE、配置、数据库、运行时契约或全局环境；仅安装已验证的新增运行材料，正常启动造成的日志/状态由应用自行管理。

## 执行与验证

- 补齐 795 份材料，共 291,220,702 字节；UAV 清单声明的 784 份文件、清单自身、固定下载器、FFmpeg/ffprobe 及 7 份同版本 DLL。
- 源、暂存与目标的 UAV 静态完整性和真实隔离离线 probe 均通过：CPython 3.14.4、UAV 3.8.4。
- 下载器实际版本 2026.10.26，锁定官方 EXE SHA-256 为 `f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d`。
- FFmpeg/ffprobe 实际版本 `N-122527-g4561fc5e48-20260122`；空 PATH 执行及实际 1 秒本地 H.264 编码、时长探测、完整解码通过。
- 目标原 worker 的 `--check-uav` 从修复前退出 2 变为退出 0，输出“视频插件本地依赖和下载工具可用，UAV 3.8.4”；证据 `diagnostic-before.json` / `diagnostic-after.json`。
- 原桌面入口以原 profile 和 `.env` 启动，保留正常托盘，仅本次不自动弹浏览器；原生 IPC 报告 `state=ready`、`collectionState=ready`、UAV `available=true`、版本 3.8.4，地址 `http://127.0.0.1:8090`。
- 7 份原程序/配置保护文件摘要一致，6 张关键业务表的行数和全部内容摘要一致；证据 `desktop-after.json`。
- 原 `.env` 还显式启用了独立 JMComic；目标同样未配齐其私有运行时，该预存状态按本任务范围保持。视频已单独就绪，整体获取服务 ready；没有通过停用其他插件掩盖问题。
- 材料和目标验证概要保存在 `repair-summary.json`、`target-verification.json`；795 份详细来源和摘要收据位于 `.runtime/verify/windows-uav-local-repair/stage-receipt.json`，概要绑定该收据 SHA-256。
- 未修改生产源码，没有需编译的代码变更；验证针对实际安装材料、原 worker、原桌面实例和原用户数据。未生成临时单元测试类，也未请求视频站点。

## Spec 判断

现有 `uav-windows-contract.md` 已明确私有清单、固定下载器、受控工具路径、完整分发与本地验收边界。本次只修复目标机器缺失材料，没有修改代码/接口契约，因此无需重复增加规范。
