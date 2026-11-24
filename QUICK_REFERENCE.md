# Quick Reference Card - Go SNI Spoofing

## 🎯 One-Minute Quick Start

```powershell
# 1. Find working SNI
.\sni-scan.exe

# 2. Run proxy (as Administrator!)
.\sni-spoof-fixed.exe -listen 127.0.0.1:1080 -fake-sni speedtest.net

# 3. Configure browser SOCKS5 proxy to: 127.0.0.1:1080
```

---

## 📋 Common Commands

### Scanner
```powershell
.\sni-scan.exe                    # Basic scan
.\sni-scan.exe -v                 # Verbose
.\sni-scan.exe -o results.txt     # Save to file
.\sni-scan.exe -target 104.16.123.96  # Custom IP
```

### Proxy Modes
```powershell
# SOCKS5 for browsers
.\sni-spoof-fixed.exe -listen 127.0.0.1:1080 -mode socks5

# HTTP CONNECT for system proxy
.\sni-spoof-fixed.exe -listen 127.0.0.1:8080 -mode http

# Direct tunnel for VPN
.\sni-spoof-fixed.exe -listen 127.0.0.1:40443 -connect 172.67.139.236:443

# Mixed (auto-detect)
.\sni-spoof-fixed.exe -listen 127.0.0.1:1080 -mode mixed
```

### Testing
```powershell
# Test before using
.\sni-spoof-fixed.exe -test www.youtube.com:443 -fake-sni speedtest.net

# Verbose debugging
.\sni-spoof-fixed.exe -verbose -listen 127.0.0.1:1080
```

### Strategies
```powershell
-strategy combo      # Default (best)
-strategy fragment   # TLS splitting only
-strategy decoy      # TTL + fragment
-strategy ttl        # Low-TTL only
```

---

## 🔧 Configuration Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-listen` | 127.0.0.1:8080 | Listen address |
| `-connect` | - | Target (tunnel mode) |
| `-mode` | mixed | socks5/http/tunnel/mixed |
| `-fake-sni` | speedtest.net | Decoy domain |
| `-fake-sni-list` | - | Multiple SNIs (comma-separated) |
| `-strategy` | combo | Evasion strategy |
| `-frag-delay` | 5 | Delay in ms (min 50 recommended) |
| `-ttl` | 5 | TTL hops for decoy |
| `-verbose` | false | Debug logging |
| `-test` | - | Preflight test |
| `-config` | - | JSON config file |

---

## 🐛 Troubleshooting Quick Fixes

| Problem | Solution |
|---------|----------|
| Access Denied | Run PowerShell as Administrator |
| DLL not found | Copy WinDivert.dll + .sys beside exe |
| Timeouts | Run `.\sni-scan.exe` to find working SNI |
| Still doesn't work | Try `-strategy fragment` or `-strategy ttl` |
| Debug needed | Add `-verbose` flag |

---

## 📁 Important Files

| File | Purpose |
|------|---------|
| `START_HERE.md` | Quick start guide (begin here!) |
| `README_UPDATED.md` | Complete documentation |
| `FIXES_SUMMARY.md` | What was fixed |
| `sni-spoof-fixed.exe` | Main proxy tool |
| `sni-scan.exe` | SNI scanner |
| `build.ps1` | Build script |

---

## 🔗 Quick Links

**Getting Started:** See START_HERE.md  
**Full Manual:** See README_UPDATED.md  
**Technical Details:** See ANALYSIS_AND_COMPARISON.md  
**What Changed:** See FIXES_SUMMARY.md  

---

## 💡 Pro Tips

1. **Always run scanner first** to find current working SNIs
2. **Use Administrator** for best performance (WinDivert mode)
3. **Enable verbose** when troubleshooting: `-verbose`
4. **Try multiple strategies** if one doesn't work
5. **Update fake SNIs weekly** - ISPs change blocklists

---

## ⚠️ Important Notes

- **Administrator required** for WinDivert mode (recommended)
- **Not a VPN** - doesn't hide your IP, only bypasses SNI filtering
- **Windows only** for advanced mode (Linux/macOS use userspace fallback)
- **Legal for personal use** in most countries (check local laws)

---

**Version:** 2.0.0-fixed | **Status:** ✅ Production Ready

*Keep this file for quick reference!*
