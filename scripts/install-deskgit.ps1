# Installs deskgit and dt-hook for this Windows user, from DeskTick's
# releases, with no administrator rights:
#
#   irm https://cdn.desktimers.com/install.ps1 | iex
#
# Run again to update; once installed, `deskgit update` does the same.
# DESKGIT_RELEASES_URL points it at another copy of the releases (for
# testing), as it does `deskgit update`.

& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue' # the progress bar slows downloads a lot

  $base = if ($env:DESKGIT_RELEASES_URL) { $env:DESKGIT_RELEASES_URL.TrimEnd('/') } else { 'https://cdn.desktimers.com' }
  # A 32-bit PowerShell on 64-bit Windows says x86 here and the truth in
  # PROCESSOR_ARCHITEW6432.
  $machine = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
  $arch = switch ($machine) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "deskgit is built for 64-bit Windows only; this is $machine." }
  }
  $key = "windows/$arch"

  $manifest = Invoke-RestMethod -UseBasicParsing -Uri "$base/latest.json"
  $asset = $manifest.assets.$key
  if (-not $asset) { throw "deskgit $($manifest.version) has no build for $key." }

  $dir = Join-Path $env:LOCALAPPDATA 'Programs\deskgit'
  New-Item -ItemType Directory -Force -Path $dir | Out-Null

  # Downloads next to where it goes, checks it against the manifest, then
  # puts it in place. A deskgit that is running cannot be written over but
  # can be renamed, so the one there is moved aside under a name of its own
  # (an earlier one may still be running too), and put back if the new one
  # cannot be moved in. Those left from before are removed where they can be.
  function Install-Verified([string]$url, [string]$sha256, [string]$dest) {
    $download = "$dest.download"
    Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $download
    $got = (Get-FileHash -Algorithm SHA256 -Path $download).Hash
    if ($got -ne $sha256) {
      Remove-Item -Force $download
      throw "The download of $url is not the one released (checksum $got)."
    }
    Get-ChildItem -Path "$dest.*.old" -ErrorAction SilentlyContinue |
      Remove-Item -Force -ErrorAction SilentlyContinue
    $aside = $null
    if (Test-Path $dest) {
      $aside = "$dest.$([DateTime]::UtcNow.Ticks).old"
      Rename-Item -Path $dest -NewName (Split-Path -Leaf $aside)
    }
    try {
      Move-Item -Path $download -Destination $dest
    } catch {
      if ($aside) { Rename-Item -Path $aside -NewName (Split-Path -Leaf $dest) }
      Remove-Item -Force -ErrorAction SilentlyContinue $download
      throw
    }
  }

  Write-Host "Installing deskgit $($manifest.version) for $key in $dir"
  Install-Verified $asset.url $asset.sha256 (Join-Path $dir 'deskgit.exe')
  Install-Verified $asset.hook_url $asset.hook_sha256 (Join-Path $dir 'dt-hook.exe')

  # On this user's PATH, for new terminals and for this one. Read and
  # written in the registry as it is stored: through [Environment] each
  # %VAR% in it would be written back expanded, fixed to today's value.
  $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
  try {
    $userPath = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
    $entries = @($userPath -split ';' | Where-Object { $_ })
    if ($entries -notcontains $dir) {
      $key.SetValue('Path', (($entries + $dir) -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
      $env:Path = "$env:Path;$dir"
      # Setting a variable through [Environment] tells running programs
      # (Explorer, so new terminals) that the environment changed.
      [Environment]::SetEnvironmentVariable('DESKGIT_PATH_CHANGED', '1', 'User')
      [Environment]::SetEnvironmentVariable('DESKGIT_PATH_CHANGED', $null, 'User')
      Write-Host "Added $dir to your PATH."
    }
  } finally {
    $key.Close()
  }

  if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Warning 'deskgit needs Git for Windows, which is not on your PATH: https://git-scm.com/download/win'
  }
  Write-Host ''
  Write-Host 'Done. Run: deskgit'
}
