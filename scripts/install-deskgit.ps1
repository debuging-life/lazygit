# Installs deskgit and dt-hook for this Windows user, from DeskTick's
# releases, with no administrator rights:
#
#   irm https://cdn.desktimers.com/install.ps1 | iex
#
# Run again to update; once installed, `deskgit update` does the same.
# DESKGIT_RELEASES points it at another copy of the releases (for testing).

& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue' # the progress bar slows downloads a lot

  $base = if ($env:DESKGIT_RELEASES) { $env:DESKGIT_RELEASES.TrimEnd('/') } else { 'https://cdn.desktimers.com' }
  $arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "deskgit is built for 64-bit Windows only; this is $($env:PROCESSOR_ARCHITECTURE)." }
  }
  $key = "windows/$arch"

  $manifest = Invoke-RestMethod -UseBasicParsing -Uri "$base/latest.json"
  $asset = $manifest.assets.$key
  if (-not $asset) { throw "deskgit $($manifest.version) has no build for $key." }

  $dir = Join-Path $env:LOCALAPPDATA 'Programs\deskgit'
  New-Item -ItemType Directory -Force -Path $dir | Out-Null

  # Downloads next to where it goes, checks it against the manifest, then
  # puts it in place. A deskgit that is running cannot be written over but
  # can be renamed, so the one there is moved to .old first.
  function Install-Verified([string]$url, [string]$sha256, [string]$dest) {
    $download = "$dest.download"
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $download
    $got = (Get-FileHash -Algorithm SHA256 -Path $download).Hash
    if ($got -ne $sha256) {
      Remove-Item -Force $download
      throw "The download of $url is not the one released (checksum $got)."
    }
    Remove-Item -Force -ErrorAction SilentlyContinue "$dest.old"
    if (Test-Path $dest) { Rename-Item -Path $dest -NewName ((Split-Path -Leaf $dest) + '.old') }
    Move-Item -Path $download -Destination $dest
  }

  Write-Host "Installing deskgit $($manifest.version) for $key in $dir"
  Install-Verified $asset.url $asset.sha256 (Join-Path $dir 'deskgit.exe')
  Install-Verified $asset.hook_url $asset.hook_sha256 (Join-Path $dir 'dt-hook.exe')

  # On this user's PATH, for new terminals and for this one.
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  $entries = @($userPath -split ';' | Where-Object { $_ })
  if ($entries -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
    $env:Path = "$env:Path;$dir"
    Write-Host "Added $dir to your PATH."
  }

  if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Warning 'deskgit needs Git for Windows, which is not on your PATH: https://git-scm.com/download/win'
  }
  Write-Host ''
  Write-Host 'Done. Run: deskgit'
}
