#!/usr/bin/env bash
# Diagnostic + guided setup for mcpelauncher on Arch / CachyOS.
# Companion to docs/CLIENT.md — does NOT redistribute Mojang binaries.
#
# Usage:
#   ./scripts/setup-mcpelauncher-cachyos.sh              # diagnose only
#   ./scripts/setup-mcpelauncher-cachyos.sh --install    # diagnose, then prompt to install
#   ./scripts/setup-mcpelauncher-cachyos.sh --fix-path  # user-local PATH fix for DE/app-menu
#
# Refs:
#   https://github.com/minecraft-linux/mcpelauncher-manifest/issues/796
#   https://mcpelauncher.readthedocs.io/

set -euo pipefail

INSTALL=0
FIX_PATH=0
for arg in "$@"; do
  case "$arg" in
    --install|-i) INSTALL=1 ;;
    --fix-path) FIX_PATH=1 ;;
    -h|--help)
      sed -n '2,15p' "$0"
      exit 0
      ;;
    *)
      echo "Unknown option: $arg (try --help)" >&2
      exit 2
      ;;
  esac
done
RED=$'\033[31m'
GRN=$'\033[32m'
YEL=$'\033[33m'
BLU=$'\033[34m'
BOLD=$'\033[1m'
RST=$'\033[0m'

pass=0
warn=0
fail=0
MISSING_PKGS=()
HINTS=()

ok()   { ((pass++)) || true; printf '%s[OK]%s   %s\n' "$GRN" "$RST" "$*"; }
note() { ((warn++)) || true; printf '%s[WARN]%s %s\n' "$YEL" "$RST" "$*"; }
bad()  { ((fail++)) || true; printf '%s[FAIL]%s %s\n' "$RED" "$RST" "$*"; }
info() { printf '%s[···]%s %s\n' "$BLU" "$RST" "$*"; }
hint() { HINTS+=("$*"); }

need_pkg() {
  local pkg=$1
  if pacman -Q "$pkg" &>/dev/null; then
    ok "package $pkg"
  else
    bad "package $pkg missing"
    MISSING_PKGS+=("$pkg")
  fi
}

have_cmd() { command -v "$1" &>/dev/null; }

aur_helper() {
  if have_cmd paru; then echo paru
  elif have_cmd yay; then echo yay
  else echo ""
  fi
}

is_archish() {
  [[ -f /etc/os-release ]] || return 1
  # shellcheck disable=SC1091
  . /etc/os-release
  [[ "${ID:-}" == "arch" || "${ID:-}" == "cachyos" || "${ID_LIKE:-}" == *arch* ]]
}

multilib_enabled() {
  # Active (uncommented) [multilib] section in pacman.conf
  awk '
    /^[[:space:]]*#/ { next }
    /^\[multilib\]/ { found=1; exit }
    END { exit !found }
  ' /etc/pacman.conf
}

printf '%s\n' "${BOLD}mcpelauncher setup check (Arch / CachyOS)${RST}"
echo "Mode: $([[ $INSTALL -eq 1 ]] && echo 'diagnose + guided install' || echo 'diagnose only')"
echo

# --- Distro ---
if is_archish; then
  # shellcheck disable=SC1091
  . /etc/os-release
  ok "distro ${PRETTY_NAME:-$ID} (Arch-like)"
else
  bad "not Arch/CachyOS — this script targets pacman + AUR only"
  echo "See https://mcpelauncher.readthedocs.io/ for other distros / Flatpak / AppImage."
  exit 1
fi

# --- Package managers ---
if have_cmd pacman; then
  ok "pacman available"
else
  bad "pacman not found"
  exit 1
fi

HELPER=$(aur_helper)
if [[ -n "$HELPER" ]]; then
  ok "AUR helper: $HELPER"
else
  note "no paru/yay — AUR packages need an AUR helper or manual makepkg"
  hint "Install paru or yay, then re-run with --install"
fi

# --- Multilib (issue #796 Arch deps) ---
if multilib_enabled; then
  ok "pacman [multilib] enabled"
