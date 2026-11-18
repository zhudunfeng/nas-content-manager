# 启动本机开发服务；Go 运行 API，Vite 自动代理 /api。
param(
    # 只启动后端、只启动前端，或编译生产版本。
    [ValidateSet('backend', 'frontend', 'build')][string]$Mode = 'backend',
    # 允许注册的媒体根目录；多个目录使用分号分隔。
    [string]$MediaRoots = ''
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'project-env.ps1')
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
# 优先使用系统 Go，其次使用项目本地可选工具链。
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goExecutable = if ($goCommand) { $goCommand.Source } else { Join-Path $projectRoot '.tools/go/bin/go.exe' }
if ($Mode -eq 'frontend') {
    & npm --prefix web run dev -- --host 127.0.0.1
    exit $LASTEXITCODE
}
if (-not (Test-Path -LiteralPath $goExecutable)) { throw '请安装 Go，或将便携工具链放入 .tools/go。' }
if ($MediaRoots) { $env:NAS_MEDIA_ROOTS = $MediaRoots }
if ($Mode -eq 'build') {
    & npm --prefix web run build
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    New-Item -ItemType Directory -Force bin | Out-Null
    & $goExecutable build -trimpath -o bin/nas.exe ./cmd/nas
    exit $LASTEXITCODE
}
& $goExecutable run ./cmd/nas
exit $LASTEXITCODE
