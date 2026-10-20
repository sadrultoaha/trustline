#!/usr/bin/env bash
# Trustline device agent — reports disk encryption, screen lock and anti-malware
# status to Trustline. Run it from cron/launchd every few hours.
#
#   TRUSTLINE_URL=https://trustline.example.com TRUSTLINE_TOKEN=tld_... OWNER_EMAIL=you@company.com ./trustline-agent.sh
#
# Supported: macOS, Linux. On Windows use Git Bash or WSL with the same variables.
set -euo pipefail

: "${TRUSTLINE_URL:?set TRUSTLINE_URL}"
: "${TRUSTLINE_TOKEN:?set TRUSTLINE_TOKEN (Settings → Devices)}"
: "${OWNER_EMAIL:?set OWNER_EMAIL}"
VERSION="1.0.0"

json_bool() { if [ "$1" = "1" ]; then echo true; else echo false; fi; }

os="$(uname -s)"
case "$os" in
  Darwin)
    platform=macos
    os_version="$(sw_vers -productVersion)"
    serial="$(ioreg -rd1 -c IOPlatformExpertDevice | awk -F'"' '/IOPlatformSerialNumber/{print $4}')"
    hostname="$(scutil --get ComputerName 2>/dev/null || hostname)"
    enc=0; fdesetup status 2>/dev/null | grep -q "FileVault is On" && enc=1
    # Seconds until the screen saver starts, plus the password delay.
    idle="$(defaults -currentHost read com.apple.screensaver idleTime 2>/dev/null || echo 1200)"
    delay="$(sysadminctl -screenLock status 2>&1 | grep -oE '[0-9]+ seconds' | grep -oE '[0-9]+' || echo 0)"
    [ -z "$delay" ] && delay=0
    lock=$(( (idle + delay + 59) / 60 ))
    av=0; av_name="XProtect"
    [ -d /Library/Apple/System/Library/CoreServices/XProtect.bundle ] && av=1
    ;;
  Linux)
    platform=linux
    os_version="$( (. /etc/os-release && echo "$PRETTY_NAME") 2>/dev/null || uname -r)"
    serial="$(cat /sys/class/dmi/id/product_serial 2>/dev/null || cat /etc/machine-id)"
    hostname="$(hostname)"
    enc=0; lsblk -o TYPE 2>/dev/null | grep -q crypt && enc=1
    lock=999
    if command -v gsettings >/dev/null 2>&1; then
      d="$(gsettings get org.gnome.desktop.session idle-delay 2>/dev/null | awk '{print $2}')"
      l="$(gsettings get org.gnome.desktop.screensaver lock-enabled 2>/dev/null)"
      if [ -n "$d" ] && [ "$l" = "true" ] && [ "$d" -gt 0 ]; then lock=$(( (d + 59) / 60 )); fi
    fi
    av=0; av_name=""
    for svc in falcon-sensor clamav-daemon sentinelone mdatp; do
      if systemctl is-active --quiet "$svc" 2>/dev/null; then av=1; av_name="$svc"; break; fi
    done
    ;;
  MINGW*|MSYS*|CYGWIN*)
    platform=windows
    os_version="$(powershell -NoProfile -Command '(Get-CimInstance Win32_OperatingSystem).Version' | tr -d '\r')"
    serial="$(powershell -NoProfile -Command '(Get-CimInstance Win32_BIOS).SerialNumber' | tr -d '\r')"
    hostname="$(hostname)"
    enc=0; powershell -NoProfile -Command '(Get-BitLockerVolume -MountPoint C:).ProtectionStatus' 2>/dev/null | grep -q On && enc=1
    lock=999
    secs="$(powershell -NoProfile -Command '(Get-ItemProperty "HKCU:\Control Panel\Desktop").ScreenSaveTimeOut' 2>/dev/null | tr -d '\r')"
    [ -n "$secs" ] && lock=$(( (secs + 59) / 60 ))
    av=0; av_name="Microsoft Defender"
    powershell -NoProfile -Command '(Get-MpComputerStatus).RealTimeProtectionEnabled' 2>/dev/null | grep -q True && av=1
    ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

payload=$(cat <<JSON
{"serial":"$serial","hostname":"$hostname","owner_email":"$OWNER_EMAIL","platform":"$platform","os_version":"$os_version",
 "disk_encrypted":$(json_bool "$enc"),"screen_lock_minutes":$lock,"antivirus_running":$(json_bool "$av"),"antivirus_name":"$av_name","agent_version":"$VERSION"}
JSON
)
curl -fsS -X POST "$TRUSTLINE_URL/api/v1/agent/report" \
  -H "Authorization: Bearer $TRUSTLINE_TOKEN" -H "Content-Type: application/json" \
  --data "$payload" >/dev/null && echo "reported $hostname ($serial) to Trustline"
