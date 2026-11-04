#requires -Version 5.1
<#
Install the native Windows x64 Codex App plugin (preview).
No Go, Node, administrator access, PATH edits, or project trust changes.
Dot-sourcing exposes helpers for isolated tests; it never installs anything.
#>
[CmdletBinding()]
param(
    [string]$Version = '', [string]$InstallDir = '', [string]$CacheDir = '',
    [string]$CodexPath = '', [string]$FromDir = '', [switch]$Uninstall
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-Field($Value, [string]$Name, $Default = $null) {
    if ($null -ne $Value -and $null -ne $Value.PSObject.Properties[$Name]) { return $Value.$Name }
    return $Default
}
function Write-ProgressLog([string]$Message) {
    [Console]::Error.WriteLine(('Lumi [+{0:N1}s] [{1}]: {2}' -f $script:installContext.watch.Elapsed.TotalSeconds, $script:installContext.stage, $Message))
}
function Read-JsonFile([string]$Path) { return ([IO.File]::ReadAllText($Path) | ConvertFrom-Json) }
function Write-JsonFile([string]$Path, $Value) {
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    try {
        [IO.File]::WriteAllText($temporary, ($Value | ConvertTo-Json -Depth 30), (New-Object Text.UTF8Encoding($false)))
        if ([IO.File]::Exists($Path)) { [IO.File]::Replace($temporary, $Path, [System.Management.Automation.Language.NullString]::Value) }
        else { [IO.File]::Move($temporary, $Path) }
    } finally { if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) } }
}
function Get-AbsolutePath([string]$Path) {
    if ([string]::IsNullOrWhiteSpace($Path) -or -not [IO.Path]::IsPathRooted($Path)) { throw "An absolute path is required: $Path" }
    if ($Path -match '^[A-Za-z]:[^\\/]' -or $Path -match '^\\\\') { throw "Use an absolute local drive path: $Path" }
    return [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar)
}
function Test-SamePath([string]$Left, [string]$Right) {
    return [string]::Equals((Get-AbsolutePath $Left), (Get-AbsolutePath $Right), [StringComparison]::OrdinalIgnoreCase)
}
function Assert-PlainPath([string]$Path) {
    $cursor = $Path
    while ($cursor) {
        if (Test-Path -LiteralPath $cursor) {
            $item = Get-Item -LiteralPath $cursor -Force
            if (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Links/reparse points are not supported here: $cursor" }
        }
        $parent = [IO.Path]::GetDirectoryName($cursor)
        if ($parent -eq $cursor) { break }
        $cursor = $parent
    }
}
function Assert-PlainTree([string]$Path) {
    Assert-PlainPath $Path
    if (Test-Path -LiteralPath $Path -PathType Container) {
        foreach ($child in Get-ChildItem -LiteralPath $Path -Force) {
            if (($child.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "Managed tree contains a link: $($child.FullName)" }
            if ($child.PSIsContainer) { Assert-PlainTree $child.FullName }
        }
    }
}
function Remove-ManagedTree([string]$Path) {
    if (Test-Path -LiteralPath $Path) { Assert-PlainTree $Path; Remove-Item -LiteralPath $Path -Recurse -Force }
}
function Assert-NativeWindows {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) { throw 'Only native Windows Codex App is supported; WSL is not supported.' }
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if (-not $architecture) { $architecture = $env:PROCESSOR_ARCHITECTURE }
    if ($architecture -ne 'AMD64') { throw "Only native Windows x64 is supported; detected $architecture." }
}
function Find-ParentCodex {
    # When the App launches the installer, its running CLI is an ancestor. This
    # uses process metadata rather than guessing a Store/App installation path.
    $processId = $PID
    $seen = New-Object 'System.Collections.Generic.HashSet[int]'
    for ($depth = 0; $depth -lt 16 -and $processId -gt 0; $depth++) {
        if (-not $seen.Add([int]$processId)) { break }
        try { $process = Get-CimInstance Win32_Process -Filter "ProcessId = $processId" -ErrorAction Stop }
        catch { return '' }
        if ($null -eq $process) { break }
        if ($process.ExecutablePath -and [IO.Path]::GetFileName($process.ExecutablePath) -match '^codex(?:-[a-z0-9_-]+)?\.exe$') {
            return $process.ExecutablePath
        }
        $processId = [int]$process.ParentProcessId
    }
    return ''
}
function Find-Codex([string]$Explicit) {
    if ($Explicit) { $candidate = Get-AbsolutePath $Explicit }
    else {
        $parentCodex = Find-ParentCodex
        if ($parentCodex) { $candidate = Get-AbsolutePath $parentCodex }
        else {
            $candidates = @(Get-Command codex.exe -CommandType Application -All -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Source -Unique)
            if ($candidates.Count -ne 1) { throw 'Cannot select a unique native codex.exe from PATH. Supply -CodexPath with the actual App-compatible CLI path.' }
            $candidate = Get-AbsolutePath $candidates[0]
            Write-ProgressLog 'Using PATH codex; a working Codex App is still required. Use -CodexPath to select its bundled CLI.'
        }
    }
    if (-not (Test-Path -LiteralPath $candidate -PathType Leaf)) { throw "Codex executable is missing: $candidate" }
    if ([IO.Path]::GetExtension($candidate) -ne '.exe') { throw 'CodexPath must name the native codex.exe, not an npm .cmd or .ps1 launcher.' }
    return $candidate
}
function Invoke-Native([string]$Executable, [string[]]$Arguments, [int[]]$AllowedExitCodes = @(0)) {
    $errorFile = Join-Path $script:installContext.scratch ([guid]::NewGuid().ToString('N') + '.stderr')
    $previousPreference = $ErrorActionPreference
    $previousEncoding = [Console]::OutputEncoding
    try {
        # Windows PowerShell 5.1 represents native stderr as ErrorRecord; capture
        # it without turning otherwise successful native commands into failures.
        $ErrorActionPreference = 'Continue'
        [Console]::OutputEncoding = New-Object Text.UTF8Encoding($false)
        $output = & $Executable @Arguments 2> $errorFile
        $code = $LASTEXITCODE
    } finally { $ErrorActionPreference = $previousPreference; [Console]::OutputEncoding = $previousEncoding }
    $diagnostic = if (Test-Path -LiteralPath $errorFile) { [IO.File]::ReadAllText($errorFile) } else { '' }
    if ($diagnostic.Trim()) { [Console]::Error.WriteLine($diagnostic.Trim()) }
    if ($code -notin $AllowedExitCodes) { throw "Command failed ($code): $Executable $($Arguments -join ' ')`n$diagnostic" }
    return ($output -join "`n")
}
function Invoke-Codex([string[]]$Arguments) {
    $raw = Invoke-Native $script:installContext.codex $Arguments
    return ($raw | ConvertFrom-Json)
}
function Get-MarketState {
    $ctx = $script:installContext
    $list = Invoke-Codex @('plugin', 'list', '--marketplace', 'lumi-local', '--available', '--json')
    if ($null -eq $list.PSObject.Properties['installed'] -or $null -eq $list.PSObject.Properties['available']) { throw 'Invalid scoped plugin list.' }
    $registered = $false
    $installed = $false
    $enabled = $true
    $installedVersion = ''
    foreach ($entry in @($list.installed) + @($list.available)) {
        $source = Get-Field $entry 'marketplaceSource'
        if ((Get-Field $entry 'marketplaceName') -ne 'lumi-local' -or (Get-Field $source 'sourceType') -ne 'local' -or -not (Test-SamePath (Get-Field $source 'source') $ctx.market)) {
            throw 'lumi-local belongs to a different source. Use its original install directory or uninstall it first.'
        }
        $registered = $true
    }
    foreach ($entry in @($list.installed)) {
        if ((Get-Field $entry 'pluginId') -eq 'lumi@lumi-local') {
            $installed = (Get-Field $entry 'installed' $false) -eq $true
            $enabled = (Get-Field $entry 'enabled' $false) -eq $true
            $installedVersion = Get-Field $entry 'version' ''
        }
    }
    $known = $registered
    if (-not $registered) {
        try { $markets = Invoke-Codex @('plugin', 'marketplace', 'list', '--json') }
        catch {
            Write-ProgressLog 'Global marketplace metadata is unavailable; scoped Lumi lookup succeeded. Registration must enforce source ownership.'
            return [pscustomobject]@{ registered = $false; installed = $installed; enabled = $enabled; version = $installedVersion; known = $false }
        }
        if ($null -eq $markets.PSObject.Properties['marketplaces']) { throw 'Invalid marketplace list.' }
        $known = $true
        foreach ($market in @($markets.marketplaces)) {
            if ($market.name -eq 'lumi-local') {
                if (-not (Test-SamePath $market.root $ctx.market)) { throw 'lumi-local belongs to a different source.' }
                $registered = $true
            }
        }
    }
    return [pscustomobject]@{ registered = $registered; installed = $installed; enabled = $enabled; version = $installedVersion; known = $known }
}
function Receive-File([string]$Url, [string]$Destination) {
    Write-ProgressLog "Downloading $($Url.Split('/')[-1])"
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    # Check each redirect ourselves so HTTPS cannot silently downgrade.
    for ($attempt = 0; $attempt -lt 3; $attempt++) {
        try {
            $current = $Url
            for ($redirect = 0; $redirect -lt 10; $redirect++) {
                $uri = [uri]$current
                if ($uri.Scheme -ne 'https') { throw 'Downloads must remain HTTPS.' }
                $request = [Net.HttpWebRequest]::Create($uri)
                $request.AllowAutoRedirect = $false
                $request.Timeout = 300000
                $request.ReadWriteTimeout = 300000
                $response = $request.GetResponse()
                try {
                    if ([int]$response.StatusCode -in @(301,302,303,307,308)) {
                        $current = ([uri]::new($uri, $response.Headers['Location'])).AbsoluteUri
                        continue
                    }
                    if ([int]$response.StatusCode -ne 200) { throw "Unexpected HTTP status: $($response.StatusCode)" }
                    $stream = [IO.File]::Create($Destination)
                    try { $response.GetResponseStream().CopyTo($stream) } finally { $stream.Dispose() }
                    return
                } finally { $response.Dispose() }
            }
            throw 'Too many download redirects.'
        } catch { if ($attempt -eq 2) { throw }; Write-ProgressLog "Download retry: $($_.Exception.Message)" }
    }
}
function Get-ReleaseFile([string]$Name, [string]$Destination) {
    $ctx = $script:installContext
    if ($ctx.from) { Copy-Item -LiteralPath (Join-Path $ctx.from $Name) -Destination $Destination }
    else { Receive-File "https://github.com/lumikaka/lumi/releases/download/v$($ctx.version)/$Name" $Destination }
}
function Get-VerifiedAsset([string]$Name) {
    $ctx = $script:installContext
    $path = Join-Path $ctx.scratch $Name
    Get-ReleaseFile "$Name.sha256" "$path.sha256"
    $line = [IO.File]::ReadAllText("$path.sha256").Trim()
    if ($line -notmatch '^([a-fA-F0-9]{64})\s+\*?([^\r\n]+)$' -or $Matches[2] -cne $Name) { throw "Invalid SHA256 file: $Name" }
    $expected = $Matches[1].ToLowerInvariant()
    $cached = Join-Path $ctx.cache "$($ctx.version)/amd64/$expected/$Name"
    Assert-PlainPath $cached
    if (-not $ctx.from -and (Test-Path -LiteralPath $cached -PathType Leaf)) {
        Copy-Item -LiteralPath $cached -Destination $path
        if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -eq $expected) {
            Write-ProgressLog "Cache hit (SHA256 verified): $Name"
            return $expected
        }
        Write-ProgressLog "Corrupt cache, downloading again: $Name"
    }
    Get-ReleaseFile $Name $path
    if ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw "SHA256 mismatch: $Name" }
    if (-not $ctx.from) {
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($cached))
        $temporary = "$cached.$([guid]::NewGuid().ToString('N')).tmp"
        try {
            Copy-Item -LiteralPath $path -Destination $temporary
            if ([IO.File]::Exists($cached)) { [IO.File]::Replace($temporary, $cached, [System.Management.Automation.Language.NullString]::Value) }
            else { [IO.File]::Move($temporary, $cached) }
        } finally { if ([IO.File]::Exists($temporary)) { [IO.File]::Delete($temporary) } }
    }
    return $expected
}
function Expand-PluginZip([string]$Archive, [string]$Destination) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        $names = New-Object 'System.Collections.Generic.HashSet[string]' ([StringComparer]::OrdinalIgnoreCase)
        $total = [long]0
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName
            $parts = $name.TrimEnd('/').Split('/')
            $mode = ($entry.ExternalAttributes -shr 16) -band 0xF000
            if ($name -notmatch '^lumi/' -or $name.Contains('\') -or $mode -notin @(0,0x8000,0x4000) -or ($entry.ExternalAttributes -band 0x400) -ne 0) { throw "Unsafe ZIP member: $name" }
            foreach ($part in $parts) {
                if (-not $part -or $part -in @('.','..') -or $part -match '[<>:"|?*\x00-\x1f]' -or $part -match '[. ]$' -or $part -match '^(?i:con|prn|aux|nul|com[0-9]|lpt[0-9])(?:\.|$)') { throw "Unsafe ZIP path: $name" }
            }
            if (-not $names.Add($name.TrimEnd('/'))) { throw "Duplicate ZIP member: $name" }
            $total += $entry.Length
            if ($total -gt 134217728 -or $zip.Entries.Count -gt 10000) { throw 'Plugin archive exceeds the extraction limit.' }
        }
        [void][IO.Directory]::CreateDirectory($Destination)
        foreach ($entry in $zip.Entries) {
            $target = Join-Path $Destination $entry.FullName
            if ($entry.FullName.EndsWith('/')) { [void][IO.Directory]::CreateDirectory($target); continue }
            [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target))
            $inputStream = $entry.Open()
            try {
                $outputStream = [IO.File]::Open($target, [IO.FileMode]::CreateNew)
                try { $inputStream.CopyTo($outputStream) } finally { $outputStream.Dispose() }
            } finally { $inputStream.Dispose() }
        }
    } finally { $zip.Dispose() }
}
function Assert-Plugin([string]$Plugin, [string]$ExpectedCommand, [string]$ExpectedVersion = $script:installContext.version) {
    $manifest = Read-JsonFile (Join-Path $Plugin '.codex-plugin/plugin.json')
    if ($manifest.name -ne 'lumi' -or $manifest.version -ne $ExpectedVersion) { throw 'Plugin identity/version differs from release.' }
    $mcp = Read-JsonFile (Join-Path $Plugin '.mcp.json')
    $server = $mcp.mcpServers.lumi
    if ($server.command -cne $ExpectedCommand -or @($server.args).Count -ne 1 -or $server.args[0] -ne 'mcp' -or $null -ne $server.PSObject.Properties['cwd']) { throw 'Invalid MCP command, arguments or cwd.' }
    $skill = Join-Path $Plugin 'skills/create-picture-book/SKILL.md'
    $text = [IO.File]::ReadAllText($skill)
    foreach ($match in [regex]::Matches($text, '\]\((references/[^)#]+)(?:#[^)]*)?\)')) {
        $root = [IO.Path]::GetDirectoryName($skill) + [IO.Path]::DirectorySeparatorChar
        $target = [IO.Path]::GetFullPath((Join-Path $root $match.Groups[1].Value))
        if (-not $target.StartsWith($root, [StringComparison]::OrdinalIgnoreCase) -or -not [IO.File]::Exists($target)) { throw 'Missing or escaping skill reference.' }
    }
}
function Restore-Transaction {
    $ctx = $script:installContext
    if (-not (Test-Path -LiteralPath $ctx.transaction)) { return }
    $ctx.stage = 'recover'
    Assert-PlainTree $ctx.transaction
    $journalPath = Join-Path $ctx.transaction 'journal.json'
    if (-not (Test-Path -LiteralPath $journalPath)) {
        # Preparation was interrupted before any live state could be changed.
        Remove-ManagedTree $ctx.transaction
        return
    }
    $journal = Read-JsonFile $journalPath
    if ($journal.schema_version -ne 1) { throw 'Unknown transaction format; preserve the transaction for manual recovery.' }
    if ($journal.had_marketplace) {
        Remove-ManagedTree $ctx.market
        Copy-Item -LiteralPath (Join-Path $ctx.transaction 'marketplace') -Destination $ctx.market -Recurse -Force
    }
    if ($journal.registration_attempted) {
        # Never remove a marketplace unless its source is proven to be ours.
        $now = Get-MarketState
        if (-not $now.known) { throw 'Cannot prove marketplace ownership for rollback. Preserve .transaction and retry after fixing host metadata.' }
        if ($journal.market_registered) { $null = Invoke-Codex @('plugin','marketplace','add',$ctx.market,'--json') }
        if ($journal.plugin_installed) {
            $restored = Invoke-Codex @('plugin','add','lumi@lumi-local','--json')
            $oldPlugin = Join-Path $ctx.market 'plugins/lumi'
            $oldManifest = Read-JsonFile (Join-Path $oldPlugin '.codex-plugin/plugin.json')
            $oldMcp = Read-JsonFile (Join-Path $oldPlugin '.mcp.json')
            $restoredState = Get-MarketState
            if (-not $restoredState.installed -or -not $restoredState.enabled -or $restoredState.version -ne $oldManifest.version) { throw 'Restored plugin is not installed/enabled at its original version.' }
            Assert-Plugin (Get-AbsolutePath (Get-Field $restored 'installedPath')) $oldMcp.mcpServers.lumi.command $oldManifest.version
        }
        elseif ($now.installed) { $null = Invoke-Codex @('plugin','remove','lumi@lumi-local','--json') }
        if (-not $journal.market_registered -and $now.registered) { $null = Invoke-Codex @('plugin','marketplace','remove','lumi-local','--json') }
    }
    if (-not $journal.had_marketplace) { Remove-ManagedTree $ctx.market }
    if ($journal.had_state) { Write-JsonFile $ctx.state (Read-JsonFile (Join-Path $ctx.transaction 'state.json')) }
    elseif (Test-Path -LiteralPath $ctx.state) { Remove-Item -LiteralPath $ctx.state -Force }
    Remove-ManagedTree $ctx.transaction
    Write-ProgressLog 'Previous Lumi payload and registration restored.'
}
function Remove-OldVersions([string[]]$Keep = @()) {
    $ctx = $script:installContext
    $pending = @()
    foreach ($directory in @(Get-ChildItem -LiteralPath $ctx.versions -Directory -ErrorAction SilentlyContinue)) {
        if ($directory.Name -in $Keep) { continue }
        try {
            Assert-PlainTree $directory.FullName
            # Delete the executable first. Windows refuses while it is running;
            # leave the rest of that version (including its uninstaller) intact.
            $exe = Join-Path $directory.FullName 'lumi.exe'
            if ([IO.File]::Exists($exe)) { [IO.File]::Delete($exe) }
            Remove-ManagedTree $directory.FullName
        } catch { $pending += $directory.FullName; Write-ProgressLog "Version retained: $($directory.FullName): $($_.Exception.Message)" }
    }
    return $pending
}
function Invoke-LumiInstall {
    param([string]$ReleaseVersion, [string]$Destination, [string]$DownloadCache, [string]$CodexExecutable, [string]$LocalAssets, [switch]$Remove)
    $script:installContext = @{ stage = 'preflight'; watch = [Diagnostics.Stopwatch]::StartNew(); scratch = ''; codex = ''; recoveryAllowed = $false }
    $ctx = $script:installContext
    $lock = $null
    try {
        Assert-NativeWindows
        if (-not $Destination) { $Destination = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'LumiCodex' }
        if (-not $DownloadCache) { $DownloadCache = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'LumiCodexCache' }
        $ctx.root = Get-AbsolutePath $Destination
        $ctx.cache = Get-AbsolutePath $DownloadCache
        $ctx.codexHome = if ($env:CODEX_HOME) { Get-AbsolutePath $env:CODEX_HOME } else { Join-Path ([Environment]::GetFolderPath('UserProfile')) '.codex' }
        # Commands inherit an existing CODEX_HOME. The host uses its default when
        # unset, so the installer does not repurpose this environment variable.
        foreach ($path in @($ctx.root, $ctx.cache)) { Assert-PlainPath $path }
        if ($ctx.root -eq [IO.Path]::GetPathRoot($ctx.root).TrimEnd([IO.Path]::DirectorySeparatorChar) -or (Test-SamePath $ctx.root ([Environment]::GetFolderPath('UserProfile'))) -or (Test-SamePath $ctx.root $ctx.codexHome)) { throw 'Use a dedicated installation directory.' }
        $ctx.market = Join-Path $ctx.root 'marketplace'
        $ctx.state = Join-Path $ctx.root 'install-state.json'
        $ctx.transaction = Join-Path $ctx.root '.transaction'
        $ctx.versions = Join-Path $ctx.root 'versions'
        $marker = Join-Path $ctx.root '.lumi-codex-install'
        $ctx.scratch = Join-Path ([IO.Path]::GetTempPath()) ('lumi-codex-' + [guid]::NewGuid().ToString('N'))
        [void][IO.Directory]::CreateDirectory($ctx.scratch)
        [void][IO.Directory]::CreateDirectory($ctx.root)
        Assert-PlainPath (Join-Path $ctx.root '.install.lock')
        try { $lock = [IO.File]::Open((Join-Path $ctx.root '.install.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None) }
        catch { throw 'Another installation is running; retry after it exits.' }
        if (Test-Path -LiteralPath $marker) {
            if (-not (Test-SamePath ([IO.File]::ReadAllText($marker).Trim()) $ctx.codexHome)) { throw 'This installation belongs to a different CODEX_HOME.' }
        } elseif (@(Get-ChildItem -LiteralPath $ctx.root -Force | Where-Object Name -ne '.install.lock').Count) { throw 'Refusing a non-empty directory not owned by this installer.' }
        elseif ($Remove) { return @{ installed = $false; pending_cleanup = @(); new_chat_required = $true } }
        Assert-PlainTree $ctx.root
        $ctx.codex = Find-Codex $CodexExecutable
        $ctx.codexVersion = (Invoke-Native $ctx.codex @('--version')).Trim()
        Write-ProgressLog "Codex: $($ctx.codex) ($($ctx.codexVersion)); CODEX_HOME: $($ctx.codexHome)"
        foreach ($arguments in @(@('plugin','add','--help'), @('plugin','remove','--help'), @('plugin','marketplace','add','--help'), @('plugin','marketplace','remove','--help'), @('plugin','list','--help'))) { $null = Invoke-Native $ctx.codex $arguments }
        $ctx.recoveryAllowed = $true
        Restore-Transaction
        $before = Get-MarketState
        if (-not $Remove -and $before.installed -and -not $before.enabled) { throw 'Lumi is disabled. Resolve the host policy or enable it before upgrading.' }
        if (-not [IO.File]::Exists($marker)) { [IO.File]::WriteAllText($marker, $ctx.codexHome, (New-Object Text.UTF8Encoding($false))) }
        [void][IO.Directory]::CreateDirectory($ctx.versions)
        if ($Remove) {
            $ctx.stage = 'uninstall'
            if (-not $before.known) { throw 'Cannot verify ownership of an empty marketplace for uninstall.' }
            if ($before.installed) { $null = Invoke-Codex @('plugin','remove','lumi@lumi-local','--json') }
            if ($before.registered) { $null = Invoke-Codex @('plugin','marketplace','remove','lumi-local','--json') }
            $after = Get-MarketState
            if ($after.installed -or $after.registered) { throw 'Host still reports Lumi registered after uninstall.' }
            Remove-ManagedTree $ctx.market
            $pending = @(Remove-OldVersions)
            if (Test-Path -LiteralPath $ctx.state) { Remove-Item -LiteralPath $ctx.state -Force }
            # Keep the ownership marker so a retry can clean an occupied exe.
            Write-ProgressLog 'Plugin removed. Projects, application data and cache preserved. Close old chats; pending files can be removed by running uninstall again.'
            return @{ installed = $false; enabled = $false; pending_cleanup = $pending; new_chat_required = $true }
        }
        $ctx.stage = 'download'
        $ctx.from = if ($LocalAssets) { Get-AbsolutePath $LocalAssets } else { '' }
        $ctx.version = $ReleaseVersion -replace '^v',''
        if (-not $ctx.version) {
            $versionFile = Join-Path $ctx.scratch 'version.txt'
            if ($ctx.from) { Copy-Item -LiteralPath (Join-Path $ctx.from 'lumi-codex-version.txt') -Destination $versionFile }
            else { Receive-File 'https://github.com/lumikaka/lumi/releases/latest/download/lumi-codex-version.txt' $versionFile }
            $ctx.version = [IO.File]::ReadAllText($versionFile).Trim()
        }
        if ($ctx.version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$') { throw 'Invalid release version.' }
        $binaryHash = Get-VerifiedAsset 'lumi-windows-amd64.exe'
        $pluginHash = Get-VerifiedAsset 'lumi-codex-plugin.zip'
        $installerHash = Get-VerifiedAsset 'install-codex.ps1'
        if ((Get-FileHash -LiteralPath $script:installerSource -Algorithm SHA256).Hash.ToLowerInvariant() -ne $installerHash) { throw 'Run the installer from the selected release; script checksum differs.' }
        $ctx.stage = 'validate'
        $stagedMarket = Join-Path $ctx.scratch 'marketplace'
        Expand-PluginZip (Join-Path $ctx.scratch 'lumi-codex-plugin.zip') (Join-Path $stagedMarket 'plugins')
        $plugin = Join-Path $stagedMarket 'plugins/lumi'
        Assert-Plugin $plugin '__LUMI_ABSOLUTE_BINARY__'
        $sha = [Security.Cryptography.SHA256]::Create()
        try { $digest = ([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes("$binaryHash$pluginHash$installerHash")))).Replace('-','').ToLowerInvariant() }
        finally { $sha.Dispose() }
        $revision = "$($ctx.version)-$digest"
        $versionDir = Join-Path $ctx.versions $revision
        $binary = Join-Path $versionDir 'lumi.exe'
        if (Test-Path -LiteralPath $versionDir) {
            if ((Get-FileHash -LiteralPath $binary -Algorithm SHA256).Hash.ToLowerInvariant() -ne $binaryHash -or (Get-FileHash -LiteralPath (Join-Path $versionDir 'install-codex.ps1') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $installerHash) { throw 'Existing immutable version is corrupt; preserve it and repair after its processes exit.' }
        } else {
            $stagedVersion = Join-Path $ctx.root ('.version-' + [guid]::NewGuid().ToString('N'))
            try {
                [void][IO.Directory]::CreateDirectory($stagedVersion)
                Copy-Item -LiteralPath (Join-Path $ctx.scratch 'lumi-windows-amd64.exe') -Destination (Join-Path $stagedVersion 'lumi.exe')
                Copy-Item -LiteralPath (Join-Path $ctx.scratch 'install-codex.ps1') -Destination $stagedVersion
                [IO.Directory]::Move($stagedVersion, $versionDir)
            } finally { Remove-ManagedTree $stagedVersion }
        }
        $null = Invoke-Native $binary @('--help')
        $mcpPath = Join-Path $plugin '.mcp.json'
        $mcp = Read-JsonFile $mcpPath
        $mcp.mcpServers.lumi.command = $binary
        Write-JsonFile $mcpPath $mcp
        $catalog = Join-Path $stagedMarket '.agents/plugins'
        [void][IO.Directory]::CreateDirectory($catalog)
        Write-JsonFile (Join-Path $catalog 'marketplace.json') @{
            name = 'lumi-local'; interface = @{ displayName = 'Lumi Local' }
            plugins = @(@{ name = 'lumi'; source = @{ source = 'local'; path = './plugins/lumi' }; policy = @{ installation = 'AVAILABLE'; authentication = 'ON_INSTALL' }; category = 'Productivity' })
        }
        $ctx.stage = 'install'
        [void][IO.Directory]::CreateDirectory($ctx.transaction)
        $oldState = if (Test-Path -LiteralPath $ctx.state) { Read-JsonFile $ctx.state } else { $null }
        $journal = [pscustomobject]@{ schema_version = 1; had_marketplace = (Test-Path -LiteralPath $ctx.market); had_state = ($null -ne $oldState); market_registered = $before.registered; plugin_installed = $before.installed; registration_attempted = $false }
        if ($journal.had_marketplace) { Copy-Item -LiteralPath $ctx.market -Destination (Join-Path $ctx.transaction 'marketplace') -Recurse -Force }
        if ($journal.had_state) { Write-JsonFile (Join-Path $ctx.transaction 'state.json') $oldState }
        Write-JsonFile (Join-Path $ctx.transaction 'journal.json') $journal
        Remove-ManagedTree $ctx.market
        Copy-Item -LiteralPath $stagedMarket -Destination $ctx.market -Recurse -Force
        $journal.registration_attempted = $true
        Write-JsonFile (Join-Path $ctx.transaction 'journal.json') $journal
        $null = Invoke-Codex @('plugin','marketplace','add',$ctx.market,'--json')
        $added = Invoke-Codex @('plugin','add','lumi@lumi-local','--json')
        $ctx.stage = 'verify'
        $after = Get-MarketState
        if (-not $after.installed -or -not $after.enabled -or $after.version -ne $ctx.version) { throw 'Lumi is not installed and enabled at the release version; check host policy.' }
        $installedPath = Get-AbsolutePath (Get-Field $added 'installedPath')
        Assert-Plugin $installedPath $binary
        $previous = Get-Field $oldState 'revision' ''
        if ($previous -eq $revision) { $previous = Get-Field $oldState 'previous_revision' '' }
        Write-JsonFile $ctx.state @{ schema_version = 1; codex_home = $ctx.codexHome; revision = $revision; previous_revision = $previous; lumi_version = $ctx.version; cli_path = $binary; installed_path = $installedPath }
        Remove-ManagedTree $ctx.transaction
        $pending = @(Remove-OldVersions @($revision, $previous))
        $serverRunning = $null
        try { $status = (Invoke-Native $binary @('status','--read-only','--json') @(0,3)) | ConvertFrom-Json; if ($status.success) { $serverRunning = $status.data.running } }
        catch { Write-ProgressLog 'Unable to inspect the existing server; installation is complete. Check lumi status before restarting shared work.' }
        $ctx.stage = 'complete'
        Write-ProgressLog "Installed lumi@lumi-local $($ctx.version). Open a NEW App chat. Existing servers are not stopped; coordinate lumi stop if an upgrade is required."
        return @{ plugin_id = 'lumi@lumi-local'; lumi_version = $ctx.version; codex_path = $ctx.codex; codex_version = $ctx.codexVersion; codex_home = $ctx.codexHome; cli_path = $binary; installed_path = $installedPath; installed = $true; enabled = $true; new_chat_required = $true; server_running = $serverRunning; pending_cleanup = $pending }
    } catch {
        $failure = $_.Exception.Message
        $failedStage = $ctx.stage
        if ($null -ne $lock -and $ctx.recoveryAllowed -and (Test-Path -LiteralPath $ctx.transaction)) {
            try { Restore-Transaction } catch { $failure += "`nRollback pending: $($_.Exception.Message). Preserve .transaction and retry with the same parameters." }
        }
        $ctx.stage = $failedStage
        throw $failure
    } finally {
        if ($null -ne $lock) { $lock.Dispose() }
        if ($ctx.scratch -and (Test-Path -LiteralPath $ctx.scratch)) { Remove-Item -LiteralPath $ctx.scratch -Recurse -Force -ErrorAction SilentlyContinue }
    }
}

$script:installerSource = $PSCommandPath
if ($MyInvocation.InvocationName -ne '.') {
    try {
        $data = Invoke-LumiInstall -ReleaseVersion $Version -Destination $InstallDir -DownloadCache $CacheDir -CodexExecutable $CodexPath -LocalAssets $FromDir -Remove:$Uninstall
        @{ success = $true; data = $data } | ConvertTo-Json -Depth 20
        exit 0
    } catch {
        $message = $_.Exception.Message
        [Console]::Error.WriteLine($message)
        @{ success = $false; data = $null; error = @{ code = 'installation_failed'; message = $message; details = $script:installContext.stage } } | ConvertTo-Json -Depth 10
        exit 1
    }
}
