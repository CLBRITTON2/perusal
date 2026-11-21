<#
.SYNOPSIS
Builds perusal for Windows, Linux and macOS and publishes the builds as the GitHub release for an existing tag, with
the tag's RELEASENOTES.md section as its notes.

.DESCRIPTION
Go stamps the version `perusal -version` prints from the checked-out tag, so the checkout must be clean and at the
tag.

.PARAMETER Tag
The pushed tag, `v` then the version, as v0.2.0.

.PARAMETER Token
A GitHub token that can write releases: the workflow's GITHUB_TOKEN, or `gh auth token` locally.

.EXAMPLE
.\scripts\release.ps1 -Tag v0.2.0 -Token (gh auth token)
#>
param(
    [Parameter(Mandatory)][string]$Tag,
    [Parameter(Mandatory)][string]$Token
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')

if ($Tag -notmatch '^v(\d+\.\d+\.\d+)$') {
    throw "tag $Tag is not v<major>.<minor>.<patch>"
}
$version = $Matches[1]
$headTag = git -C $root describe --tags --exact-match HEAD
if ($LASTEXITCODE -ne 0 -or $headTag -ne $Tag) {
    throw "HEAD of $root is not at tag $Tag (git describe gave '$headTag')"
}
$releaseNotes = Join-Path $root 'RELEASENOTES.md'
$lines = Get-Content $releaseNotes
$heading = "## [$version]"
$start = [array]::FindIndex([string[]]$lines, [Predicate[string]] { param([string]$line) $line.StartsWith($heading) })
if ($start -lt 0) {
    throw "$releaseNotes has no '$heading' section"
}
$end = [array]::FindIndex([string[]]$lines, $start + 1, [Predicate[string]] { param([string]$line) $line.StartsWith('## ') })
if ($end -lt 0) {
    $end = $lines.Count
}
$notes = ($lines[($start + 1)..($end - 1)] -join "`n").Trim()
if (-not $notes) {
    throw "the '$heading' section of $releaseNotes is empty"
}

$package = Join-Path $root 'build\release'
if (Test-Path $package) {
    Remove-Item -Recurse $package
}
New-Item -ItemType Directory $package | Out-Null
$targets = @(
    @{ OS = 'windows'; Arch = 'amd64' }, @{ OS = 'windows'; Arch = 'arm64' },
    @{ OS = 'linux'; Arch = 'amd64' }, @{ OS = 'linux'; Arch = 'arm64' },
    @{ OS = 'darwin'; Arch = 'amd64' }, @{ OS = 'darwin'; Arch = 'arm64' }
)
$archives = foreach ($target in $targets) {
    $name = "perusal-$Tag-$($target.OS)-$($target.Arch)"
    $staging = Join-Path $package $name
    New-Item -ItemType Directory $staging | Out-Null
    $exe = if ($target.OS -eq 'windows') { 'perusal.exe' } else { 'perusal' }
    $env:GOOS = $target.OS
    $env:GOARCH = $target.Arch
    $env:CGO_ENABLED = '0'
    go -C $root build -trimpath -o (Join-Path $staging $exe) ./cmd/perusal
    if ($LASTEXITCODE -ne 0) {
        throw "go build for $($target.OS)/$($target.Arch) failed with exit code $LASTEXITCODE"
    }
    Copy-Item -Path (Join-Path $root 'LICENSE'), (Join-Path $root 'README.md') -Destination $staging
    if ($target.OS -eq 'windows') {
        $archive = Join-Path $package "$name.zip"
        Compress-Archive -Path (Join-Path $staging '*') -DestinationPath $archive
    } else {
        $archive = Join-Path $package "$name.tar.gz"
        # NTFS has no executable bit, so an mtree spec gives each entry its Unix mode.
        $spec = Join-Path $package "$name.mtree"
        $entries = @{ $exe = '0755'; 'LICENSE' = '0644'; 'README.md' = '0644' }
        $specLines = @('#mtree') + ($entries.Keys | ForEach-Object {
                $contents = (Join-Path $staging $_) -replace '\\', '/'
                "$_ type=file mode=$($entries[$_]) uname=root gname=root contents=$contents"
            })
        Set-Content -Path $spec -Value $specLines
        # Windows' own bsdtar: a GNU tar earlier on PATH, as Git's, reads the C: in an archive path as a remote host.
        & (Join-Path $env:SystemRoot 'System32\tar.exe') -czf $archive "@$spec"
        if ($LASTEXITCODE -ne 0) {
            throw "tar for $name failed with exit code $LASTEXITCODE"
        }
    }
    $archive
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED

$hostExe = Join-Path $package "perusal-$Tag-windows-amd64\perusal.exe"
$stamped = & $hostExe -version
if ($stamped -ne $Tag) {
    throw "$hostExe -version printed '$stamped', not ${Tag}: the checkout is dirty or not at the tag"
}

$notesFile = Join-Path $package "notes-$Tag.md"
Set-Content -Path $notesFile -Value $notes
$env:GH_TOKEN = $Token
& gh release create $Tag @archives --repo CLBRITTON2/perusal --title $Tag --notes-file $notesFile --verify-tag
if ($LASTEXITCODE -ne 0) {
    throw "gh release create $Tag failed with exit code $LASTEXITCODE"
}
