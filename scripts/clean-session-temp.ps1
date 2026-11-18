#requires -Version 5.1
<#
.SYNOPSIS
预览或按既有清单清理当前用户的开发会话缓存。
.DESCRIPTION
默认只选择包含本仓库路径的 Node 编译缓存。共享 Go 缓存与测试浏览器
必须显式选择。先审阅清单、关闭相关进程，再用 -Execute 消费同一清单。
#>
[CmdletBinding()]
param(
    # 明确执行已有清单；不指定时只预览，不删除缓存。
    [switch]$Execute,
    # 清单文件位置，限本仓库 .runtime/session-temp-cleanup 下的 JSON 文件。
    [string]$ManifestPath = '',
    # 显式选择当前用户默认 Go 编译缓存；此缓存由多个项目共享且可重新生成。
    [switch]$IncludeGoBuildCache,
    # 显式选择当前用户默认 Go 模块与校验缓存；下次构建可能需要联网下载。
    [switch]$IncludeGoModuleCache,
    # 精确测试浏览器目录名，格式为 mcp-chrome-十六进制标识，不接受通配符。
    [string]$BrowserProfileName = '',
    # 可选会话开始时间；只选择在该时间或之后创建的文件，建议包含时区。
    [Nullable[datetimeoffset]]$Since = $null
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:CleanupScriptDirectory = $PSScriptRoot

function Get-CanonicalLocalPath {
    <# .SYNOPSIS 校验并规范化本机绝对路径，拒绝相对路径及 Windows 路径别名。 #>
    [OutputType([string])]
    param([Parameter(Mandatory = $true)][string]$Path)
    if ($Path -notmatch '^[A-Za-z]:[\\/]' -or $Path -match '[\x00-\x1f]') {
        throw "只允许本机驱动器绝对路径：$Path"
    }
    $normalized = $Path.Replace('/', '\')
    foreach ($component in $normalized.Substring(3).Split('\')) {
        if ($component -eq '') { continue }
        if ($component -eq '.' -or $component -eq '..' -or $component -match '[ .]$' -or
            $component -match '[:*?"<>|]' -or $component -match '^(?i:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)') {
            throw "路径含不允许的别名、通配符或越界片段：$Path"
        }
    }
    return [IO.Path]::GetFullPath($normalized).TrimEnd('\')
}

function Test-PathWithinRoot {
    <# .SYNOPSIS 以目录边界判断路径是否属于指定根，防止同名前缀兄弟目录越界。 #>
    [OutputType([bool])]
    param([string]$Path, [string]$Root, [switch]$AllowRoot)
    return (($AllowRoot -and $Path.Equals($Root, [StringComparison]::OrdinalIgnoreCase)) -or
        $Path.StartsWith($Root + '\', [StringComparison]::OrdinalIgnoreCase))
}

function Assert-NoReparsePoint {
    <# .SYNOPSIS 检查目标与每层祖先，拒绝符号链接、目录联接和其他重解析点。 #>
    [OutputType([void])]
    param([Parameter(Mandatory = $true)][string]$Path)
    $cursor = $Path
    while ($cursor) {
        try {
            $attributes = [IO.File]::GetAttributes($cursor)
            if (($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) {
                throw "拒绝重解析点及其后代：$cursor"
            }
        }
        catch {
            $cause = $_.Exception.GetBaseException()
            if ($cause -isnot [IO.FileNotFoundException] -and $cause -isnot [IO.DirectoryNotFoundException]) { throw }
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}

function Get-CleanupContext {
    <# .SYNOPSIS 从系统当前用户位置建立固定白名单，不接受任意清理根目录。 #>
    [OutputType([pscustomobject])]
    param()
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw '本脚本仅支持 Windows。' }
    $repository = Get-CanonicalLocalPath (Split-Path -Parent $script:CleanupScriptDirectory)
    $local = Get-CanonicalLocalPath ([Environment]::GetFolderPath('LocalApplicationData'))
    $profile = Get-CanonicalLocalPath ([Environment]::GetFolderPath('UserProfile'))
    # 只允许系统解析到的当前用户 C 盘缓存，不根据调用者提供的路径扩大范围。
    if ($local -notmatch '^(?i)C:\\' -or $profile -notmatch '^(?i)C:\\') {
        throw '当前用户配置目录不在 C 盘，本脚本不选择这些缓存。'
    }
    return [pscustomobject]@{
        # 绑定本次清单的仓库根目录。
        RepositoryRoot = $repository
        # 清单与结果记录的固定输出目录。
        ManifestDirectory = Join-Path $repository '.runtime\session-temp-cleanup'
        # 允许的目标种类及唯一缓存根。
        Roots = @{
            Node = Join-Path $local 'Temp\node-compile-cache'
            GoBuild = Join-Path $local 'go-build'
            GoModule = Join-Path $profile 'go\pkg\mod'
            GoSum = Join-Path $profile 'go\pkg\sumdb'
            BrowserParent = Join-Path $local 'ms-playwright-mcp'
        }
        # Go 默认工作区，仅用于拒绝重定向到其他位置的配置。
        GoPath = Join-Path $profile 'go'
    }
}

function Get-ManifestFilePath {
    <# .SYNOPSIS 将清单限定在仓库专用目录，避免输出覆盖其他文件或跟随链接。 #>
    [OutputType([string])]
    param([pscustomobject]$Context, [string]$RequestedPath)
    $candidate = if (-not $RequestedPath) { Join-Path $Context.ManifestDirectory 'manifest.json' }
        elseif ([IO.Path]::IsPathRooted($RequestedPath)) { $RequestedPath }
        else { Join-Path $Context.RepositoryRoot $RequestedPath }
    $candidate = Get-CanonicalLocalPath $candidate
    if (-not (Test-PathWithinRoot $candidate $Context.ManifestDirectory) -or
        [IO.Path]::GetExtension($candidate) -cne '.json' -or $candidate -match '\.result\.json$') {
        throw '清单必须是本仓库 .runtime/session-temp-cleanup 下的 .json 文件，且不能使用结果文件名。'
    }
    Assert-NoReparsePoint $candidate
    return $candidate
}

function Write-CleanupJson {
    <# .SYNOPSIS 在清单目录中用同目录临时文件原子发布 UTF-8 JSON，不向 C 盘写临时文件。 #>
    [OutputType([void])]
    param([pscustomobject]$Context, [string]$Path, [object]$Value)
    if (-not (Test-PathWithinRoot (Get-CanonicalLocalPath $Path) $Context.ManifestDirectory)) { throw '输出路径超出清单目录。' }
    Assert-NoReparsePoint $Path
    $directory = [IO.Path]::GetDirectoryName($Path)
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    Assert-NoReparsePoint $directory
    $temporary = Join-Path $directory ('.cleanup-' + [guid]::NewGuid().ToString('N') + '.tmp')
    try {
        [IO.File]::WriteAllText($temporary, ($Value | ConvertTo-Json -Depth 8), (New-Object Text.UTF8Encoding($false)))
        Assert-NoReparsePoint $Path
        # 使用真正的 CLR null，避免 PowerShell 将空备份路径绑定为空字符串。
        if ([IO.File]::Exists($Path)) { [IO.File]::Replace($temporary, $Path, [System.Management.Automation.Language.NullString]::Value) }
        else { [IO.File]::Move($temporary, $Path) }
    }
    finally {
        if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) }
    }
}

function Assert-GoCacheConfiguration {
    <# .SYNOPSIS 读取实际 Go 配置，拒绝将自定义共享缓存误认为当前用户默认位置。 #>
    [OutputType([void])]
    param([pscustomobject]$Context, [bool]$Build, [bool]$Module)
    if (-not $Build -and -not $Module) { return }
    $command = Get-Command go -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    $executable = if ($command) { $command.Source } else { Join-Path $Context.RepositoryRoot '.tools\go\bin\go.exe' }
    if (-not [IO.File]::Exists($executable)) { throw '找不到 Go，无法核对缓存配置；请取消 Go 选择或配置可用的 Go。' }
    $raw = & $executable env -json GOCACHE GOPATH GOMODCACHE
    if ($LASTEXITCODE -ne 0) { throw '读取 Go 缓存配置失败，未执行清理。' }
    $settings = ($raw -join "`n") | ConvertFrom-Json
    if ($Build -and (Get-CanonicalLocalPath $settings.GOCACHE) -ine $Context.Roots.GoBuild) {
        throw 'GOCACHE 已重定向，本脚本仅支持当前用户默认 Go 编译缓存。'
    }
    if ($Module -and ((Get-CanonicalLocalPath $settings.GOPATH) -ine $Context.GoPath -or
        (Get-CanonicalLocalPath $settings.GOMODCACHE) -ine $Context.Roots.GoModule)) {
        throw 'GOPATH 或 GOMODCACHE 已重定向，本脚本仅支持当前用户默认 Go 模块缓存。'
    }
}

function Test-RepositoryCacheMarker {
    <# .SYNOPSIS 只读取缓存内容判断本仓库路径标记，不记录或输出缓存内容。 #>
    [OutputType([bool])]
    param([string]$Path, [string]$RepositoryRoot)
    $file = New-Object IO.FileInfo($Path)
    # 限制单文件读取量，异常超大缓存不会占满内存或被自动选择。
    if (-not $file.Exists -or $file.Length -gt 64MB) { return $false }
    $content = [Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($Path))
    foreach ($marker in @($RepositoryRoot, $RepositoryRoot.Replace('\', '/'))) {
        # 仓库名之后必须是目录分隔符或路径终止符，不能命中同名前缀的兄弟项目。
        $pattern = [regex]::Escape($marker) + '(?=[\\/\x00\r\n"]|$)'
        if ([regex]::IsMatch($content, $pattern, ([Text.RegularExpressions.RegexOptions]::IgnoreCase -bor [Text.RegularExpressions.RegexOptions]::CultureInvariant))) { return $true }
    }
    return $false
}

function New-CleanupTarget {
    <# .SYNOPSIS 创建白名单目标，浏览器仅接受明确指定的测试资料目录。 #>
    [OutputType([pscustomobject])]
    param([pscustomobject]$Context, [string]$Kind, [string]$ProfileName = '')
    if (@('Node', 'GoBuild', 'GoModule', 'GoSum', 'BrowserProfile') -cnotcontains $Kind) { throw "未知清理目标种类：$Kind" }
    $root = if ($Kind -eq 'BrowserProfile') {
        if ($ProfileName -cnotmatch '^mcp-chrome-[0-9a-f]+$') { throw '浏览器目录名必须精确匹配 mcp-chrome-<小写十六进制标识>。' }
        Join-Path $Context.Roots.BrowserParent $ProfileName
    }
    elseif (@('Node', 'GoBuild', 'GoModule', 'GoSum') -ccontains $Kind) { $Context.Roots[$Kind] }
    else { throw "未知清理目标种类：$Kind" }
    $root = Get-CanonicalLocalPath $root
    Assert-NoReparsePoint $root
    return [pscustomobject]@{
        # 固定目标标识，同时用于文件与目录的归属校验。
        id = $Kind
        # 固定白名单中的缓存类别。
        kind = $Kind
        # 当前用户的精确缓存目录，不可由清单改写到其他位置。
        root = $root
    }
}

function Get-CleanupSnapshot {
    <# .SYNOPSIS 不跟随链接地扫描已选缓存，记录精确文件及其祖先目录。 #>
    [OutputType([pscustomobject])]
    param([pscustomobject]$Context, [object[]]$Targets, [Nullable[datetimeoffset]]$CreatedSince)
    $files = New-Object 'Collections.Generic.List[object]'
    $directories = @{}
    foreach ($target in $Targets) {
        Assert-NoReparsePoint $target.root
        if (-not [IO.Directory]::Exists($target.root)) {
            Write-Host ('缓存目录不存在，跳过：' + $target.root)
            continue
        }
        $pending = New-Object 'Collections.Generic.Stack[string]'
        $pending.Push($target.root)
        while ($pending.Count -gt 0) {
            $directory = $pending.Pop()
            Assert-NoReparsePoint $directory
            $entries = (New-Object IO.DirectoryInfo($directory)).GetFileSystemInfos()
            foreach ($entry in $entries) {
                if (($entry.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "拒绝缓存中的重解析点：$($entry.FullName)" }
                if (($entry.Attributes -band [IO.FileAttributes]::Directory) -ne 0) {
                    $pending.Push($entry.FullName)
                    continue
                }
                if ($null -ne $CreatedSince -and $entry.CreationTimeUtc -lt $CreatedSince.UtcDateTime) { continue }
                if ($target.kind -eq 'Node' -and -not (Test-RepositoryCacheMarker $entry.FullName $Context.RepositoryRoot)) { continue }
                # 内容读取可能遇到并发更新，重新读取元数据后才保存快照。
                $entry.Refresh()
                if (-not $entry.Exists) { continue }
                Assert-NoReparsePoint $entry.FullName
                $files.Add([pscustomobject]@{
                    # 文件归属的固定目标标识。
                    targetId = $target.id
                    # 文件的规范绝对路径。
                    path = $entry.FullName
                    # 计划文件的逻辑字节数，不等于最终释放空间。
                    length = [long]$entry.Length
                    # 创建时间 UTC ticks，用于发现文件被替换。
                    creationUtcTicks = [long]$entry.CreationTimeUtc.Ticks
                    # 修改时间 UTC ticks，用于跳过后续写入的文件。
                    lastWriteUtcTicks = [long]$entry.LastWriteTimeUtc.Ticks
                })
                $ancestor = $directory
                while (Test-PathWithinRoot $ancestor $target.root -AllowRoot) {
                    # Node 根为其他项目共享，永远保留该根目录。
                    if ($target.kind -eq 'Node' -and $ancestor -ieq $target.root) { break }
                    if (-not $directories.ContainsKey($ancestor)) {
                        $info = New-Object IO.DirectoryInfo($ancestor)
                        $directories[$ancestor] = [pscustomobject]@{
                            # 目录所属目标。
                            targetId = $target.id
                            # 只允许在执行时清除已空的此目录。
                            path = $ancestor
                            # 目录创建时间，防止清除后来重建的同名目录。
                            creationUtcTicks = [long]$info.CreationTimeUtc.Ticks
                        }
                    }
                    if ($ancestor -ieq $target.root) { break }
                    $ancestor = [IO.Path]::GetDirectoryName($ancestor)
                }
            }
        }
    }
    return [pscustomobject]@{
        # 具有本次选择依据的精确文件快照。
        Files = $files.ToArray()
        # 仅记录入选文件的祖先目录，避免清除无关空目录。
        Directories = @($directories.Values | Sort-Object path)
    }
}

function Assert-ObjectFields {
    <# .SYNOPSIS 拒绝缺失、多余或大小写别名字段，避免错误解释清单结构。 #>
    [OutputType([void])]
    param([object]$Value, [string[]]$Names)
    if ($null -eq $Value -or $Value -isnot [pscustomobject]) { throw '清单项目必须为 JSON 对象。' }
    $actual = @($Value.PSObject.Properties.Name)
    if ($actual.Count -ne $Names.Count) { throw '清单字段数量不正确。' }
    foreach ($name in $Names) { if ($actual -cnotcontains $name) { throw "清单缺少准确字段：$name" } }
}

function Assert-IntegerField {
    <# .SYNOPSIS 校验 JSON 整数和有效范围，拒绝字符串、负数及溢出时间。 #>
    [OutputType([void])]
    param([object]$Value, [long]$Minimum = 0, [long]$Maximum = [long]::MaxValue)
    if (($Value -isnot [int] -and $Value -isnot [long]) -or $Value -lt $Minimum -or $Value -gt $Maximum) {
        throw '清单数字字段不是范围内的整数。'
    }
}

function Read-CleanupManifest {
    <# .SYNOPSIS 加载并完整校验已有清单，在任何删除前拒绝越界、链接及未知目标。 #>
    [OutputType([pscustomobject])]
    param([pscustomobject]$Context, [string]$Path)
    Assert-NoReparsePoint $Path
    $file = New-Object IO.FileInfo($Path)
    if (-not $file.Exists -or $file.Length -gt 128MB) { throw '清单不存在或超过 128 MiB；请先生成并审阅预览。' }
    $plan = [IO.File]::ReadAllText($Path, [Text.Encoding]::UTF8) | ConvertFrom-Json
    Assert-ObjectFields $plan @('schemaVersion', 'repositoryRoot', 'createdUtc', 'sinceUtc', 'targets', 'files', 'directories')
    Assert-IntegerField $plan.schemaVersion 1 1
    if ($plan.repositoryRoot -isnot [string] -or (Get-CanonicalLocalPath $plan.repositoryRoot) -ine $Context.RepositoryRoot) { throw '清单不属于当前仓库。' }
    $parsedDate = [datetimeoffset]::MinValue
    # PowerShell 7 会将 ISO JSON 字符串自动转换为 DateTime，5.1 则保留字符串。
    if ($plan.createdUtc -is [datetime] -or $plan.createdUtc -is [datetimeoffset]) { $plan.createdUtc = $plan.createdUtc.ToString('o') }
    if ($plan.sinceUtc -is [datetime] -or $plan.sinceUtc -is [datetimeoffset]) { $plan.sinceUtc = $plan.sinceUtc.ToString('o') }
    if ($plan.createdUtc -isnot [string] -or -not [datetimeoffset]::TryParse($plan.createdUtc, [ref]$parsedDate)) { throw '清单生成时间无效。' }
    if ($null -ne $plan.sinceUtc -and ($plan.sinceUtc -isnot [string] -or -not [datetimeoffset]::TryParse($plan.sinceUtc, [ref]$parsedDate))) { throw '清单时间筛选无效。' }
    if ($plan.targets -isnot [array] -or $plan.files -isnot [array] -or $plan.directories -isnot [array]) { throw '清单目标、文件和目录必须为数组。' }
    $targets = @{}
    foreach ($target in $plan.targets) {
        Assert-ObjectFields $target @('id', 'kind', 'root')
        if ($target.kind -isnot [string] -or $target.id -cne $target.kind -or $targets.ContainsKey($target.id)) { throw '清单目标种类或标识无效。' }
        $profileName = if ($target.kind -eq 'BrowserProfile') { [IO.Path]::GetFileName([string]$target.root) } else { '' }
        $expected = New-CleanupTarget $Context $target.kind $profileName
        if ($target.root -isnot [string] -or (Get-CanonicalLocalPath $target.root) -ine $expected.root) { throw '清单缓存根不在固定白名单中。' }
        $targets[$target.id] = $expected
    }
    $seen = New-Object 'Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
    $groups = @(
        # 文件组必须严格使用文件结构，不能伪装成目录。
        @{ Items = @($plan.files); IsFile = $true },
        # 目录组必须严格使用目录结构。
        @{ Items = @($plan.directories); IsFile = $false }
    )
    foreach ($group in $groups) {
      foreach ($item in $group.Items) {
        $isFile = $group.IsFile
        $fields = if ($isFile) { @('targetId', 'path', 'length', 'creationUtcTicks', 'lastWriteUtcTicks') } else { @('targetId', 'path', 'creationUtcTicks') }
        Assert-ObjectFields $item $fields
        if ($item.targetId -isnot [string] -or -not $targets.ContainsKey($item.targetId) -or $item.path -isnot [string]) { throw '清单项目缺少有效目标或路径。' }
        $canonical = Get-CanonicalLocalPath $item.path
        $target = $targets[$item.targetId]
        $allowRoot = -not $isFile -and $target.kind -ne 'Node'
        if (-not (Test-PathWithinRoot $canonical $target.root -AllowRoot:$allowRoot) -or -not $seen.Add($canonical)) { throw "清单项目越界或重复：$canonical" }
        Assert-NoReparsePoint $canonical
        Assert-IntegerField $item.creationUtcTicks 1 ([datetime]::MaxValue.Ticks)
        if ($isFile) {
            Assert-IntegerField $item.length
            Assert-IntegerField $item.lastWriteUtcTicks 1 ([datetime]::MaxValue.Ticks)
            if ($target.kind -eq 'Node' -and [IO.File]::Exists($canonical)) {
                $current = New-Object IO.FileInfo($canonical)
                $unchanged = $current.Length -eq $item.length -and $current.CreationTimeUtc.Ticks -eq $item.creationUtcTicks -and $current.LastWriteTimeUtc.Ticks -eq $item.lastWriteUtcTicks
                if ($unchanged -and -not (Test-RepositoryCacheMarker $canonical $Context.RepositoryRoot)) { throw "Node 缓存缺少本仓库标记：$canonical" }
            }
        }
      }
    }
    return $plan
}

function Assert-TargetsIdle {
    <# .SYNOPSIS 查询相关构建和浏览器进程；查询失败或仍在使用目标时拒绝执行，不结束进程。 #>
    [OutputType([void])]
    param([object[]]$Targets)
    $hasGo = @($Targets | Where-Object { $_.kind -like 'Go*' }).Count -gt 0
    $browser = @($Targets | Where-Object { $_.kind -eq 'BrowserProfile' })
    if (-not $hasGo -and $browser.Count -eq 0) { return }
    try {
        $processes = @(Get-CimInstance -ClassName Win32_Process -Filter "Name = 'go.exe' OR Name = 'compile.exe' OR Name = 'link.exe' OR Name = 'asm.exe' OR Name = 'cgo.exe' OR Name = 'chrome.exe' OR Name = 'msedge.exe' OR Name = 'chromium.exe' OR Name = 'headless_shell.exe' OR Name = 'chrome-headless-shell.exe'" -ErrorAction Stop)
    }
    catch { throw '无法可靠查询构建/浏览器进程，拒绝清理；请排查 CIM 权限或服务后重试。' }
    foreach ($process in $processes) {
        if ($hasGo -and $process.Name -in @('go.exe', 'compile.exe', 'link.exe', 'asm.exe', 'cgo.exe')) {
            throw "发现 Go 构建进程 $($process.Name)（PID $($process.ProcessId)），请等待其结束后重试。"
        }
        if ($browser.Count -gt 0 -and $process.Name -in @('chrome.exe', 'msedge.exe', 'chromium.exe', 'headless_shell.exe', 'chrome-headless-shell.exe')) {
            if (-not $process.CommandLine) { throw '存在无法读取命令行的浏览器进程，不能确认测试 profile 已关闭，拒绝清理。' }
            $name = [IO.Path]::GetFileName($browser[0].root)
            if ($process.CommandLine.IndexOf($name, [StringComparison]::OrdinalIgnoreCase) -ge 0) {
                throw "测试浏览器 $name 仍在运行（PID $($process.ProcessId)），请关闭后重试。"
            }
        }
    }
}

function Remove-PlannedCacheFile {
    <# .SYNOPSIS 只删除已经核对的单个文件，支持只读缓存并确认删除结果。 #>
    [OutputType([void])]
    param([string]$Path)
    Remove-Item -LiteralPath $Path -Force -ErrorAction Stop
    if ([IO.File]::Exists($Path) -or [IO.Directory]::Exists($Path)) { throw '删除后目标仍存在。' }
}

function Invoke-CleanupPlan {
    <# .SYNOPSIS 按已校验清单逐文件复核并清理，保留新增或变化文件，记录真实结果。 #>
    [OutputType([pscustomobject])]
    param([pscustomobject]$Plan)
    $outcomes = New-Object 'Collections.Generic.List[object]'
    $directoryOutcomes = New-Object 'Collections.Generic.List[object]'
    [long]$deletedBytes = 0
    foreach ($item in $Plan.files) {
        $status = 'missing'
        $message = '文件已不存在。'
        [long]$bytes = 0
        try {
            Assert-NoReparsePoint $item.path
            $current = New-Object IO.FileInfo($item.path)
            if ($current.Exists) {
                if ($current.Length -ne $item.length -or $current.CreationTimeUtc.Ticks -ne $item.creationUtcTicks -or
                    $current.LastWriteTimeUtc.Ticks -ne $item.lastWriteUtcTicks) {
                    $status = 'changed'; $message = '文件元数据变化，保留。'
                }
                else {
                    Remove-PlannedCacheFile $item.path
                    $status = 'deleted'; $message = '文件已删除。'; $bytes = [long]$item.length
                    $deletedBytes += $bytes
                }
            }
            elseif ([IO.Directory]::Exists($item.path)) { $status = 'changed'; $message = '原文件已变为目录，保留。' }
        }
        catch { $status = 'failed'; $message = $_.Exception.Message }
        $outcomes.Add([pscustomobject]@{
            # 原清单中的目标标识。
            targetId = $item.targetId
            # 本次处理的精确路径。
            path = $item.path
            # deleted、missing、changed 或 failed，不能将跳过计为删除。
            status = $status
            # 仅确认删除成功的文件计入逻辑字节数。
            deletedBytes = $bytes
            # 中文处理原因，不含缓存内容。
            message = $message
        })
    }
    # 从最深目录开始；非递归删除会原子拒绝含新增文件的目录。
    foreach ($item in @($Plan.directories | Sort-Object { $_.path.Length } -Descending)) {
        $status = 'missing'; $message = '目录已不存在。'
        try {
            Assert-NoReparsePoint $item.path
            $current = New-Object IO.DirectoryInfo($item.path)
            if ($current.Exists) {
                if ($current.CreationTimeUtc.Ticks -ne $item.creationUtcTicks) { $status = 'changed'; $message = '目录已重建，保留。' }
                elseif ($current.GetFileSystemInfos().Count -gt 0) { $status = 'retained'; $message = '目录非空，保留。' }
                else { [IO.Directory]::Delete($item.path, $false); $status = 'deleted'; $message = '空目录已删除。' }
            }
            elseif ([IO.File]::Exists($item.path)) { $status = 'changed'; $message = '原目录已变为文件，保留。' }
        }
        catch { $status = 'failed'; $message = $_.Exception.Message }
        $directoryOutcomes.Add([pscustomobject]@{
            # 目录所属目标。
            targetId = $item.targetId
            # 原清单目录路径。
            path = $item.path
            # 目录实际处理状态。
            status = $status
            # 中文保留或失败原因。
            message = $message
        })
    }
    return [pscustomobject]@{
        # 结果格式版本。
        schemaVersion = 1
        # 本次执行完成时间。
        completedUtc = [datetimeoffset]::UtcNow.ToString('o')
        # 仅汇总已确认删除文件的逻辑大小，不宣称实际磁盘释放量。
        deletedBytes = $deletedBytes
        # 每个计划文件的真实处理结果。
        files = $outcomes.ToArray()
        # 每个计划目录的真实处理结果。
        directories = $directoryOutcomes.ToArray()
    }
}

function Start-SessionTempCleanup {
    <# .SYNOPSIS 执行预览或消费已有清单，输出中文摘要并返回明确退出码。 #>
    [OutputType([int])]
    param([hashtable]$Options)
    $context = Get-CleanupContext
    $path = Get-ManifestFilePath $context $Options.ManifestPath
    if ($Options.Execute) {
        if ($Options.IncludeGoBuildCache -or $Options.IncludeGoModuleCache -or $Options.BrowserProfileName -or $null -ne $Options.Since) {
            throw '-Execute 只接受 -ManifestPath；请先单独预览，不得在执行时追加选择。'
        }
        $plan = Read-CleanupManifest $context $path
        $hasBuild = @($plan.targets | Where-Object { $_.kind -eq 'GoBuild' }).Count -gt 0
        $hasModule = @($plan.targets | Where-Object { $_.kind -eq 'GoModule' -or $_.kind -eq 'GoSum' }).Count -gt 0
        Assert-GoCacheConfiguration $context $hasBuild $hasModule
        Assert-TargetsIdle $plan.targets
        Write-Host ('开始按清单清理，不扩大范围：' + $path)
        $result = Invoke-CleanupPlan $plan
        $resultPath = [IO.Path]::ChangeExtension($path, ([datetime]::UtcNow.ToString('yyyyMMdd-HHmmss-fffffff') + '.result.json'))
        Write-CleanupJson $context $resultPath $result
        $deleted = @($result.files | Where-Object { $_.status -eq 'deleted' }).Count
        $missing = @($result.files | Where-Object { $_.status -eq 'missing' }).Count
        $changed = @($result.files | Where-Object { $_.status -eq 'changed' }).Count
        $failed = @($result.files | Where-Object { $_.status -eq 'failed' }).Count + @($result.directories | Where-Object { $_.status -eq 'failed' }).Count
        Write-Host ("处理完成：删除 $deleted 个文件，已不存在 $missing 个，变化保留 $changed 个，失败 $failed 项；确认删除逻辑字节 $($result.deletedBytes)。")
        Write-Host ('结果记录：' + $resultPath)
        if ($failed -gt 0) { return 1 }
        return 0
    }
    $targets = New-Object 'Collections.Generic.List[object]'
    $targets.Add((New-CleanupTarget $context 'Node'))
    if ($env:NODE_COMPILE_CACHE -and (Get-CanonicalLocalPath $env:NODE_COMPILE_CACHE) -ine $context.Roots.Node) {
        throw 'NODE_COMPILE_CACHE 已重定向，本脚本只识别默认 Node 编译缓存。'
    }
    Assert-GoCacheConfiguration $context ([bool]$Options.IncludeGoBuildCache) ([bool]$Options.IncludeGoModuleCache)
    if ($Options.IncludeGoBuildCache) {
        Write-Warning '已选择共享 Go 编译缓存，其他项目下次构建也可能需要重新编译。'
        $targets.Add((New-CleanupTarget $context 'GoBuild'))
    }
    if ($Options.IncludeGoModuleCache) {
        Write-Warning '已选择共享 Go 模块与校验缓存，下次构建可能需要联网重新下载依赖。'
        $targets.Add((New-CleanupTarget $context 'GoModule'))
        $targets.Add((New-CleanupTarget $context 'GoSum'))
    }
    if ($Options.BrowserProfileName) { $targets.Add((New-CleanupTarget $context 'BrowserProfile' $Options.BrowserProfileName)) }
    $snapshot = Get-CleanupSnapshot $context $targets.ToArray() $Options.Since
    $plan = [pscustomobject]@{
        # 清单格式版本；不兼容格式必须拒绝执行。
        schemaVersion = 1
        # 清单绑定的当前仓库绝对路径。
        repositoryRoot = $context.RepositoryRoot
        # 预览生成时间。
        createdUtc = [datetimeoffset]::UtcNow.ToString('o')
        # 可选创建时间下界，不能单独作为文件归属依据。
        sinceUtc = if ($null -ne $Options.Since) { $Options.Since.ToUniversalTime().ToString('o') } else { $null }
        # 显式选择的白名单目标。
        targets = $targets.ToArray()
        # 执行时唯一允许处理的文件列表。
        files = @($snapshot.Files)
        # 唯一允许尝试删除的空目录列表。
        directories = @($snapshot.Directories)
    }
    Write-CleanupJson $context $path $plan
    foreach ($target in $plan.targets) {
        $selected = @($plan.files | Where-Object { $_.targetId -eq $target.id })
        [long]$size = 0
        foreach ($item in $selected) { $size += $item.length }
        Write-Host ("预览 [$($target.kind)] $($target.root)：$($selected.Count) 个文件，$size 字节。")
    }
    Write-Host ('仅完成预览，未删除缓存。清单：' + $path)
    Write-Host '请审阅清单并关闭相关构建/测试浏览器后，再用 -Execute 和同一 -ManifestPath 执行。'
    return 0
}

# 点加载仅供隔离验证函数；普通调用始终从固定系统白名单建立上下文。
if ($MyInvocation.InvocationName -ne '.') {
    try {
        $options = @{
            # 是否消费既有清单。
            Execute = [bool]$Execute
            # 仓库内清单位置。
            ManifestPath = $ManifestPath
            # 共享 Go 编译缓存选择。
            IncludeGoBuildCache = [bool]$IncludeGoBuildCache
            # 共享 Go 模块缓存选择。
            IncludeGoModuleCache = [bool]$IncludeGoModuleCache
            # 精确测试浏览器目录名称。
            BrowserProfileName = $BrowserProfileName
            # 文件创建时间下界。
            Since = $Since
        }
        exit (Start-SessionTempCleanup $options)
    }
    catch { Write-Error ('清理失败：' + $_.Exception.Message) -ErrorAction Continue; exit 1 }
}
