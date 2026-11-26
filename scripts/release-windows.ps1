# 将发布参数交给统一构建器；终端用户只运行成品EXE，无需此脚本。
param(
    # 固定FFmpeg版本、摘要、许可证和对应源码材料清单。
    [string]$ToolsManifest,
    # 可选Docker固定镜像及离线归档清单。
    [string]$ImagesManifest,
    # 可选JMComic私有Python运行时清单，构建完整漫画功能包时显式提供。
    [string]$JMComicRuntimeManifest,
    # 归档输出目录，默认使用仓库bin/releases。
    [string]$Output,
    # 明确生成不含FFmpeg的开发包，不能作为完整离线包分发。
    [switch]$DevelopmentWithoutTools,
    # 复用已经构建的web/dist，默认重新构建。
    [switch]$SkipWebBuild
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
$releaseArguments = @((Join-Path $PSScriptRoot 'release.py'), '--target', 'windows-amd64')
if ($ToolsManifest) { $releaseArguments += @('--tools-manifest', $ToolsManifest) }
if ($ImagesManifest) { $releaseArguments += @('--images-manifest', $ImagesManifest) }
if ($JMComicRuntimeManifest) { $releaseArguments += @('--jmcomic-runtime-manifest', $JMComicRuntimeManifest) }
if ($Output) { $releaseArguments += @('--output', $Output) }
if ($DevelopmentWithoutTools) { $releaseArguments += '--development-without-tools' }
if ($SkipWebBuild) { $releaseArguments += '--skip-web-build' }
& python @releaseArguments
exit $LASTEXITCODE
