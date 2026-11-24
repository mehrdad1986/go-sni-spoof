# Build script for SNI Spoofing Project
# Run as: .\build.ps1

Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host "   Building Go SNI Spoofing - Fixed Edition" -ForegroundColor Cyan
Write-Host "========================================================================" -ForegroundColor Cyan
Write-Host ""

# Check if Go is installed
Write-Host "[*] Checking Go installation..." -ForegroundColor Yellow
$goVersion = go version 2>$null
if ($LASTEXITCODE -ne 0) {
    Write-Host "[-] Go is not installed!" -ForegroundColor Red
    Write-Host "    Download from: https://go.dev/dl/" -ForegroundColor Red
    exit 1
}
Write-Host "[+] Found: $goVersion" -ForegroundColor Green
Write-Host ""

# Clean old builds
Write-Host "[*] Cleaning old builds..." -ForegroundColor Yellow
Remove-Item -Path "sni-spoof-fixed.exe" -ErrorAction SilentlyContinue
Remove-Item -Path "sni-scan.exe" -ErrorAction SilentlyContinue
Remove-Item -Path "test-build.exe" -ErrorAction SilentlyContinue
Write-Host "[+] Clean complete" -ForegroundColor Green
Write-Host ""

# Update dependencies
Write-Host "[*] Updating dependencies..." -ForegroundColor Yellow
go mod tidy
if ($LASTEXITCODE -ne 0) {
    Write-Host "[-] Failed to update dependencies" -ForegroundColor Red
    exit 1
}
Write-Host "[+] Dependencies updated" -ForegroundColor Green
Write-Host ""

# Build main proxy
Write-Host "[*] Building main proxy (sni-spoof-fixed.exe)..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o sni-spoof-fixed.exe ./cmd/sni-spoof
if ($LASTEXITCODE -ne 0) {
    Write-Host "[-] Failed to build main proxy" -ForegroundColor Red
    exit 1
}
Write-Host "[+] Built: sni-spoof-fixed.exe" -ForegroundColor Green
Write-Host ""

# Build scanner
Write-Host "[*] Building SNI scanner (sni-scan.exe)..." -ForegroundColor Yellow
go build -ldflags="-s -w" -o sni-scan.exe ./cmd/sni-scan
if ($LASTEXITCODE -ne 0) {
    Write-Host "[-] Failed to build scanner" -ForegroundColor Red
    exit 1
}
Write-Host "[+] Built: sni-scan.exe" -ForegroundColor Green
Write-Host ""

# Check WinDivert files
Write-Host "[*] Checking WinDivert files..." -ForegroundColor Yellow
if (!(Test-Path "WinDivert.dll")) {
    Write-Host "[!] WARNING: WinDivert.dll not found" -ForegroundColor Yellow
    Write-Host "    WinDivert mode will not work without it!" -ForegroundColor Yellow
} else {
    Write-Host "[+] Found: WinDivert.dll" -ForegroundColor Green
}

if (!(Test-Path "WinDivert64.sys")) {
    Write-Host "[!] WARNING: WinDivert64.sys not found" -ForegroundColor Yellow
    Write-Host "    WinDivert mode will not work without it!" -ForegroundColor Yellow
} else {
    Write-Host "[+] Found: WinDivert64.sys" -ForegroundColor Green
}
Write-Host ""

# Success summary
Write-Host "========================================================================" -ForegroundColor Green
Write-Host "   Build Complete! ✅" -ForegroundColor Green
Write-Host "========================================================================" -ForegroundColor Green
Write-Host ""
Write-Host "Built files:" -ForegroundColor Cyan
Write-Host "  • sni-spoof-fixed.exe  - Main proxy tool" -ForegroundColor White
Write-Host "  • sni-scan.exe         - Fake SNI scanner" -ForegroundColor White
Write-Host ""
Write-Host "Quick Start:" -ForegroundColor Cyan
Write-Host "  1. Find working fake SNI:" -ForegroundColor White
Write-Host "     .\sni-scan.exe" -ForegroundColor Gray
Write-Host ""
Write-Host "  2. Run proxy:" -ForegroundColor White
Write-Host "     .\sni-spoof-fixed.exe -listen 127.0.0.1:1080 -fake-sni speedtest.net" -ForegroundColor Gray
Write-Host ""
Write-Host "  3. Configure your browser/app to use SOCKS5 proxy:" -ForegroundColor White
Write-Host "     Host: 127.0.0.1" -ForegroundColor Gray
Write-Host "     Port: 1080" -ForegroundColor Gray
Write-Host ""
Write-Host "For more help, see:" -ForegroundColor Cyan
Write-Host "  • README_UPDATED.md - Full documentation" -ForegroundColor White
Write-Host "  • ANALYSIS_AND_COMPARISON.md - Technical details" -ForegroundColor White
Write-Host ""
Write-Host "NOTE: Run as Administrator for WinDivert mode!" -ForegroundColor Yellow
Write-Host "========================================================================" -ForegroundColor Green
