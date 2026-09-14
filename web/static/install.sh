#!/bin/sh
# install.sh — one-click installer for the nps server and npc client
#
# Every installation generates a RANDOM process/service name: "sys" + 4 random
# lowercase letters (e.g. syskxqz). The service name, binary (/usr/bin/<name>),
# config directory (/etc/<name>/) and log (/var/log/<name>.log) all follow that
# name, so every machine gets a different process name.
#
# The config file names inside stay fixed (sysuahb.conf for the server,
# sysficb.conf for the client), so your data is always easy to find.
#
# Re-running this script automatically removes previous random-named installs
# (detected via their config marker) and installs fresh ones with new names.
#
# Usage:
#   ./install.sh [mode] [version] [npc args...]
#     mode:    npc | nps | all (default: all)
#     version: release tag, e.g. v0.34.7 (default: latest)
#     npc args: extra arguments forwarded to the client service, e.g.
#       ./install.sh npc v0.34.7 -server=1.2.3.4:8024 -vkey=YOUR_VKEY
#
# Environment variables:
#   NPS_INSTALL_MODE=npc|nps|all   same as the positional mode argument
#   NPS_INSTALL_VERSION=vX.Y.Z     same as the positional version argument
#   NPS_INSTALL_DIR=/path          portable install: extract only, no service
#   NPC_BIN_NAME=...               force client binary name (default: random)
#   NPS_BIN_NAME=...               force server binary name (default: random)
#   NPS_START=0                    do not auto-start the service after install
#   NPS_GH_PROXY=                  prefix for GitHub downloads; when set,
#                                  only this prefix is used (disables the
#                                  built-in mirror fallback list), e.g.
#                                  https://mirror.ghproxy.com/
#   NPS_CONNECT_TIMEOUT=10         downloader connect timeout in seconds
#   NPS_INSECURE=1                 skip TLS certificate verification
#   NPS_IPV4=1                     force IPv4 for downloads

# Must run as root
if command -v id >/dev/null 2>&1; then
  if [ "$(id -u)" -ne 0 ]; then
    echo "Error: Please run as root or use sudo." >&2
    exit 1
  fi
fi

set -e

has() { command -v "$1" >/dev/null 2>&1; }

# Pre-flight checks: ensure all external commands we rely on are present
for cmd in uname tar; do
  if ! has "$cmd"; then
    echo "Error: missing required command: $cmd" >&2
    exit 1
  fi
done
if ! has curl && ! has wget && ! has uclient-fetch; then
  echo "Error: need curl or wget or uclient-fetch" >&2
  exit 1
fi

REPO="2016xyz/sysuahb"
GH_BASE="https://github.com/${REPO}"
GH_API="https://api.github.com/repos/${REPO}"

# Tunables for downloaders (optional)
CONNECT_TIMEOUT="${NPS_CONNECT_TIMEOUT:-10}"
if [ "${NPS_INSECURE:-0}" = "1" ]; then
  CURL_INSECURE="--insecure"
  WGET_INSECURE="--no-check-certificate"
  UCLIENT_INSECURE="--no-check-certificate"
else
  CURL_INSECURE=""
  WGET_INSECURE=""
  UCLIENT_INSECURE=""
fi
if [ "${NPS_IPV4:-0}" = "1" ]; then
  CURL_IP="-4"
  WGET_IP="-4"
else
  CURL_IP=""
  WGET_IP=""
fi
GH_PROXY="${NPS_GH_PROXY:-}"
# Mirror prefixes tried in order when direct github.com access fails
# (mainland China servers usually cannot reach github.com directly)
GH_MIRRORS="https://ghfast.top/ https://gh-proxy.com/ https://ghproxy.net/ https://mirror.ghproxy.com/"

# Fix one fetch tool
if has curl; then
  FETCH_TOOL="curl"
elif has wget; then
  FETCH_TOOL="wget"
else
  FETCH_TOOL="uclient-fetch"
fi

