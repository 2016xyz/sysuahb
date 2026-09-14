# NPS 内网穿透 (改名版)

[![GitHub Release](https://img.shields.io/github/v/release/2016xyz/sysuahb)](https://github.com/2016xyz/sysuahb/releases)
[![GitHub All Releases](https://img.shields.io/github/downloads/2016xyz/sysuahb/total)](https://github.com/2016xyz/sysuahb/releases)

> 本仓库基于 [djylb/nps](https://github.com/djylb/nps) v0.34.7 重新打包，采用**随机进程名**：服务端二进制为 `sysuahb`，客户端为 `sysficb`，一键安装脚本会在**每次安装时生成一个全新的随机进程名**。

- [English](https://github.com/2016xyz/sysuahb/blob/v0.34.7/README.md)

---

## 简介

NPS 是一款轻量高效的内网穿透代理服务器，支持多种协议（TCP、UDP、HTTP、HTTPS、SOCKS5 等）转发。它提供直观的 Web 管理界面，使得内网资源能安全、便捷地在外网访问，同时满足多种复杂场景的需求。

由于[NPS](https://github.com/ehang-io/nps)停更已久，[djylb/nps](https://github.com/djylb/nps) 整合社区更新二次开发而来，本仓库是其改名重打包版本。

**本版本的不同之处：**

- 服务端二进制：`sysuahb` · 客户端二进制：`sysficb`
- Linux 一键安装脚本在**每次安装时**生成**随机进程名**（`sys` + 4 位随机小写字母，如 `syskxqz`）——每台机器的进程名都不一样
- 服务名、二进制路径（`/usr/bin/<name>`）、配置目录（`/etc/<name>/`）、日志文件（`/var/log/<name>.log`）都跟随随机名；目录内的配置文件名保持固定（`sysuahb.conf` / `sysficb.conf`），数据永远好找
- 重复运行安装脚本会自动清理旧的随机名安装（通过配置标记识别），并以新名字重新安装

- 新增**统一代理**功能——一个 HTTP/SOCKS5 代理端口按登录用户名将流量路由到不同客户端，连接密码必填（详见[统一代理](#统一代理)）

- **文档（上游）：** https://d-jy.net/docs/nps/
- **讨论交流：**  [Telegram 交流群](https://t.me/npsdev)
- **Android：**  [djylb/npsclient](https://github.com/djylb/npsclient) | **OpenWrt：**  [djylb/nps-openwrt](https://github.com/djylb/nps-openwrt)

![NPS Web UI](https://cdn.jsdelivr.net/gh/djylb/nps/image/web.png)

---

## 主要特性

- **多协议支持**  
  TCP/UDP 转发、HTTP/HTTPS 转发、HTTP/SOCKS5 代理、P2P 模式、Proxy Protocol支持、HTTP/3支持等，满足各种内网访问场景。

- **统一代理（用户名路由）**  
  一个代理端口、一个必填密码：登录用户名决定流量从哪个客户端出去——每连接随机、固定客户端 ID 或粘滞自定义时长（详见[统一代理](#统一代理)）。

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

## 统一代理

只需创建一个 HTTP/SOCKS5 代理任务（一个公网端口 + 一个必填密码），即可通过代理**用户名**为每个连接选择出口客户端，无需为每台内网机器单独建隧道：

| 用户名 | 行为 |
| --- | --- |
| `auto` | 每个新连接随机选择一个**在线**客户端 |
| `1`（纯数字） | 固定从**客户端 1** 出去；该客户端不在线则连接失败 |
| `abc-auto` | **粘滞**模式：首次随机选定客户端，并按任务的默认缓存时长保持（默认 10 分钟） |
| `abc-auto-30m` | **粘滞**模式并自定义时长——支持 `m`（分钟）、`h`（小时）、`d`（天），如 `5m`、`2h`、`1d` |
| 其他任何用户名 | **拒绝连接** |

- **密码必填**：密码缺失或错误时，HTTP 代理返回 `407 Proxy Authentication Required`，SOCKS5 直接拒绝连接。
- 粘滞缓存以**完整用户名**为键；缓存客户端离线（或条目过期）后，下个连接会重新选择客户端并重置时长。
- 出口客户端按**连接**固定——已建立的连接不会中途切换出口。
- 粘滞用户名的默认缓存时长可在任务中配置（`0` 表示默认 10 分钟）；用户名自带的时长优先。

---

## 安装与使用

更多详细配置请参考 [文档](https://d-jy.net/docs/nps/)（部分内容可能未更新）。

### 一键部署（Linux）

安装脚本自动检测系统/架构，从 [Releases](https://github.com/2016xyz/sysuahb/releases) 下载对应压缩包，生成随机名（`sys` + 4 位字母）、注册系统服务并启动。需要 root 权限。

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

本仓库不发布 Docker 镜像；如需容器部署可使用上游镜像（[duan2001/nps](https://hub.docker.com/r/duan2001/nps)、[duan2001/npc](https://hub.docker.com/r/duan2001/npc)）。

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
