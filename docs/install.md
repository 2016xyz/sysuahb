# 安装指南

NPS 提供多种安装方式，推荐使用 **一键脚本安装**（Linux），也支持 **二进制发布包安装** 及 **源码编译**。

> 本仓库构建的服务端二进制名为 `sysuahb`，客户端为 `sysficb`。Linux 一键脚本会在**每次安装时**生成随机进程名（`sys` + 4 位随机字母，如 `syskxqz`），服务名、二进制路径（`/usr/bin/<name>`）、配置目录（`/etc/<name>/`）、日志文件均跟随该名字；目录内配置文件名固定不变（`sysuahb.conf` / `sysficb.conf`）。

---

## 0. 版本选择

| 类型 | 版本号写法 | 说明 |
| --- | --- | --- |
| 稳定版（推荐） | `v0.34.7` | 不指定版本时默认 `latest`，即稳定版 **`v0.34.7`** |
| 测试版 | `v0.34.7-test6`（当前推荐）；历史：`-test1` / `-test2` / `-test3` / `-test4`（`-test5` 已撤下） | 必须**显式指定**，可带 `v` 前缀或纯数字开头 |

**`latest` 永远指向稳定版 `v0.34.7`**：预发布版本（版本号含 `-`）自动标记为 pre-release，不会占用 `releases/latest`，也不会覆盖 Docker 的 `latest` 镜像。

指定版本安装：

```bash
# 稳定版（显式固定版本，等价于不写版本号）
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s nps v0.34.7

# 测试版（必须显式指定版本号）
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s nps v0.34.7-test6
```

> ⚠️ 版本号写错（如不存在的 `0.34.8`）会**直接报错退出**，不会静默装成 `latest`。
> 测试版的新功能、安装、回滚与风险说明见 [测试版安装指南](install-test.md)。
>
> 📌 **关于脚本版本**：上面的命令把 `install.sh` 固定在发布标签 `v0.34.7` 上，该标签里的脚本**不含** 2026-09-21 加入的版本号归一化修复（`0.34.7-testN` 不带 `v` 会被静默装成稳定版）。**安装测试版请使用 `v0.34.7-test7` 或更新标签里的脚本**（见[测试版安装指南](install-test.md)）。

---

## 1. 一键脚本安装（Linux，推荐）

> 此方式不支持 **Windows** 安装。

### 1.1 服务端（nps）

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s nps
```

安装结束时会输出本次生成的随机进程名和配置路径：

```
Installing nps as: syskxqz
nps done. name=syskxqz config=/etc/syskxqz/conf/sysuahb.conf
```

首次安装后请先编辑 `/etc/<name>/conf/sysuahb.conf`，确认无误后执行 `sudo <name> restart`。

管理命令（`<name>` 替换为安装时生成的名字）：

```bash
sudo <name> status|stop|restart|uninstall

# 更新
sudo <name> update && sudo <name> restart
```

### 1.2 客户端（npc）

连接命令请从 NPS Web 管理端客户端页面复制，`npc` 之后的参数会原样透传给客户端服务：

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s npc -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tls -log=off
```

安装结束时会输出本次生成的随机进程名：

```
Installing npc as: sysmtpw
npc done. name=sysmtpw config=/etc/sysmtpw/conf/sysficb.conf
```

也可以先不带参数安装，稍后编辑 `/etc/<name>/conf/sysficb.conf` 或带参数重跑脚本。

### 1.3 脚本说明

* **每次安装生成随机进程名**（`sys` + 4 位字母，每台机器不同）；重复运行脚本会自动清理旧的随机名安装，并以新名字重新安装
* 支持参数：
  * **模式**：`nps` | `npc` | `all`（默认 `all`）
  * **版本**：例如 `v0.34.7`，默认 `latest`
  * **客户端参数**：`npc` 模式下，`-` 开头的参数会透传给客户端服务
* 环境变量：
  * `NPS_INSTALL_MODE` / `NPS_INSTALL_VERSION`：等同对应位置参数
  * `NPS_INSTALL_DIR`：便携模式，仅解压到该目录，不注册服务
  * `NPC_BIN_NAME` / `NPS_BIN_NAME`：强制指定进程名（不使用随机名）
  * `NPS_START=0`：安装后不自动启动
  * `NPS_GH_PROXY`：GitHub 下载加速前缀，如 `https://mirror.ghproxy.com/`
  * `NPS_INSECURE=1`：跳过 TLS 证书校验；`NPS_IPV4=1`：强制 IPv4 下载