else
  bad "[multilib] not enabled in /etc/pacman.conf"
  hint "Uncomment [multilib] + Include in /etc/pacman.conf, then: sudo pacman -Sy"
fi

# --- Core launcher packages ---
BIN_UI=/opt/mcpelauncher-bin/bin/mcpelauncher-ui-qt
BIN_CLIENT=/opt/mcpelauncher-bin/bin/mcpelauncher-client
PROFILE_D=/etc/profile.d/mcpelauncher-bin.sh

AUR_INSTALLED=0
if pacman -Q mcpelauncher-linux-bin &>/dev/null || pacman -Q mcpelauncher-linux-git &>/dev/null || pacman -Q mcpelauncher-linux &>/dev/null; then
  ok "mcpelauncher client package installed"
  AUR_INSTALLED=1
else
  bad "mcpelauncher client not installed (mcpelauncher-linux-bin or -git)"
  MISSING_PKGS+=("mcpelauncher-linux-bin")
fi

if pacman -Q mcpelauncher-ui-bin &>/dev/null || pacman -Q mcpelauncher-ui-git &>/dev/null || pacman -Q mcpelauncher-ui &>/dev/null; then
  ok "mcpelauncher UI package installed"
else
  bad "mcpelauncher UI not installed (mcpelauncher-ui-bin or -git)"
  MISSING_PKGS+=("mcpelauncher-ui-bin")
fi

if [[ -x "$BIN_UI" ]]; then
  ok "binary $BIN_UI"
else
  note "UI binary not at $BIN_UI (path differs for -git builds)"
fi

if [[ -x "$BIN_CLIENT" ]]; then
  ok "binary $BIN_CLIENT"
else
  note "client binary not at $BIN_CLIENT (path differs for -git builds)"
fi

# --- PATH / profile.d / DE session ---
LOCAL_BIN="${HOME}/.local/bin"
ENV_D="${HOME}/.config/environment.d/99-mcpelauncher-path.conf"
USER_DESKTOP="${HOME}/.local/share/applications/mcpelauncher-ui-qt.desktop"
OPT_BIN_DIR=/opt/mcpelauncher-bin/bin

if [[ -f "$PROFILE_D" ]]; then
  ok "profile.d script $PROFILE_D"
else
  if [[ $AUR_INSTALLED -eq 1 ]]; then
    note "no $PROFILE_D — may still be on PATH via package"
  else
    info "profile.d not present yet (expected after mcpelauncher-*-bin install)"
  fi
fi

SHELL_HAS_CLIENT=0
SHELL_HAS_UI=0
have_cmd mcpelauncher-client && SHELL_HAS_CLIENT=1
have_cmd mcpelauncher-ui-qt && SHELL_HAS_UI=1

if [[ $SHELL_HAS_CLIENT -eq 1 && $SHELL_HAS_UI -eq 1 ]]; then
  ok "mcpelauncher-client and mcpelauncher-ui-qt on PATH (this shell)"
elif [[ $SHELL_HAS_UI -eq 1 && $SHELL_HAS_CLIENT -eq 0 ]]; then
  note "UI on PATH but mcpelauncher-client missing — Play will fail from this shell"
  hint "source $PROFILE_D  or: ./scripts/setup-mcpelauncher-cachyos.sh --fix-path"
elif [[ -x "$BIN_UI" || -x "$BIN_CLIENT" ]]; then
  note "binaries exist under $OPT_BIN_DIR but not on PATH in this shell"
  hint "source $PROFILE_D   # or open a new login session"
  hint "or: ./scripts/setup-mcpelauncher-cachyos.sh --fix-path"
else
  note "launcher not on PATH yet"
fi

# App-menu / systemd --user often skips profile.d (common "Game not found" cause)
DE_PATH=""
if command -v systemctl &>/dev/null; then
  DE_PATH=$(systemctl --user show-environment 2>/dev/null | sed -n 's/^PATH=//p' || true)
