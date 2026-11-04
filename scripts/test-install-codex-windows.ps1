#requires -Version 5.1
param([Parameter(Mandatory = $true)][string]$Assets)
$ErrorActionPreference = 'Stop'
$testAssets = [IO.Path]::GetFullPath($Assets)
# Exercise the exact installer bytes shipped with the selected release assets.
. (Join-Path $testAssets 'install-codex.ps1')
$testTemp = if (Test-Path -LiteralPath '/private/tmp') { '/private/tmp' } else { [IO.Path]::GetTempPath() }
$testRoot = Join-Path $testTemp ('lumi-installer-test-' + [guid]::NewGuid().ToString('N'))
$originalCodexHome = $env:CODEX_HOME
$script:sourceAssets = $testAssets
$script:downloads = 0
$script:failAdd = 0
$script:disableNext = $false
$script:globalBroken = $true
$script:targetBroken = $false
$script:marketRoot = ''
$script:cachedPlugin = ''
$script:fixtureInstalled = $false
$script:fixtureEnabled = $true
$script:blockedDirectory = ''
$realDelete = ${function:Remove-ManagedTree}
$realNative = ${function:Invoke-Native}
$script:checks = 0
function Assert($Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Pass([string]$Message) { $script:checks++; Write-Host "PASS: $Message" }
function Assert-NativeWindows {} # Only the test process bypasses the OS guard.
function Find-Codex([string]$Explicit) { return (Join-Path $testRoot 'fixture-codex.exe') }
function Invoke-Native([string]$Executable, [string[]]$Arguments, [int[]]$AllowedExitCodes = @(0)) {
    if ($Arguments[0] -eq '--version') { return 'codex fixture 1.0' }
    if ($Arguments[0] -eq 'status') { return '{"success":true,"data":{"running":false}}' }
    return 'fixture help'
}
function Receive-File([string]$Url, [string]$Destination) {
    $script:downloads++
    Copy-Item -LiteralPath (Join-Path $script:sourceAssets ($Url.Split('/')[-1])) -Destination $Destination
}
function Remove-ManagedTree([string]$Path) {
    if ($script:blockedDirectory -and (Test-SamePath $Path $script:blockedDirectory)) { throw 'Simulated occupied version' }
    & $realDelete $Path
}
function Invoke-Codex([string[]]$Arguments) {
    if (($Arguments -join ' ') -eq 'plugin marketplace list --json') {
        if ($script:globalBroken) { throw 'Unrelated marketplace broken' }
        $rows = @()
        if ($script:marketRoot) { $rows += @{ name = 'lumi-local'; root = $script:marketRoot } }
        return ([pscustomobject]@{ marketplaces = $rows })
    }
    if ($Arguments[1] -eq 'list') {
        if ($script:targetBroken) { throw 'Target marketplace unreadable' }
        $installed = @(); $available = @()
        if ($script:marketRoot) {
            $manifest = Join-Path $script:marketRoot 'plugins/lumi/.codex-plugin/plugin.json'
            if (Test-Path -LiteralPath $manifest) {
                $version = (Read-JsonFile $manifest).version
                $entry = [pscustomobject]@{ marketplaceName = 'lumi-local'; marketplaceSource = [pscustomobject]@{ sourceType = 'local'; source = $script:marketRoot }; pluginId = 'lumi@lumi-local'; version = $version; installed = $script:fixtureInstalled; enabled = $script:fixtureEnabled }
                $available += $entry
                if ($script:fixtureInstalled) { $installed += $entry }
            }
        }
        return ([pscustomobject]@{ installed = $installed; available = $available })
    }
    if ($Arguments[1] -eq 'marketplace' -and $Arguments[2] -eq 'add') {
        if ($script:marketRoot -and -not (Test-SamePath $script:marketRoot $Arguments[3])) { throw 'Marketplace collision' }
        $script:marketRoot = $Arguments[3]
        return ([pscustomobject]@{ alreadyAdded = $false })
    }
    if ($Arguments[1] -eq 'marketplace' -and $Arguments[2] -eq 'remove') { $script:marketRoot = ''; return ([pscustomobject]@{}) }
    if ($Arguments[1] -eq 'remove') { $script:fixtureInstalled = $false; return ([pscustomobject]@{}) }
    if ($Arguments[1] -eq 'add') {
        if ($script:failAdd -gt 0) { $script:failAdd--; throw 'Injected plugin add failure' }
        $source = Join-Path $script:marketRoot 'plugins/lumi'
        $script:cachedPlugin = Join-Path $testRoot 'host cache/lumi'
        if (Test-Path -LiteralPath $script:cachedPlugin) { Remove-Item -LiteralPath $script:cachedPlugin -Recurse -Force }
        [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($script:cachedPlugin))
        Copy-Item -LiteralPath $source -Destination $script:cachedPlugin -Recurse -Force
        $script:fixtureInstalled = $true
        $script:fixtureEnabled = -not $script:disableNext
        $script:disableNext = $false
        return ([pscustomobject]@{ installedPath = $script:cachedPlugin })
    }
    throw "Unexpected fixture command: $Arguments"
}
function Install-Test([string]$Local = '', [switch]$Remove, [string]$Root = '') {
    if (-not $Root) { $Root = $script:installRoot }
    return Invoke-LumiInstall -Destination $Root -DownloadCache (Join-Path $testRoot 'download cache') -LocalAssets $Local -Remove:$Remove
}
function Expect-Failure([scriptblock]$Action, [string]$Pattern) {
    try { $null = & $Action } catch {
        Assert ($_.Exception.Message -match $Pattern) "Expected '$Pattern', got $($_.Exception.Message)"
        return
    }
    throw "Expected failure: $Pattern"
}
function Copy-Assets([string]$Name) {
    $target = Join-Path $testRoot $Name
    Copy-Item -LiteralPath $testAssets -Destination $target -Recurse -Force
    return $target
}
function Rehash([string]$Directory, [string]$Name) {
    $digest = (Get-FileHash -LiteralPath (Join-Path $Directory $Name) -Algorithm SHA256).Hash.ToLowerInvariant()
    [IO.File]::WriteAllText((Join-Path $Directory "$Name.sha256"), "$digest  $Name`n")
}
function Change-Zip([string]$Directory, [string]$Member, [string]$Content) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::Open((Join-Path $Directory 'lumi-codex-plugin.zip'), [IO.Compression.ZipArchiveMode]::Update)
    try {
        $entry = $zip.GetEntry($Member)
        if ($null -ne $entry) { $entry.Delete() }
        $entry = $zip.CreateEntry($Member)
        $writer = New-Object IO.StreamWriter($entry.Open(), (New-Object Text.UTF8Encoding($false)))
        try { $writer.Write($Content) } finally { $writer.Dispose() }
    } finally { $zip.Dispose() }
    Rehash $Directory 'lumi-codex-plugin.zip'
}
try {
    [void][IO.Directory]::CreateDirectory($testRoot)
    $script:installContext = @{ scratch = $testRoot }
    $currentShell = (Get-Process -Id $PID).Path
    # Windows PowerShell 5.1 strips embedded double quotes from native arguments.
    # Encode the probe as UTF-16LE so both shells execute the same source text.
    $nativeProbe = '[Console]::OutputEncoding = New-Object Text.UTF8Encoding($false); [Console]::Error.WriteLine("diagnostic"); [Console]::WriteLine([char]0x7ed8); exit 0'
    $encodedProbe = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($nativeProbe))
    # This probe writes raw console text; prevent Windows PowerShell from
    # selecting CLIXML for the nested shell's redirected error stream.
    $nativeText = & $realNative $currentShell @('-NoProfile','-NonInteractive','-OutputFormat','Text','-EncodedCommand', $encodedProbe)
    Assert ($nativeText.Trim() -eq [string][char]0x7ed8) 'Native UTF-8 stdout was corrupted or mixed with stderr'
    $nativeErrorFile = Get-ChildItem -LiteralPath $testRoot -Filter '*.stderr' -File | Select-Object -First 1
    Assert ($null -ne $nativeErrorFile -and [IO.File]::ReadAllText($nativeErrorFile.FullName) -match 'diagnostic') 'Native stderr diagnostic was lost'
    Expect-Failure { & $realNative $currentShell @('-NoProfile','-NonInteractive','-Command','exit 7') } 'Command failed \(7\)'
    Pass 'native subprocess preserves UTF-8, separates stderr and checks exit status'
    $env:CODEX_HOME = Join-Path $testRoot 'codex configuration'
    [void][IO.Directory]::CreateDirectory($env:CODEX_HOME)
    $sentinel = Join-Path $env:CODEX_HOME 'config.toml'
    [IO.File]::WriteAllText($sentinel, '# unrelated configuration must survive')
    $unicodeName = 'install ' + [char]0x7ed8 + [char]0x672c
    $script:installRoot = Join-Path $testRoot $unicodeName
    $result = Install-Test
    Assert ($result.installed -and $result.enabled) 'Install did not succeed'
    $first = $result.cli_path
    Assert ((Read-JsonFile (Join-Path $script:cachedPlugin '.mcp.json')).mcpServers.lumi.command -eq $first) 'Wrong cached command'
    Pass 'fresh install, Unicode/space paths, scoped lookup tolerates unrelated broken marketplace'
    $count = $script:downloads
    $again = Install-Test
    Assert ($again.cli_path -eq $first -and $script:downloads -eq $count + 4) 'Expected version + checksums only on cache hit'
    Pass 'same version is immutable and reuses verified cached assets'
    $corruptCache = Get-ChildItem -LiteralPath (Join-Path $testRoot 'download cache') -Recurse -File | Where-Object Name -eq 'lumi-windows-amd64.exe' | Select-Object -First 1
    [IO.File]::WriteAllText($corruptCache.FullName, 'corrupt')
    $count = $script:downloads
    $null = Install-Test
    Assert ($script:downloads -eq $count + 5) 'Corrupt cache was not downloaded again'
    Pass 'cache corruption triggers fresh download and verification'
    $bad = Copy-Assets 'bad'
    [IO.File]::WriteAllText((Join-Path $bad 'lumi-windows-amd64.exe'), 'corrupt')
    Expect-Failure { Install-Test $bad } 'SHA256 mismatch'
    Assert ((Read-JsonFile (Join-Path $script:installRoot 'install-state.json')).cli_path -eq $first) 'Checksum failure changed current version'
    Pass 'offline assets are verified even when a valid cache exists'
    $differentInstaller = Copy-Assets 'different-installer'
    [IO.File]::AppendAllText((Join-Path $differentInstaller 'install-codex.ps1'), "`n# different release installer`n")
    Rehash $differentInstaller 'install-codex.ps1'
    Expect-Failure { Install-Test $differentInstaller } 'script checksum differs'
    Assert ((Read-JsonFile (Join-Path $script:installRoot 'install-state.json')).cli_path -eq $first) 'Installer mismatch changed current version'
    Pass 'installer from a different release is rejected even with a valid asset checksum'
    $count = $script:downloads
    Expect-Failure { Install-Test -Root (Join-Path $testRoot 'other install') } 'different source'
    Assert ($script:downloads -eq $count) 'Collision downloaded assets'
    $script:targetBroken = $true
    Expect-Failure { Install-Test } 'Target marketplace unreadable'
    $script:targetBroken = $false
    Pass 'foreign and broken target marketplaces fail before downloads'
    $upgrade = Copy-Assets 'upgrade'
    [IO.File]::AppendAllText((Join-Path $upgrade 'lumi-windows-amd64.exe'), 'different test binary')
    Rehash $upgrade 'lumi-windows-amd64.exe'
    $script:failAdd = 1
    Expect-Failure { Install-Test $upgrade } 'Injected plugin add failure'
    Assert ((Read-JsonFile (Join-Path $script:cachedPlugin '.mcp.json')).mcpServers.lumi.command -eq $first) 'Rollback did not restore cached command'
    Assert (-not (Test-Path -LiteralPath (Join-Path $script:installRoot '.transaction'))) 'Rollback journal not cleared'
    Pass 'registration failure restores original marketplace, state and host cache'
    $script:failAdd = 2
    Expect-Failure { Install-Test $upgrade } 'Rollback pending'
    Assert (Test-Path -LiteralPath (Join-Path $script:installRoot '.transaction/journal.json')) 'Recovery evidence was lost'
    $null = Install-Test $testAssets
    Assert (-not (Test-Path -LiteralPath (Join-Path $script:installRoot '.transaction'))) 'Next run did not recover'
    Pass 'failed rollback preserves journal; next run recovers before proceeding'
    $script:disableNext = $true
    Expect-Failure { Install-Test $upgrade } 'not installed and enabled'
    Assert $script:fixtureEnabled 'Rollback did not restore enabled old plugin'
    $result = Install-Test $upgrade
    Assert ($result.cli_path -ne $first -and (Test-Path -LiteralPath $first)) 'Upgrade overwrote old executable'
    Pass 'policy failure rolls back; successful upgrade retains prior executable'
    if ([Environment]::OSVersion.Platform -eq [PlatformID]::Win32NT) {
        $occupied = [IO.File]::Open($first, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
        try {
            $third = Copy-Assets 'third-version'
            [IO.File]::AppendAllText((Join-Path $third 'lumi-windows-amd64.exe'), 'third test binary')
            Rehash $third 'lumi-windows-amd64.exe'
            $result = Install-Test $third
            Assert ((Test-Path -LiteralPath $first) -and @($result.pending_cleanup).Count -gt 0) 'Occupied old executable was not retained'
        } finally { $occupied.Dispose() }
        Pass 'real Windows file handle prevents deletion during upgrade without failing installation'
    }
    $statePath = Join-Path $script:installRoot 'install-state.json'
    $lockPath = Join-Path $script:installRoot '.install.lock'
    $held = [IO.File]::Open($lockPath, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
    try { Expect-Failure { Install-Test } 'Another installation' } finally { $held.Dispose() }
    $env:CODEX_HOME = Join-Path $testRoot 'other codex home'
    Expect-Failure { Install-Test } 'different CODEX_HOME'
    $env:CODEX_HOME = Join-Path $testRoot 'codex configuration'
    Pass 'exclusive lock and installation ownership reject conflicting callers'
    foreach ($member in @('lumi/../escape', 'lumi/CON.txt', 'lumi/path:stream', 'lumi/trailing.')) {
        $malicious = Copy-Assets ('unsafe-' + [guid]::NewGuid().ToString('N'))
        Change-Zip $malicious $member 'bad'
        Expect-Failure { Install-Test $malicious } 'Unsafe ZIP'
    }
    Pass 'ZIP traversal, device names, ADS and ambiguous Windows names rejected'
    $duplicate = Copy-Assets 'duplicate'
    Change-Zip $duplicate 'lumi/.MCP.json' '{}'
    Expect-Failure { Install-Test $duplicate } 'Duplicate ZIP'
    $mismatch = Copy-Assets 'mismatch'
    [IO.File]::WriteAllText((Join-Path $mismatch 'lumi-codex-version.txt'), '999.0.0')
    Expect-Failure { Install-Test $mismatch } 'version differs'
    Pass 'case-insensitive duplicate archive names and mixed release versions rejected'
    $foreign = Join-Path $testRoot 'unowned'
    [void][IO.Directory]::CreateDirectory((Join-Path $foreign '.transaction'))
    [IO.File]::WriteAllText((Join-Path $foreign '.transaction/journal.json'), '{}')
    Expect-Failure { Install-Test -Root $foreign } 'not owned'
    Assert ([IO.File]::Exists((Join-Path $foreign '.transaction/journal.json'))) 'Unowned transaction was touched'
    Pass 'unowned installation cannot trigger recovery or cleanup'
    $script:blockedDirectory = [IO.Path]::GetDirectoryName($result.cli_path)
    $removed = Install-Test -Remove
    Assert (-not $removed.installed -and @($removed.pending_cleanup).Count -gt 0) 'Occupied files were not reported'
    $script:globalBroken = $false
    $script:blockedDirectory = ''
    $removed = Install-Test -Remove
    Assert (@($removed.pending_cleanup).Count -eq 0) 'Retry did not clean pending files'
    $null = Install-Test -Remove
    Assert ([IO.File]::ReadAllText($sentinel) -eq '# unrelated configuration must survive') 'Unrelated config was modified'
    Pass 'uninstall reports pending files, retries safely and preserves unrelated configuration'
    $script:failAdd = 1
    Expect-Failure { Install-Test $testAssets } 'Injected plugin add failure'
    Assert (-not $script:fixtureInstalled -and -not $script:marketRoot) 'Fresh install failure left registration behind'
    Pass 'failed first install removes only its own new registration'
    Write-Host "$script:checks installer scenarios passed (fixture host; not App acceptance)."
} finally {
    $env:CODEX_HOME = $originalCodexHome
    if (Test-Path -LiteralPath $testRoot) { Remove-Item -LiteralPath $testRoot -Recurse -Force }
}