* 国内加速示例（脚本可先经 jsdelivr 下载，压缩包通过 `NPS_GH_PROXY` 加速）：

```bash
curl -fsSLo install.sh https://fastly.jsdelivr.net/gh/2016xyz/sysuahb@v0.34.7/install.sh
sudo NPS_GH_PROXY="https://mirror.ghproxy.com/" sh install.sh nps
```

> 💡 **如何找回随机进程名**：随机名以安装输出为准。忘记时可执行 `ls -d /etc/sys????` 列出配置目录——目录内是 `conf/sysuahb.conf` 即服务端、`conf/sysficb.conf` 即客户端；对应的管理命令就是目录名，如 `sudo syskxqz status`。

---

## 2. 发布包安装

二进制发布包适用于 **Windows、Linux、macOS、FreeBSD、Android(Termux)** 等平台。

📌 **下载地址**：[🔗 最新发布页面](https://github.com/2016xyz/sysuahb/releases/latest)

压缩包命名：`<os>_<arch>_server.tar.gz`（服务端 `sysuahb`）、`<os>_<arch>_client.tar.gz`（客户端 `sysficb`）。

> 进程名/服务名跟随二进制文件名。想自定义名字，安装前把二进制改成任意名字即可（Windows 上为 exe 文件名）。

### **2.1 Windows 安装**

> 需要 Windows 10 或更新版本。

**Windows 10/11 用户**：
- [64 位（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_amd64_server.tar.gz)
- [64 位（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_amd64_client.tar.gz)
- [32 位（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_386_server.tar.gz)
- [32 位（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_386_client.tar.gz)
- [ARM64（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_arm64_server.tar.gz)
- [ARM64（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/windows_arm64_client.tar.gz)

📌 **安装方式（解压后进入文件夹）**
```powershell
# NPS Server（服务端二进制为 sysuahb.exe）
.\sysuahb.exe install
.\sysuahb.exe start|stop|restart|uninstall

# 支持自定义配置路径
.\sysuahb.exe -conf_path="D:\test\nps"
.\sysuahb.exe install -conf_path="D:\test\nps"

# 更新
.\sysuahb.exe stop
.\sysuahb.exe update
.\sysuahb.exe start

# NPC Client（客户端二进制为 sysficb.exe）
.\sysficb.exe install -server="xxx:123,yyy:456" -vkey="xxx,yyy" -type="tcp,tls" -log="off"
.\sysficb.exe start|stop|restart|uninstall

# 更新
.\sysficb.exe stop
.\sysficb.exe update
.\sysficb.exe start
```

---

### **2.2 Linux 安装**
📌 **推荐使用 [一键脚本安装](#1-一键脚本安装linux推荐)。**

#### **X86/64**
- [64 位（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_amd64_server.tar.gz)
- [64 位（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_amd64_client.tar.gz)
- [32 位（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_386_server.tar.gz)
- [32 位（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_386_client.tar.gz)

#### **ARM**
- [ARM64（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm64_server.tar.gz)
- [ARM64（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm64_client.tar.gz)
- [ARMv5（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v5_server.tar.gz)
- [ARMv5（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v5_client.tar.gz)
- [ARMv6（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v6_server.tar.gz)
- [ARMv6（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v6_client.tar.gz)
- [ARMv7（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v7_server.tar.gz)
- [ARMv7（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/linux_arm_v7_client.tar.gz)

📌 **安装方式（解压后进入文件夹）**
```bash
# NPS Server（服务端二进制为 sysuahb）
sudo ./sysuahb install
sudo ./sysuahb start|stop|restart|uninstall

# 支持自定义配置路径
sudo ./sysuahb -conf_path="/app/nps"
sudo ./sysuahb install -conf_path="/app/nps"

# 更新
sudo ./sysuahb update && sudo ./sysuahb restart

# NPC Client（客户端二进制为 sysficb）
sudo ./sysficb install -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tcp,tls -log=off
sudo ./sysficb start|stop|restart|uninstall

# 更新
sudo ./sysficb update && sudo ./sysficb restart
```

---

### **2.3 macOS 安装**
- [Intel（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/darwin_amd64_server.tar.gz)
- [Intel（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/darwin_amd64_client.tar.gz)
- [Apple Silicon（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/darwin_arm64_server.tar.gz)
- [Apple Silicon（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/darwin_arm64_client.tar.gz)

📌 **安装方式同 Linux（解压后进入文件夹）**
```bash
# NPS Server（服务端二进制为 sysuahb）
sudo ./sysuahb install
sudo ./sysuahb start|stop|restart|uninstall

# NPC Client（客户端二进制为 sysficb）
sudo ./sysficb install -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tcp,tls -log=off
sudo ./sysficb start|stop|restart|uninstall
```

---

### **2.4 FreeBSD 安装**
- [AMD64（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_amd64_server.tar.gz)
- [AMD64（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_amd64_client.tar.gz)
- [386（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_386_server.tar.gz)
- [386（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_386_client.tar.gz)
- [ARM（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_arm_server.tar.gz)
- [ARM（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/freebsd_arm_client.tar.gz)

---

## 3. Docker 部署（上游镜像）

本仓库不发布 Docker 镜像；如需容器部署可使用上游镜像（与本项目协议完全兼容）。

***DockerHub***： [NPS](https://hub.docker.com/r/duan2001/nps) [NPC](https://hub.docker.com/r/duan2001/npc)

***GHCR***： [NPS](https://github.com/djylb/nps/pkgs/container/nps) [NPC](https://github.com/djylb/nps/pkgs/container/npc)

#### NPS 服务端
```bash
docker pull duan2001/nps
docker run -d --restart=always --name nps --net=host -v <local_conf_dir>:/conf -v /etc/localtime:/etc/localtime:ro duan2001/nps
```

#### NPC 客户端
```bash
docker pull duan2001/npc
docker run -d --restart=always --name npc --net=host duan2001/npc -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tls,tcp -log=off
```

> 有真实IP获取需求可配合 [mmproxy](https://github.com/djylb/mmproxy-docker) 使用。例如：SSH

---

## 4. Android 使用

### **4.1 APK (仅限NPC)**
#### [NPS Client](https://github.com/djylb/npsclient)
#### [Google Play](https://play.google.com/store/apps/details?id=com.duanlab.npsclient)
- [全架构](https://github.com/djylb/npsclient/releases/latest/download/app-universal-release.apk)
- [ARM64](https://github.com/djylb/npsclient/releases/latest/download/app-arm64-v8a-release.apk)
- [ARM32](https://github.com/djylb/npsclient/releases/latest/download/app-armeabi-v7a-release.apk)
- [X8664](https://github.com/djylb/npsclient/releases/latest/download/app-x86_64-release.apk)


### **4.2 Termux 运行**
- [ARM64（Server）](https://github.com/2016xyz/sysuahb/releases/latest/download/android_arm64_server.tar.gz)
- [ARM64（Client）](https://github.com/2016xyz/sysuahb/releases/latest/download/android_arm64_client.tar.gz)。

---

## 5. OpenWrt 使用

#### [djylb/nps-openwrt](https://github.com/djylb/nps-openwrt)

---

## 6. 源码安装（Go 编译）

### **6.1 获取源码**
```bash
git clone https://github.com/2016xyz/sysuahb.git
cd sysuahb
```

### **6.2 编译**
#### **NPS 服务器**
```bash
go build -o sysuahb cmd/nps/nps.go
```

#### **NPC 客户端**
```bash
go build -o sysficb cmd/npc/client.go
```

编译完成后，即可使用 `./sysuahb` 或 `./sysficb` 启动；安装为服务时进程名跟随二进制文件名。

---

## 7. 相关链接

- **最新发布版本**：[GitHub Releases](https://github.com/2016xyz/sysuahb/releases/latest)（= 稳定版 `v0.34.7`）
- **测试版**：[测试版安装指南](install-test.md) · [全部 Releases 列表](https://github.com/2016xyz/sysuahb/releases)
- **Android**：[djylb/npsclient](https://github.com/djylb/npsclient)
- **OpenWrt**：[djylb/nps-openwrt](https://github.com/djylb/nps-openwrt)
- **DockerHub 镜像（上游）**
  - [NPS Server](https://hub.docker.com/r/duan2001/nps)
  - [NPC Client](https://hub.docker.com/r/duan2001/npc)