# Safe cleanup
TMP_DIRS=""
cleanup() {
  for d in $TMP_DIRS; do
    case "$d" in
      /tmp/*)
        [ -n "$d" ] && [ "$d" != "/" ] && [ -d "$d" ] && rm -rf "$d"
        ;;
    esac
  done
}
trap cleanup 0 INT TERM

# ---- arguments ---------------------------------------------------------

MODE=""
if [ $# -gt 0 ]; then
  case "$1" in
    npc|nps|all) MODE="$1"; shift ;;
  esac
fi
[ -n "$MODE" ] || MODE="${NPS_INSTALL_MODE:-all}"
case "$MODE" in
  npc|nps|all) ;;
  *)
    echo "Error: unsupported mode: $MODE" >&2
    exit 1
    ;;
esac

INSTALL_VERSION="${NPS_INSTALL_VERSION:-latest}"
EXTRA_ARGS=""
if [ $# -gt 0 ]; then
  case "$1" in
    -*) EXTRA_ARGS="$*" ;;
    *)
      INSTALL_VERSION="$1"
      shift
      [ $# -gt 0 ] && EXTRA_ARGS="$*"
      ;;
  esac
fi

INSTALL_DIR="${NPS_INSTALL_DIR:-}"

echo "Mode: $MODE"

USE_LATEST=0

# Fetch latest version if unspecified
if [ "$INSTALL_VERSION" = "latest" ]; then
  echo "Get latest version..."
  if has grep && has sed; then
    # Try the GitHub API directly first, then via mirror proxies
    if [ -n "$GH_PROXY" ]; then
      API_URLS="${GH_PROXY}${GH_API}/releases/latest"
    else
      API_URLS="${GH_API}/releases/latest"
      for m in $GH_MIRRORS; do
        API_URLS="$API_URLS ${m}${GH_API}/releases/latest"
      done
    fi
    for api in $API_URLS; do
      if has curl; then
        RAW_JSON=$(curl -sSLf $CURL_IP $CURL_INSECURE --connect-timeout "$CONNECT_TIMEOUT" "$api" || true)
      else
        RAW_JSON=$(wget -q $WGET_INSECURE -T "$CONNECT_TIMEOUT" -O- "$api" || true)
      fi

      INSTALL_VERSION=$(printf '%s' "$RAW_JSON" \
        | grep -m1 '"tag_name"' \
        | sed 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/')
      [ -n "$INSTALL_VERSION" ] && break
    done
  fi

  if [ -z "$INSTALL_VERSION" ]; then
    echo "Warn: failed to detect version from GitHub API, will try releases/latest/download." >&2
    INSTALL_VERSION=latest
    USE_LATEST=1
  fi
fi

if [ "$USE_LATEST" -eq 1 ]; then
  echo "Version: latest (releases/latest/download)"
else
  echo "Version: $INSTALL_VERSION"
fi

# ---- platform detection ------------------------------------------------

# Determine OS
OS="$(uname -s)"
case "$OS" in
  Linux)   OS=linux;;
  Darwin)  OS=darwin;;
  FreeBSD) OS=freebsd;;
  *)
    echo "Error: OS not supported: $OS" >&2
    exit 1
    ;;
esac

# Determine ARCH
RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
  x86_64|amd64)      ARCH=amd64;;
  i?86)              ARCH=386;;
  aarch64|arm64)     ARCH=arm64;;
  armv7*|armv7l)     ARCH=arm_v7;;
  armv6*|armv6l)     ARCH=arm_v6;;
  armv5*|armv5l)     ARCH=arm_v5;;
  arm)               ARCH=arm;;
  mips64le)          ARCH=mips64le;;
  mips64)            ARCH=mips64;;
  mipsle)            ARCH=mipsle;;
  mips)              ARCH=mips;;
  loongarch64)       ARCH=loong64;;
  riscv64)           ARCH=riscv64;;
  *)
    echo "Error: ARCH not supported: $RAW_ARCH" >&2
    exit 1
    ;;
esac

echo "Detected OS/ARCH: $OS/$ARCH"

# MIPS float ABI detection
if [ "$ARCH" = "mips" ] || [ "$ARCH" = "mipsle" ]; then
  if has file; then
    out="$(file /bin/sh 2>/dev/null || true)"
    case "$out" in
      *hard[-\ ]float*)
        echo "Use hard-float for $ARCH"
        ;;
      *)
        ARCH="${ARCH}_softfloat"
        echo "Use soft-float, new ARCH: $ARCH"
        ;;
    esac
  else
    ARCH="${ARCH}_softfloat"
    echo "No 'file' tool; default to soft-float: $ARCH"
  fi
fi

# ---- helpers -----------------------------------------------------------

# Extraction
tar_extract() {
  f="$1"

  if tar -xzf "$f" 2>/dev/null; then
    return 0
  fi

  if tar xf "$f" 2>/dev/null; then
    return 0
  fi

  if has gzip; then
    if gzip -dc "$f" | tar xf -; then
      return 0
    fi
  fi

  if has gunzip; then
    if gunzip -c "$f" | tar xf -; then
      return 0
    fi
  fi

  return 1
}

# Download helper
download() {
  NAME=$1
  FILE="${OS}_${ARCH}_${NAME}.tar.gz"

  if [ "$USE_LATEST" -eq 1 ]; then
    URLS="
      ${GH_BASE}/releases/latest/download/${FILE}
    "
  else
    URLS="
      ${GH_BASE}/releases/download/${INSTALL_VERSION}/${FILE}
      ${GH_BASE}/releases/latest/download/${FILE}
    "
  fi

  # Decide workdir
  if [ -n "$INSTALL_DIR" ]; then
    mkdir -p "$INSTALL_DIR"
    WORKDIR="$INSTALL_DIR"
  else
    if has mktemp; then
      TMPD=$(mktemp -d 2>/dev/null || mktemp -d -t sys-install.XXXXXX)
    else
      TS="$(date +%s 2>/dev/null || echo 0)"
      TMPD="/tmp/sys-install-$$.$TS"
      mkdir -p "$TMPD"
    fi
    TMP_DIRS="$TMP_DIRS $TMPD"
    WORKDIR="$TMPD"
  fi

  cd "$WORKDIR"

  success=0
  for u in $URLS; do
    # Candidate order: explicit proxy > direct > known mirrors
    if [ -n "$GH_PROXY" ]; then
      CANDIDATES="${GH_PROXY}${u}"
    else
      CANDIDATES="$u"
      for m in $GH_MIRRORS; do
        CANDIDATES="$CANDIDATES ${m}${u}"
      done
    fi

    for cu in $CANDIDATES; do
      echo "Trying $cu" >&2

      rm -f -- "./$FILE"

      case "$FETCH_TOOL" in
        curl)
          if curl $CURL_IP -sfSL $CURL_INSECURE --connect-timeout "$CONNECT_TIMEOUT" -o "$FILE" "$cu"; then
            success=1
          fi
          ;;
        wget)
          if wget $WGET_IP -q $WGET_INSECURE -T "$CONNECT_TIMEOUT" -O "$FILE" "$cu"; then
            success=1
          fi
          ;;
        uclient-fetch)
          if uclient-fetch -q $UCLIENT_INSECURE -T "$CONNECT_TIMEOUT" -O "$FILE" "$cu"; then
            success=1
          fi
          ;;
      esac

      if [ "$success" -eq 1 ] && [ -s "$FILE" ]; then
        break 2
      fi
    done
  done

  if [ "$success" -ne 1 ] || [ ! -f "$FILE" ]; then
    echo "Error: Download failed for all candidate URLs:" >&2
    for u in $URLS; do
      if [ -n "$GH_PROXY" ]; then
        echo "  - ${GH_PROXY}${u}" >&2
      else
        echo "  - $u" >&2
        for m in $GH_MIRRORS; do
          echo "  - ${m}${u}" >&2
        done
      fi
    done
    exit 1
  fi

  if ! tar_extract "$FILE"; then
    echo "Error: failed to extract $FILE" >&2
    exit 1
  fi

  printf '%s\n' "$WORKDIR"
}

# Validate a forced binary name
valid_name() {
  case "$1" in
    ""|*[!A-Za-z0-9_-]*) return 1 ;;
  esac
  return 0
}

# Generate a random binary name: "sys" + 4 random lowercase letters
gen_name() {
  tries=0
  while [ "$tries" -lt 100 ]; do
    s="$(dd if=/dev/urandom bs=256 count=4 2>/dev/null | LC_ALL=C tr -dc 'a-z' | head -c 4)"
    if [ "${#s}" -eq 4 ]; then
      n="sys$s"
      if [ ! -e "/usr/bin/$n" ] && [ ! -e "/usr/local/bin/$n" ] && [ ! -d "/etc/$n" ]; then
        printf '%s\n' "$n"
        return 0
      fi
    fi
    tries=$((tries + 1))
  done
  echo "Error: cannot generate a unique random name; set NPC_BIN_NAME / NPS_BIN_NAME manually." >&2
  return 1
}

# Remove previous random-named installs, identified by their fixed config marker
clean_old() {
  marker="$1"
  for f in /usr/bin/sys[a-z][a-z][a-z][a-z] /usr/local/bin/sys[a-z][a-z][a-z][a-z]; do
    [ -x "$f" ] || continue
    n="$(basename "$f")"
    case "$n" in
      sys[a-z][a-z][a-z][a-z]) ;;
      *) continue ;;
    esac
    [ -f "/etc/$n/conf/$marker" ] || continue
    echo "Removing old install: $n"
    "$f" stop >/dev/null 2>&1 || true
    "$f" uninstall >/dev/null 2>&1 || true
    rm -f "$f" "/usr/bin/$n-update" "/usr/local/bin/$n-update" "/var/log/$n.log" 2>/dev/null || true
    rm -rf "/etc/$n" 2>/dev/null || true
  done
}

# ---- installation ------------------------------------------------------

# Install NPC (client)
install_npc() {
  SRC=$(download client)

  if [ -n "$INSTALL_DIR" ]; then
    echo "npc extracted to $INSTALL_DIR (portable mode, no service)"
    return
  fi

  clean_old "sysficb.conf"

  if [ -n "$NPC_BIN_NAME" ]; then
    valid_name "$NPC_BIN_NAME" || { echo "Error: invalid NPC_BIN_NAME: $NPC_BIN_NAME" >&2; exit 1; }
    NEW_NAME="$NPC_BIN_NAME"
  else
    NEW_NAME="$(gen_name)" || exit 1
  fi

  [ -x "$SRC/sysficb" ] || { echo "Error: 'sysficb' binary not found in $SRC" >&2; exit 1; }
  cp -f "$SRC/sysficb" "$SRC/$NEW_NAME"
  chmod 755 "$SRC/$NEW_NAME"

  echo "Installing npc as: $NEW_NAME"
  if [ -n "$EXTRA_ARGS" ]; then
    "$SRC/$NEW_NAME" install $EXTRA_ARGS
  else
    "$SRC/$NEW_NAME" install
  fi

  if [ "${NPS_START:-1}" = "1" ]; then
    "$SRC/$NEW_NAME" start || echo "Warn: failed to start $NEW_NAME; try manually: $NEW_NAME start" >&2
  fi

  echo "npc done. name=$NEW_NAME config=/etc/$NEW_NAME/conf/sysficb.conf"
}

# Install NPS (server)
install_nps() {
  SRC=$(download server)

  if [ -n "$INSTALL_DIR" ]; then
    echo "nps extracted to $INSTALL_DIR (portable mode, no service)"
    return
  fi

  clean_old "sysuahb.conf"

  if [ -n "$NPS_BIN_NAME" ]; then
    valid_name "$NPS_BIN_NAME" || { echo "Error: invalid NPS_BIN_NAME: $NPS_BIN_NAME" >&2; exit 1; }
    NEW_NAME="$NPS_BIN_NAME"
  else
    NEW_NAME="$(gen_name)" || exit 1
  fi

  [ -x "$SRC/sysuahb" ] || { echo "Error: 'sysuahb' binary not found in $SRC" >&2; exit 1; }
  cp -f "$SRC/sysuahb" "$SRC/$NEW_NAME"
  chmod 755 "$SRC/$NEW_NAME"

  echo "Installing nps as: $NEW_NAME"
  "$SRC/$NEW_NAME" install

  if [ "${NPS_START:-1}" = "1" ]; then
    "$SRC/$NEW_NAME" start || echo "Warn: failed to start $NEW_NAME; try manually: $NEW_NAME start" >&2
  fi

  echo "nps done. name=$NEW_NAME config=/etc/$NEW_NAME/conf/sysuahb.conf"
}

# Run installation per mode
case "$MODE" in
  npc) install_npc ;;
  nps) install_nps ;;
  all) install_npc; install_nps ;;
esac

echo "All done. Manage any installed service with:"
echo "  <name> status|stop|restart|uninstall|update"
