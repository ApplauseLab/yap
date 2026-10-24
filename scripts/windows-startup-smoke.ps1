param([Parameter(Mandatory = $true)][string]$AppPath)

$ErrorActionPreference = "Stop"
$oldAppData = $env:APPDATA
$root = Join-Path $env:RUNNER_TEMP "yap-startup-smoke"
New-Item -ItemType Directory -Force -Path $root | Out-Null
$env:APPDATA = Join-Path $root "appdata"
$app = $null
try {
    # A clean first launch, like a new install before model download.
    $app = Start-Process -FilePath $AppPath -PassThru `
        -RedirectStandardError (Join-Path $root "stderr.log") `
        -RedirectStandardOutput (Join-Path $root "stdout.log")
    $deadline = (Get-Date).AddSeconds(60)
    $ready = $false
    while ((Get-Date) -lt $deadline) {
        if ($app.HasExited) { throw "Yap exited during startup (exit $($app.ExitCode))" }
        $logs = @(Get-ChildItem "$env:APPDATA\yap\logs\*.log" -ErrorAction SilentlyContinue)
        $text = ($logs | Get-Content) -join "`n"
        if ($text -match "nil pointer|process message error|Invalid JSON passed to callback") {
            throw "frontend binding failure during startup"
        }
        if ($text -match "OnStartup: complete" -and $text -match "App initial data load requested") {
            $ready = $true
            break
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $ready) { throw "Yap did not finish backend and frontend startup" }
    Start-Sleep -Seconds 3
    if ($app.HasExited) { throw "Yap exited after startup (exit $($app.ExitCode))" }
    $text = ($logs | Get-Content) -join "`n"
    if ($text -match "nil pointer|process message error|Invalid JSON passed to callback") {
        throw "frontend binding failure after startup"
    }
    Write-Host "Installed Yap.exe completed backend/frontend startup and remained alive."
} finally {
    if ($null -ne $app -and -not $app.HasExited) {
        Stop-Process -Id $app.Id -Force -ErrorAction SilentlyContinue
        $app.WaitForExit()
    }
    Get-ChildItem "$env:APPDATA\yap\logs\*.log" -ErrorAction SilentlyContinue | Get-Content | Write-Host
    Get-Content (Join-Path $root "stderr.log") -ErrorAction SilentlyContinue | Write-Host
    $env:APPDATA = $oldAppData
}
