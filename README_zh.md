# NPS 内网穿透 (改名版)

[![GitHub Release](https://img.shields.io/github/v/release/2016xyz/sysuahb)](https://github.com/2016xyz/sysuahb/releases)
[![GitHub All Releases](https://img.shields.io/github/downloads/2016xyz/sysuahb/total)](https://github.com/2016xyz/sysuahb/releases)

> 本仓库基于 [djylb/nps](https://github.com/djylb/nps) v0.34.7 重新打包，采用**随机进程名**：服务端二进制为 `sysuahb`，客户端为 `sysficb`，一键安装脚本会在**每次安装时生成一个全新的随机进程名**。

- [English](https://github.com/2016xyz/sysuahb/blob/master/README.md)

---

## 简介

NPS 是一款轻量高效的内网穿透代理服务器，支持多种协议（TCP、UDP、HTTP、HTTPS、SOCKS5 等）转发。它提供直观的 Web 管理界面，使得内网资源能安全、便捷地在外网访问，同时满足多种复杂场景的需求。

由于[NPS](https://github.com/ehang-io/nps)停更已久，[djylb/nps](https://github.com/djylb/nps) 整合社区更新二次开发而来，本仓库是其改名重打包版本。

**本版本的不同之处：**

- 服务端二进制：`sysuahb` · 客户端二进制：`sysficb`
- Linux 一键安装脚本在**每次安装时**生成**随机进程名**（`sys` + 4 位随机小写字母，如 `syskxqz`）——每台机器的进程名都不一样
- 服务名、二进制路径（`/usr/bin/<name>`）、配置目录（`/etc/<name>/`）、日志文件（`/var/log/<name>.log`）都跟随随机名；目录内的配置文件名保持固定（`sysuahb.conf` / `sysficb.conf`），数据永远好找
- 重复运行安装脚本会自动清理旧的随机名安装（通过配置标记识别），并以新名字重新安装

- 新增**客户端标签（Tags）**（仅测试版，稳定版 `v0.34.7` 无此功能）——每个客户端可打多个标签（如 `gz`、`telecom`、`jp`、`hk`），统一代理可按标签选择出口（详见[客户端标签](#客户端标签)）

- 新增**统一代理**功能——一个 HTTP/SOCKS5 代理端口按登录用户名将流量路由到不同客户端，连接密码必填（详见[统一代理](#统一代理)）；`auto` / 纯数字 / `xxx-auto[-ttl]` 在稳定版 `v0.34.7` 即可用，`gz.auto`、`abc.gz-auto-2h` 这类**标签路由**需要测试版

- 新增**代理节点**（仅测试版）——外部 HTTP/SOCKS5 代理与 NPS Client 进入同一个出口池（详见[测试版安装指南](docs/install-test.md)）

- **文档（上游）：** https://d-jy.net/docs/nps/
- **讨论交流：**  [Telegram 交流群](https://t.me/npsdev)
- **Android：**  [djylb/npsclient](https://github.com/djylb/npsclient) | **OpenWrt：**  [djylb/nps-openwrt](https://github.com/djylb/nps-openwrt)

![NPS Web UI](https://cdn.jsdelivr.net/gh/djylb/nps/image/web.png)

---

## 主要特性

- **多协议支持**  
  TCP/UDP 转发、HTTP/HTTPS 转发、HTTP/SOCKS5 代理、P2P 模式、Proxy Protocol支持、HTTP/3支持等，满足各种内网访问场景。

- **客户端标签（Tags）**  
  每个客户端可携带 0~多个标签（`[a-z0-9_-]+`），标签持久化保存，统一代理据此把出口限制在指定分组内。

- **统一代理（用户名路由）**  
  一个代理端口、一个必填密码：登录用户名决定流量从哪个客户端出去——每连接随机、固定客户端 ID，或按标签粘滞自定义时长（详见[统一代理](#统一代理)）。

- **跨平台部署**  
  支持 Linux、Windows 等主流平台，可轻松安装为系统服务。

- **随机进程名**  
  每次安装生成不同的进程名/服务名（`sys` + 4 位随机字母）；也可通过环境变量强制指定固定名字。

- **Web 管理界面**  
  实时监控流量、连接情况以及客户端状态，操作简单直观。

- **安全与扩展**  
  内置加密传输、流量限制、到期限制、证书管理续签等多重功能，保障数据安全。

- **多连接协议**  
  支持 TCP、KCP、TLS、QUIC、WS、WSS 协议连接服务器。

---

## 客户端标签

在 Web 管理端 **客户端 → 新增/编辑客户端 → 标签** 中填写，一行一个（也支持逗号、空格分隔）：

```text
gz
telecom
```

规则：

- 保存时自动 **去空格、转小写、去重**；
- 只允许 `[a-z0-9_-]+`，**不允许 `.`**（`.` 是统一代理用户名的分隔符，保留以免歧义）；
- 一个客户端可有多个标签，标签随客户端配置**持久化**，重启不丢失；
- 客户端列表以 **Badge** 形式展示标签。

---

## 统一代理

只需创建一个 HTTP/SOCKS5 代理任务（一个公网端口 + 一个必填密码），即可通过代理**用户名**为每个连接选择出口客户端，无需为每台内网机器单独建隧道。

**用户名 = 出口路由；密码 = 认证。** 两者职责分离，密码必须非空（前端与后端都会校验，禁止空密码连接）。

### 用户名语法

| 用户名 | 行为 |
| --- | --- |
| `auto` | 在所有**可用**客户端中随机；**每个新连接重新选择**，不缓存 |
| `12`（纯数字） | 固定 **Client ID=12**；该客户端不存在或离线则**直接失败**，不 fallback |
| `gz.auto` | 只在 `tag=gz` 的可用客户端中随机；每个新连接重新选择，不缓存 |
| `abc-auto` | 所有客户端中 **Sticky Random**；`stickyKey=abc`，使用默认 TTL |
| `abc-auto-30m` | 同上，但 `TTL=30m` |
| `abc.gz-auto` | 只在 `tag=gz` 的客户端中 **Sticky Random**；`stickyKey=abc`，默认 TTL |
| `abc.gz-auto-30m` | `stickyKey=abc`、`tag=gz`、`TTL=30m` |

实际例子：`jp.auto`、`crawler.jp-auto`、`crawler.jp-auto-2h`、`user001.hk-auto-1d`。

### TTL

- 单位支持 `m`（分钟）、`h`（小时）、`d`（天）；
- 范围：**最小 `1m`，最大 `24h`**；
- 未写 TTL 时使用任务的 `DefaultCacheTTL`（默认 **10 分钟**）；
- 非法 TTL **直接失败**，不猜测、不降级。

### 行为约定

- **密码必填**：缺失或错误时 HTTP 代理返回 `407 Proxy Authentication Required`，SOCKS5 直接拒绝。
- **随机发生在“新代理连接建立”时**，不是每个 HTTP 请求；HTTP Keep-Alive、HTTPS CONNECT、SOCKS5 TCP 一旦选定出口，**连接生命周期内不再切换**。
- **Sticky TTL 到期只影响下一条新连接**，不会断开已有连接。
- 缓存以 `UnifiedProxyID + 完整用户名` 为键（存 **ClientID**，不存指针）；命中时会重新校验客户端**是否存在 / 在线 / 可用 / 是否仍带有所需标签**，任一不满足立即失效并重新选择。
- 固定 ID 与 Sticky 的失败语义不同：`12` 离线 → 直接失败；`abc.gz-auto` 缓存的客户端离线或标签被删 → **自动重新随机**一个符合 `gz` 的客户端。
- 若没有任何客户端满足指定标签，**直接失败**，不会退化为“全部客户端随机”。
- 非法用户名、非法标签、非法 TTL **一律拒绝**，不做猜测或 fallback。

---

## 安装与使用

更多详细配置请参考 [文档](https://d-jy.net/docs/nps/)（部分内容可能未更新）。

### 一键部署（Linux）

安装脚本自动检测系统/架构，从 [Releases](https://github.com/2016xyz/sysuahb/releases) 下载对应压缩包，生成随机名（`sys` + 4 位字母）、注册系统服务并启动。需要 root 权限。

> **版本说明**：不带版本号时默认 `latest`，即**稳定版 `v0.34.7`**。测试版（`v0.34.7-testN`）必须显式指定，不会被自动安装——详见[测试版安装指南](docs/install-test.md)。

#### 服务端（nps）

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s nps
```

安装结束时输出的最后几行会显示生成的进程名和配置路径：

```
Installing nps as: syskxqz
nps done. name=syskxqz config=/etc/syskxqz/conf/sysuahb.conf
```

> **提示：** 首次安装后请先编辑 `/etc/<name>/conf/sysuahb.conf`（监听端口、Web 管理账号等），再执行 `sudo <name> restart`。

#### 客户端（npc）

连接命令请从 NPS Web 管理端的客户端页面复制——`npc` 之后的参数会原样透传给客户端服务：

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s npc -server=1.2.3.4:8024 -vkey=YOUR_VKEY
```

也可以先不带参数安装，稍后再配置（编辑 `/etc/<name>/conf/sysficb.conf`，或带参数重跑安装脚本重新注册）：

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s npc
```

> **提示：** 客户端支持同时连接多个服务器，示例：
> `-server=xxx:123,yyy:456,zzz:789 -vkey=key1,key2,key3 -type=tcp,tls`
> 这里 `xxx:123` 使用 tcp，`yyy:456` 和 `zzz:789` 使用 tls。如需连接旧版本服务器请添加 `-proto_version=0`。

#### 同一台机器同时装服务端和客户端

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s all
```

### 查找和管理已安装的服务

安装结束时脚本会打印生成的名字。如果没注意到，可以通过固定的配置标记查找：

```bash
for d in /etc/sys????; do
  [ -f "$d/conf/sysuahb.conf" ] && echo "服务端: ${d##*/}"
  [ -f "$d/conf/sysficb.conf" ] && echo "客户端: ${d##*/}"
done
```

把 `<name>` 替换成查到的名字，即可执行所有管理命令：

```bash
sudo <name> status|stop|restart|uninstall

# 更新到最新版本
sudo <name> update && sudo <name> restart
```

### 安装脚本选项

环境变量（通过 `sudo env VAR=... sh -s ...` 传入）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `NPS_INSTALL_MODE` | `all` | `npc` / `nps` / `all` |
| `NPS_INSTALL_VERSION` | `latest` | 固定版本号，如 `v0.34.7` |
| `NPS_INSTALL_DIR` | *(空)* | 便携模式：只解压文件，不注册服务 |
| `NPC_BIN_NAME` / `NPS_BIN_NAME` | 随机 | 强制指定二进制/服务名，不用随机名 |
| `NPS_START` | `1` | 设为 `0` 安装后不自动启动 |
| `NPS_GH_PROXY` | *(空)* | GitHub 下载加速前缀，如 `https://mirror.ghproxy.com/` |
| `NPS_CONNECT_TIMEOUT` | `10` | 下载连接超时（秒） |
| `NPS_INSECURE` | `0` | 设为 `1` 跳过 TLS 证书校验 |
| `NPS_IPV4` | `0` | 设为 `1` 强制 IPv4 下载 |

**示例**

强制固定客户端名字（如用于配置管理）：

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo env NPC_BIN_NAME=sysmycl sh -s npc -server=1.2.3.4:8024 -vkey=YOUR_VKEY
```

国内加速：脚本可先经 jsdelivr 下载，Release 压缩包通过 `NPS_GH_PROXY` 加速：

```bash
curl -fsSLo install.sh https://fastly.jsdelivr.net/gh/2016xyz/sysuahb@v0.34.7/install.sh
sudo NPS_GH_PROXY="https://mirror.ghproxy.com/" sh install.sh nps
```

安装后不自动启动：

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo env NPS_START=0 sh -s nps
```

### 手动安装

从 [Releases](https://github.com/2016xyz/sysuahb/releases) 下载对应平台的压缩包。命名规则：`<os>_<arch>_server.tar.gz` 内含服务端 `sysuahb`，`<os>_<arch>_client.tar.gz` 内含客户端 `sysficb`，例如 `linux_amd64_server.tar.gz`、`windows_amd64_client.tar.gz`。

偏好容器部署？直接跳到 [Docker 部署](#docker-部署)。

#### Linux

```bash
tar -xzf linux_amd64_server.tar.gz
sudo mv sysuahb myname && sudo chmod 755 myname   # 可选：改名以获得自定义服务名
sudo ./myname install
sudo ./myname start
```

#### Windows

> 需要 Windows 10 或更新版本。

**服务端**

1. 下载并解压 `windows_amd64_server.tar.gz`。
2. 可选：将 `sysuahb.exe` 改成任意名字（如 `syskxqz.exe`）——服务名、安装目录（`C:\Program Files\<name>`）和日志文件都跟随 exe 文件名。
3. 安装并管理（以下命令中的名字替换成你实际使用的名字）：

```powershell
.\sysuahb.exe install
.\sysuahb.exe start|stop|restart|uninstall

# 更新
.\sysuahb.exe stop
.\sysuahb.exe update
.\sysuahb.exe start
```

> **提示：** 配置文件在解压目录的 `conf\sysuahb.conf`；安装后位于 `C:\Program Files\<name>\conf\sysuahb.conf`。

**客户端**

```powershell
.\sysficb.exe install -server="1.2.3.4:8024" -vkey="YOUR_VKEY" -type="tls,tcp" -log="off"
.\sysficb.exe start|stop|restart|uninstall

# 更新
.\sysficb.exe stop
.\sysficb.exe update
.\sysficb.exe start
```

### Docker 部署

每次发布 tag 时会自动向 ghcr.io 推送多架构镜像（`linux/amd64`、`linux/arm64`、`linux/arm/v7`）：

- 服务端：`ghcr.io/2016xyz/sysuahb`
- 客户端：`ghcr.io/2016xyz/sysficb`

**服务端** —— 推荐使用 `host` 网络，代理/桥接端口无需映射。配置持久化在 `/conf`，首次启动会自动生成默认 `sysuahb.conf`。

```bash
docker run -d --name sysuahb --restart unless-stopped \
  --network host \
  -v $(pwd)/conf:/conf \
  ghcr.io/2016xyz/sysuahb:latest
```

或使用 Docker Compose——参考仓库中的 [docker-compose.yml](docker-compose.yml)：

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

管理页面：`http://<主机>:8081`（默认 `admin` / `123`，请立即修改）。

**客户端** —— `docker run` 的额外参数会原样传给 `sysficb`（建议加 `-log=stdout`，以便 `docker logs` 查看日志）：

```bash
docker run -d --name sysficb --restart unless-stopped \
  --network host \
  ghcr.io/2016xyz/sysficb:latest \
  -server=1.2.3.4:8024 -vkey=YOUR_VKEY -log=stdout
```