fi
if [[ -n "$DE_PATH" ]]; then
  if [[ ":$DE_PATH:" == *":$OPT_BIN_DIR:"* ]] || [[ ":$DE_PATH:" == *":$LOCAL_BIN:"* && -L "$LOCAL_BIN/mcpelauncher-client" ]]; then
    ok "desktop/systemd user PATH can resolve mcpelauncher-client"
  else
    note "desktop/systemd user PATH lacks $OPT_BIN_DIR (and usable ~/.local/bin client link)"
    hint "App menu launches skip profile.d → UI error: 'Could not find the gamelauncher… mcpelauncher-client'"
    hint "Fix: ./scripts/setup-mcpelauncher-cachyos.sh --fix-path   then re-open from app menu (or log out/in)"
  fi
else
  info "could not read systemd --user PATH (skipping DE PATH check)"
fi

if [[ -f "$ENV_D" ]]; then
  ok "user environment.d drop-in $ENV_D"
else
  if [[ -x "$BIN_CLIENT" ]]; then
    info "no $ENV_D yet — recommended so DE sessions see mcpelauncher on PATH"
  fi
fi

if [[ -f "$USER_DESKTOP" ]] && grep -q 'mcpelauncher-bin/bin' "$USER_DESKTOP" 2>/dev/null; then
  ok "user desktop override sets launcher PATH: $USER_DESKTOP"
fi
# --- Multilib runtime libs (issue #796 + upstream troubleshooting) ---
info "checking 32-bit runtime deps (audio/GL for older / AppImage-style paths)"
for pkg in lib32-mesa lib32-libpulse lib32-alsa-plugins; do
  need_pkg "$pkg"
done

# NVIDIA 32-bit GL if proprietary driver present
if pacman -Q nvidia-utils &>/dev/null || pacman -Q lib32-nvidia-utils &>/dev/null || lsmod 2>/dev/null | grep -q '^nvidia'; then
  if pacman -Q lib32-nvidia-utils &>/dev/null; then
    ok "package lib32-nvidia-utils (NVIDIA 32-bit GL)"
  else
    note "NVIDIA detected but lib32-nvidia-utils missing"
    MISSING_PKGS+=("lib32-nvidia-utils")
  fi
fi

# --- Display / Qt / Wayland ---
SESSION="${XDG_SESSION_TYPE:-unknown}"
info "session: XDG_SESSION_TYPE=$SESSION  WAYLAND_DISPLAY=${WAYLAND_DISPLAY:-}  DISPLAY=${DISPLAY:-}"
if [[ "$SESSION" == "wayland" ]]; then
  note "Wayland session — Qt/X11 quirks possible for native launcher"
  hint "If the UI fails to show: QT_QPA_PLATFORM=xcb $BIN_UI"
  hint "Flatpak alternative if UI hangs: flatpak run --nosocket=wayland io.mrarm.mcpelauncher"
else
  ok "not Wayland (or unset) — fewer Qt socket issues expected"
fi

# OpenGL quick glance
if have_cmd glxinfo; then
  renderer=$(glxinfo -B 2>/dev/null | awk -F': ' '/OpenGL renderer string/{print $2; exit}')
  if [[ -n "${renderer:-}" ]]; then
    if echo "$renderer" | grep -qi llvmpipe; then
      note "OpenGL renderer looks like software (llvmpipe): $renderer"
      hint "Install proper GPU drivers / 32-bit GL packages — see mcpelauncher troubleshooting"
    else
      ok "OpenGL renderer: $renderer"
    fi
  fi
else
  info "glxinfo not installed (optional: pacman -S mesa-utils)"
fi

# --- Flatpak optional path ---
if have_cmd flatpak; then
  if flatpak info io.mrarm.mcpelauncher &>/dev/null; then
    ok "Flatpak io.mrarm.mcpelauncher installed"
  else
    info "Flatpak present but mcpelauncher not installed (optional path)"
  fi
else
  info "flatpak not installed (optional; AUR path is preferred on Arch)"
fi

# --- MSA note ---
if pacman -Qs '^mcpelauncher-msa' &>/dev/null; then
  ok "MSA-related package present"
