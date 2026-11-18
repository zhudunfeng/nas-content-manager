# 在当前 PowerShell 进程初始化项目内缓存；需要用点源语法导入，且不会修改系统环境。
[CmdletBinding()]
param()

$projectEnvironmentPython = Get-Command python -CommandType Application -ErrorAction Stop | Select-Object -First 1
$projectEnvironmentJson = & $projectEnvironmentPython.Source -B (Join-Path $PSScriptRoot 'project_env.py') --print-env
if ($LASTEXITCODE -ne 0) {
    throw '项目构建环境初始化失败，请检查 scripts/project-env.json 与目录写入权限。'
}
$projectEnvironmentValues = ($projectEnvironmentJson -join [Environment]::NewLine) | ConvertFrom-Json -ErrorAction Stop
foreach ($projectEnvironmentProperty in $projectEnvironmentValues.PSObject.Properties) {
    [Environment]::SetEnvironmentVariable($projectEnvironmentProperty.Name, [string]$projectEnvironmentProperty.Value, 'Process')
}
Write-Host ('构建缓存与临时目录已限定在项目内：' + (Split-Path -Parent $env:TMPDIR))
