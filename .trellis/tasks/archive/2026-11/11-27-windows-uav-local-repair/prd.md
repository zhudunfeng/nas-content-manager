# 修复本机 Windows UAV 运行材料缺失

## Goal

修复 `E:\test\qiku-0.1.0-windows-amd64-development` 中已启用 UAV 但缺少私有运行时和媒体工具的问题，使当前本机桌面实例的视频组件通过真实离线诊断。

## Requirements

- 使用仓库已存在、固定版本且通过完整性校验的 UAV 私有运行时与官方 yt-dlp 2026.10.26。
- 复用本机已安装且可运行的 FFmpeg/ffprobe，仅用于此机器的本地修复，不宣称完成正式再分发材料审核。
- 保留现有 EXE、`.env`、profile、账户、数据库、媒体和任务；不修改用户级环境、系统 PATH 或依赖校验规则。
- 补齐前记录目标状态和摘要，拒绝覆盖未知既有材料；通过既有私有运行时校验器和目标 worker 离线诊断验证。
- 如实例正在运行，正常退出并确认自有进程释放后安装；恢复原启动方式并确认托管 worker 日志或状态为视频就绪。
- 保持 `release.json` 原有开发包身份，另存本机修复记录；不伪造完整发行包或许可审核声明。

## Acceptance Criteria

- [x] 源 UAV 运行时完整静态校验及真实离线 probe 通过。
- [x] 目标运行时逐文件摘要、大小和结构校验通过。
- [x] 固定 yt-dlp、FFmpeg、ffprobe 真实本机版本探测通过。
- [x] 目标 `nas-collection-worker.exe --check-uav` 返回 0、UAV available=true。
- [x] 原 `.env`、EXE 与配置文件摘要保持，用户数据保留；桌面重启后托管视频就绪。
- [x] Trellis 独立复核与修复证据完成，无临时单元测试遗留。

## Technical Approach

按已有 `scripts/uav_runtime.py verify --probe` 与 `--check-uav` 契约执行本机材料修复，不降低校验、不向私有运行时执行 pip。先验证源材料，后补入当前程序根固定目录，再验证目标和实际桌面实例。

## Out of Scope

- 自动升级 UAV、修改站点业务、全局安装 Python 或放宽完整性校验。
- 正式公开分发、代码签名、人工许可审核声明及真实公网站点下载验收。

## User authorization

用户在已明确说明开发包缺少运行时的诊断后要求“请解决此问题”，授权对当前本机测试目录补齐依赖并正常重启验证；无需再次确认。

## Prior context

原当前任务为 `11-26-windows-dotenv`（review），本次结束后恢复该任务上下文。