else
  info "no separate mcpelauncher-msa package — modern UI usually handles Google/Microsoft login in-process"
  hint "If UI says 'MSA daemon could not be found', install mcpelauncher-msa-git from AUR"
fi

# --- Game data presence (not ownership proof) ---
VERSIONS_DIR="${HOME}/.local/share/mcpelauncher/versions"
if [[ -d "$VERSIONS_DIR" ]] && [[ -n "$(ls -A "$VERSIONS_DIR" 2>/dev/null || true)" ]]; then
  ok "local game versions dir has content: $VERSIONS_DIR"
else
  note "no extracted game versions under $VERSIONS_DIR yet"
  hint "Own Minecraft on Google Play, open the UI, sign in, then Download and Play"
fi

# --- Summary ---
echo
printf '%sSummary:%s %s%d ok%s, %s%d warn%s, %s%d fail%s\n' \
  "$BOLD" "$RST" "$GRN" "$pass" "$RST" "$YEL" "$warn" "$RST" "$RED" "$fail" "$RST"

if ((${#HINTS[@]})); then
  echo
  printf '%sNext steps / hints:%s\n' "$BOLD" "$RST"
  i=1
  for h in "${HINTS[@]}"; do
    printf '  %d. %s\n' "$i" "$h"
    ((i++)) || true
  done
fi

echo
printf '%sGoogle Play ownership (required):%s\n' "$BOLD" "$RST"
cat <<'EOF'
  - mcpelauncher does NOT ship Minecraft. Buy it on Google Play for the account you use.
  - This script cannot purchase the game or bypass license checks.
  - In the UI: sign into that Google account → accept Play TOS → Download and Play.
  - Play reject / no APK URL: try AUR -git packages + "Refresh Play version channel"
    (see mcpelauncher-manifest#1373). Do not use random APKs from the internet.
EOF

echo
printf '%sLicense:%s bedrock-operator / this script never redistribute Mojang client binaries.\n' "$BOLD" "$RST"
echo "Docs: https://mcpelauncher.readthedocs.io/  |  Arch deps list: mcpelauncher-manifest#796"

# --- User-local PATH fix (DE / app-menu) ---
apply_fix_path() {
  echo
  printf '%sUser-local PATH fix (no sudo)%s\n' "$BOLD" "$RST"

  if [[ ! -x "$BIN_CLIENT" ]]; then
    bad "missing $BIN_CLIENT — install mcpelauncher-linux-bin first"
    return 1
  fi

  mkdir -p "$LOCAL_BIN" "${HOME}/.config/environment.d" "${HOME}/.local/share/applications"

  local b
  for b in mcpelauncher-client mcpelauncher-ui-qt mcpelauncher-extract mcpelauncher-error mcpelauncher-webview; do
    if [[ -x "$OPT_BIN_DIR/$b" ]]; then
      ln -sfn "$OPT_BIN_DIR/$b" "$LOCAL_BIN/$b"
      ok "symlink $LOCAL_BIN/$b -> $OPT_BIN_DIR/$b"
    fi
  done

  cat > "$ENV_D" <<EOF
# mcpelauncher-*-bin installs under /opt; DE/app-menu launches skip /etc/profile.d.
# Ensures ~/.local/bin (user symlinks) and the opt bin dir are visible to systemd --user.
PATH=${LOCAL_BIN}:${OPT_BIN_DIR}:\${PATH}
EOF
  ok "wrote $ENV_D"

  cat > "$USER_DESKTOP" <<EOF
[Desktop Entry]
Name=Minecraft Bedrock Launcher
Comment=A Minecraft: Bedrock Edition (earlier: Pocket Edition launcher)
Exec=env PATH="${LOCAL_BIN}:${OPT_BIN_DIR}:/usr/local/bin:/usr/bin:/bin" ${BIN_UI} %U
Icon=mcpelauncher-ui-qt
MimeType=application/zip
Type=Application
Categories=Game;
Terminal=false
EOF
  ok "wrote $USER_DESKTOP (Exec sets PATH for app-menu)"

  if command -v update-desktop-database &>/dev/null; then
    update-desktop-database "${HOME}/.local/share/applications" 2>/dev/null || true
  fi

  if command -v systemctl &>/dev/null; then
    local cur new
    cur=$(systemctl --user show-environment 2>/dev/null | sed -n 's/^PATH=//p' || true)
    if [[ -n "$cur" ]]; then
      new="${LOCAL_BIN}:${OPT_BIN_DIR}:${cur}"
      systemctl --user set-environment "PATH=${new}" 2>/dev/null && \
        ok "updated current systemd --user PATH for this session" || \
        note "could not update systemd --user PATH (log out/in after environment.d)"
    fi
  fi

  echo
  info "Verify: PATH=\$(systemctl --user show-environment | sed -n 's/^PATH=//p') command -v mcpelauncher-client"
  info "Relaunch Minecraft Bedrock Launcher from the app menu (close any old UI first)."
  info "environment.d is also applied on next full login."
  return 0
}

if [[ $FIX_PATH -eq 1 ]]; then
  apply_fix_path
  exit $?
fi

# --- Guided install ---
if [[ $INSTALL -ne 1 ]]; then
  echo
  info "Re-run with --install for interactive package install prompts (no silent --noconfirm)."
  info "If the UI says gamelauncher/mcpelauncher-client not found: re-run with --fix-path."
  exit $([[ $fail -eq 0 ]] && echo 0 || echo 1)
fi

echo
printf '%sGuided install%s\n' "$BOLD" "$RST"

# Deduplicate missing pkgs
declare -A seen=()
UNIQUE=()
for p in "${MISSING_PKGS[@]+"${MISSING_PKGS[@]}"}"; do
  [[ -n "${seen[$p]:-}" ]] && continue
  seen[$p]=1
  UNIQUE+=("$p")
done

if ((${#UNIQUE[@]} == 0)); then
  ok "nothing missing from the package checklist"
  if [[ -f "$PROFILE_D" ]] && ! have_cmd mcpelauncher-ui-qt; then
    echo
    info "Fix PATH for this shell:"
    echo "  source $PROFILE_D"
    echo "  mcpelauncher-ui-qt"
  fi
  echo
  read -r -p "Apply user-local PATH fix for app-menu / DE launches? [Y/n] " ans
  if [[ -z "${ans}" || "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
    apply_fix_path
  else
    info "skipped --fix-path (run later if Play says mcpelauncher-client not found)"
  fi
  exit 0
fi
REPO_PKGS=()
AUR_PKGS=()
for p in "${UNIQUE[@]}"; do
  case "$p" in
    mcpelauncher-*|msa-*) AUR_PKGS+=("$p") ;;
    *) REPO_PKGS+=("$p") ;;
  esac
done

if ((${#REPO_PKGS[@]})); then
  echo
  echo "Official repo packages to install:"
  printf '  %s\n' "${REPO_PKGS[@]}"
  read -r -p "Install with pacman now? [y/N] " ans
  if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
    # Intentionally NO --noconfirm — user confirms each pacman prompt.
    sudo pacman -S "${REPO_PKGS[@]}"
  else
    info "skipped repo packages"
  fi
fi

if ((${#AUR_PKGS[@]})); then
  echo
  echo "AUR packages to install:"
  printf '  %s\n' "${AUR_PKGS[@]}"
  if [[ -z "$HELPER" ]]; then
    bad "no paru/yay — install an AUR helper first, then: paru -S ${AUR_PKGS[*]}"
  else
    read -r -p "Install with $HELPER now? [y/N] " ans
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      "$HELPER" -S "${AUR_PKGS[@]}"
    else
      info "skipped AUR packages"
    fi
  fi
fi

echo
info "After install: ./scripts/setup-mcpelauncher-cachyos.sh --fix-path"
info "Or: source $PROFILE_D   (shell only), then: mcpelauncher-ui-qt"
info "Re-run this script without --install to verify."
exit 0
