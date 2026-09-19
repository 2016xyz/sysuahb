# NPS Enhanced

A high-performance NAT traversal and reverse proxy server with Web UI.

[![GitHub Release](https://img.shields.io/github/v/release/2016xyz/sysuahb)](https://github.com/2016xyz/sysuahb/releases)
[![GitHub All Releases](https://img.shields.io/github/downloads/2016xyz/sysuahb/total)](https://github.com/2016xyz/sysuahb/releases)

> This is a repackaged build of [djylb/nps](https://github.com/djylb/nps) v0.34.7 with **randomized process names**: the server ships as `sysuahb` and the client as `sysficb`, and the one-click installer assigns a fresh random name on **every installation**.

- [中文文档 / Chinese](https://github.com/2016xyz/sysuahb/blob/master/README_zh.md)

---

## Introduction

NPS is a lightweight and efficient NAT traversal and reverse proxy system for exposing services behind NAT or firewalls. It supports multiple protocols such as TCP, UDP, HTTP, HTTPS, and SOCKS5, and provides a Web management interface for convenient deployment and monitoring.

Since the original [NPS](https://github.com/ehang-io/nps) project has been inactive for a long time, [djylb/nps](https://github.com/djylb/nps) continues its development as an actively maintained community version. This repository publishes a renamed build of it.

**What is different in this build:**

- Server binary: `sysuahb` · Client binary: `sysficb`
- The Linux one-click installer generates a **random process name** (`sys` + 4 random letters, e.g. `syskxqz`) on **every installation** — every machine gets a different name
- The service name, binary (`/usr/bin/<name>`), config directory (`/etc/<name>/`) and log file (`/var/log/<name>.log`) all follow that random name; the config file names inside stay fixed (`sysuahb.conf` / `sysficb.conf`) so your data is always easy to find
- Re-running the installer automatically removes previous random-named installs (detected via their config marker) and installs fresh ones with new names

- New feature: **Client Tags** — every client can carry several tags (`gz`, `telecom`, `jp`, `hk`, ...), which become the exit groups of the unified proxy (see [Client Tags](#client-tags))

- New feature: **Client Tags** — every client can carry multiple tags (`gz`, `telecom`, `jp`, `hk`, ...), which act as the egress groups of the Unified Proxy (see [Client Tags](#client-tags))

- New feature: **Unified Proxy** — one HTTP/SOCKS5 proxy port routes traffic to different clients by login username, with a mandatory connection password (see [Unified Proxy](#unified-proxy))

- **Documentation (upstream):** https://d-jy.net/docs/nps/
- **Discussion:**  [Telegram Group](https://t.me/npsdev)
- **Android:**  [djylb/npsclient](https://github.com/djylb/npsclient) | **OpenWrt:**  [djylb/nps-openwrt](https://github.com/djylb/nps-openwrt)

![NPS Web UI](https://cdn.jsdelivr.net/gh/djylb/nps/image/web.png)

---

## Client Tags

Tags group clients by egress location, carrier or purpose, and are what the Unified Proxy routes on:

- Managed in **Web UI → Client → Add / Edit**: one tag per line (commas and spaces also work).
- Shown as **badges** in the client list.
- Persisted in `clients.json` — tags survive a server restart.
- Normalisation on save: **trim → lowercase → de-duplicate** (first-appearance order is kept).
- Allowed characters: **`[a-z0-9_-]`** only. Dots are **not** allowed, because `.` separates the sticky key from the tag in a username.
- A tag must contain at least one alphanumeric character, so `-`, `--` or `__` are rejected.
- A client may carry **any number** of tags, including none.

```
Client1: [gz, telecom]
Client2: [gz, mobile]
Client3: [jp]
Client4: [hk]
```

---

## Unified Proxy

Create a single HTTP/SOCKS5 proxy task (one public port + one mandatory password) and route each connection to an exit client through the proxy **username** — no need to create a separate tunnel for every client.

The **username only selects the egress route**; the **password is the only authentication** — it must be non-empty, and an empty password can never connect.

| Username | Behaviour |
| --- | --- |
| `auto` | Random over **all available** clients; re-picked for **every new connection**; never cached |
| `12` | **Pinned** to client ID 12. If that client does not exist or is offline, the connection **fails** — there is no fallback |
| `gz.auto` | Random over clients **tagged `gz`**; re-picked for every new connection; never cached |
| `abc-auto` | **Sticky** random over all clients. Sticky key `abc`, default TTL |
| `abc-auto-30m` | Same, with TTL = 30 minutes |
| `abc.gz-auto` | **Sticky** random over clients tagged `gz`. Sticky key `abc`, default TTL |
| `abc.gz-auto-30m` | Sticky key `abc`, tag `gz`, TTL = 30 minutes |
| anything else | **Rejected** — invalid usernames, tags and TTLs fail hard, nothing is guessed and nothing falls back |

Real-world examples:

```
jp.auto
crawler.jp-auto
crawler.jp-auto-2h
user001.hk-auto-1d
```

**TTL**: `m` (minutes), `h` (hours), `d` (days); minimum **1m**, maximum **24h**. A TTL on a random tag route (`gz.auto-30m`) is invalid — use `abc.gz-auto-30m` for sticky + TTL.

**Rules**

- The **password is mandatory**: when it is missing or wrong, the HTTP proxy answers `407 Proxy Authentication Required` and SOCKS5 rejects the connection. The server refuses to start a unified proxy task without a password.
- Tags are strict: `gz.auto` can **never** select a `jp` or `hk` client. If no client matches the requested tag, the connection **fails** — it never degrades to a random pick over all clients.
- The sticky cache stores **`CacheKey → ClientID`** (never a client pointer), and the key includes the **unified proxy task ID plus the full username**, so two unified proxy instances never share cache entries.
- Every cache hit is **re-validated**: the client must still exist, be online, be available, and still carry the requested tag. If any of these fails, the entry is dropped immediately and a new matching client is picked.
- A **pinned client ID is not sticky**: when client 12 is offline the connection fails at once, while `abc.gz-auto` simply re-picks another `gz` client.
- The exit client is fixed **per connection** — random selection happens when a new proxy connection is established, not per HTTP request. HTTP keep-alive requests, HTTPS `CONNECT` tunnels and SOCKS5 TCP sessions all keep the same exit for their whole lifetime.
- A TTL expiry only affects the **next new connection**; it never tears down an established one.
- The default cache duration for sticky usernames is configured per task (`0` = 10 minutes); an explicit TTL in the username always takes priority.
- Tags, the client selector and the sticky cache are all concurrency-safe (in-memory store with an `RWMutex`).

---

## Installation and Usage

For detailed configuration options, refer to the upstream [Documentation](https://d-jy.net/docs/nps/) (some sections may be outdated).

### One-Click Deploy (Linux)

The installer auto-detects OS/ARCH, downloads the matching tarball from [Releases](https://github.com/2016xyz/sysuahb/releases), generates a random name (`sys` + 4 letters), registers the service, and starts it. Run it as root.

#### Server (nps)

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s nps
```

The last lines of the output show the generated name and config path:

```
Installing nps as: syskxqz
nps done. name=syskxqz config=/etc/syskxqz/conf/sysuahb.conf
```

> **Tip:** For first-time setup, edit `/etc/<name>/conf/sysuahb.conf` (ports, web admin credentials, etc.) and then run `sudo <name> restart`.

#### Client (npc)

Copy the connect command from the client page of the NPS Web UI — everything after `npc` is forwarded to the client service:

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s npc -server=1.2.3.4:8024 -vkey=YOUR_VKEY
```

Or install without arguments first and configure later (edit `/etc/<name>/conf/sysficb.conf`, then re-run the installer with arguments to re-register):

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s npc
```

> **Tip:** The client supports connecting to multiple servers simultaneously. Example:
> `-server=xxx:123,yyy:456,zzz:789 -vkey=key1,key2,key3 -type=tcp,tls`
> Here `xxx:123` uses TCP, while `yyy:456` and `zzz:789` use TLS. To connect to older server versions, add `-proto_version=0`.

#### Server and client on the same machine

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s all
```

### Find and Manage the Installed Service

The installer prints the generated name when it finishes. If you missed it, locate it via the fixed config markers:

```bash
for d in /etc/sys????; do
  [ -f "$d/conf/sysuahb.conf" ] && echo "server: ${d##*/}"
  [ -f "$d/conf/sysficb.conf" ] && echo "client: ${d##*/}"
done
```

Replace `<name>` with it for all management commands:

```bash
sudo <name> status|stop|restart|uninstall

# Update to the latest release
sudo <name> update && sudo <name> restart
```

### Installer Options

Environment variables (pass with `sudo env VAR=... sh -s ...`):

| Variable | Default | Description |
| --- | --- | --- |
| `NPS_INSTALL_MODE` | `all` | `npc` / `nps` / `all` |
| `NPS_INSTALL_VERSION` | `latest` | Pin a release tag, e.g. `v0.34.7` |
| `NPS_INSTALL_DIR` | *(empty)* | Portable mode: extract files only, do not register a service |
| `NPC_BIN_NAME` / `NPS_BIN_NAME` | random | Force a fixed binary/service name instead of a random one |
| `NPS_START` | `1` | Set `0` to skip auto-start after install |
| `NPS_GH_PROXY` | *(empty)* | Prefix for GitHub release downloads, e.g. `https://mirror.ghproxy.com/` |
| `NPS_CONNECT_TIMEOUT` | `10` | Downloader connect timeout in seconds |
| `NPS_INSECURE` | `0` | Set `1` to skip TLS certificate verification |
| `NPS_IPV4` | `0` | Set `1` to force IPv4 for downloads |

**Examples**

Force a fixed client name (e.g. for configuration management):

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo env NPC_BIN_NAME=sysmycl sh -s npc -server=1.2.3.4:8024 -vkey=YOUR_VKEY
```

Use a GitHub download proxy (mainland China). Download the script first, then run it with the proxy prefix for release downloads:

```bash
curl -fsSLo install.sh https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh
sudo NPS_GH_PROXY="https://mirror.ghproxy.com/" sh install.sh nps
```

Install without starting the service:

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo env NPS_START=0 sh -s nps
```

### Manual Installation

Download the tarball for your platform from [Releases](https://github.com/2016xyz/sysuahb/releases). Naming: `<os>_<arch>_server.tar.gz` contains `sysuahb` (server), and `<os>_<arch>_client.tar.gz` contains `sysficb` (client) — e.g. `linux_amd64_server.tar.gz`, `windows_amd64_client.tar.gz`.

Prefer containers? Skip to [Docker Deployment](#docker-deployment).

#### Linux

```bash
tar -xzf linux_amd64_server.tar.gz
sudo mv sysuahb myname && sudo chmod 755 myname   # optional: rename for a custom service name
sudo ./myname install
sudo ./myname start
```

#### Windows

> Requires Windows 10 or newer.

**Server**

1. Download and extract `windows_amd64_server.tar.gz`.
2. Optional: rename `sysuahb.exe` to any name (e.g. `syskxqz.exe`) — the service name, install directory (`C:\Program Files\<name>`) and log file follow the exe name.
3. Install and manage (run the matching name in the commands below):

```powershell
.\sysuahb.exe install
.\sysuahb.exe start|stop|restart|uninstall

# Update
.\sysuahb.exe stop
.\sysuahb.exe update
.\sysuahb.exe start
```

> **Tip:** Edit the config before or after install: `conf\sysuahb.conf` in the extracted folder, or `C:\Program Files\<name>\conf\sysuahb.conf` after installation.

**Client**

```powershell
.\sysficb.exe install -server="1.2.3.4:8024" -vkey="YOUR_VKEY" -type="tls,tcp" -log="off"
.\sysficb.exe start|stop|restart|uninstall

# Update
.\sysficb.exe stop
.\sysficb.exe update
.\sysficb.exe start
```

### Docker Deployment

Docker images are published to GHCR automatically on every release tag (multi-arch: `linux/amd64`, `linux/arm64`, `linux/arm/v7`):

- Server: `ghcr.io/2016xyz/sysuahb`
- Client: `ghcr.io/2016xyz/sysficb`

**Server** — `host` networking is recommended so proxy/bridge ports work without mapping. Config is persisted under `/conf`; the default `sysuahb.conf` is generated there on first start.

```bash
docker run -d --name sysuahb --restart unless-stopped \
  --network host \
  -v $(pwd)/conf:/conf \
  ghcr.io/2016xyz/sysuahb:latest
```

Or use Docker Compose — see [docker-compose.yml](docker-compose.yml) in the repository:

```yaml
services:
  sysuahb:
    image: ghcr.io/2016xyz/sysuahb:latest
    container_name: sysuahb
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./conf:/conf
```

Web UI: `http://<host>:8081` (default `admin` / `123` — change them immediately).

**Client** — any extra `docker run` arguments are passed straight to `sysficb` (use `-log=stdout` so `docker logs` works):

```bash
docker run -d --name sysficb --restart unless-stopped \
  --network host \
  ghcr.io/2016xyz/sysficb:latest \
  -server=1.2.3.4:8024 -vkey=YOUR_VKEY -log=stdout
```
