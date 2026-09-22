# 测试版安装指南

本页说明如何安装 **测试版（pre-release）**。

> 只想用稳定版？请看 [安装指南](install.md)。稳定版安装命令里的版本号固定为 `v0.34.7`，不带任何后缀。

---

## 1. 稳定版与测试版的区别

| | 稳定版 | 测试版 |
| --- | --- | --- |
| 版本号 | `v0.34.7`（无后缀） | `v0.34.7-test7`（带 `-testN` 后缀；历史版本 `-test1` ~ `-test4`） |
| 发布位置 | [releases/latest](https://github.com/2016xyz/sysuahb/releases/latest) | [releases](https://github.com/2016xyz/sysuahb/releases) 列表，标注 **Pre-release** |
| `latest` 是否指向它 | ✅ 是 | ❌ 否（**预发布不占用 `latest`**） |
| 默认安装目标 | ✅ | ❌（必须显式指定版本号） |
| Docker `latest` 标签 | ✅ 有 | ❌ 无（只有版本号标签与 sha 标签） |
| 稳定性 | 稳定，推荐生产使用 | 含未定稿新功能，可能存在问题 |

**关键点：`latest` 永远解析到稳定版 `v0.34.7`。** 测试版不会被自动安装，也不会覆盖稳定版。

---

## 2. 测试版功能对照

不同测试版包含的功能不同，安装前请对照下表选择版本：

| 版本 | 新增内容 |
| --- | --- |
| `v0.34.7` | 稳定版基线：随机进程名、原始统一代理（用户名路由）、Docker 发布 |
| `v0.34.7-test1` | **客户端标签（Tags）** + 统一代理用户名路由重构（标签 / 随机 / 粘滞 / TTL） |
| `v0.34.7-test2` / `v0.34.7-test3` | 迭代修复（文档、pre-release 标记、TTL 边界用例） |
| `v0.34.7-test4` | **外部代理节点**接入统一出口池（Egress：NPS Client + HTTP/SOCKS5 外部代理） |
| `v0.34.7-test5` | 已撤下（tag 已删除，**无法安装**） |
| `v0.34.7-test6` | 修复代理节点页面全部 404、CI 发布竞争、连接数据竞争；客户端二进制指纹清理 |
| `v0.34.7-test7` | 修复安装脚本版本号解析（见 3.3）——`0.34.7-testN` 不再被静默装成稳定版；固定版本安装失败时直接报错而不回退 `latest`；新增本安装文档 |
| `v0.34.7-test8` | **已废弃**：曾经的「全能代理」（SS / SSR / VMess / VLESS / Trojan / TUIC / Hysteria2 / NaiveProxy 分享链接与 Clash 订阅），已于 `test10` 移除，**不建议安装** |
| `v0.34.7-test9` | 统一代理 / 代理节点 **代码审计修复**（12 处缺陷：SSRF、数据竞争、死锁、丢失更新、导入去重失效等） |
| `v0.34.7-test10` | **推荐测试版**：移除全能代理与隧道协议，回到「NPS Client + HTTP/SOCKS5 外部代理」统一出口池；菜单为 统一代理 / 代理节点 / 设置 三项；移除 sing 系列依赖 |

> 建议直接安装 `v0.34.7-test10`。

---

## 3. 一键脚本安装（Linux）

### 3.1 服务端

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7-test7/install.sh | sudo sh -s nps 0.34.7-test10
```

### 3.2 客户端

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7-test7/install.sh | sudo sh -s npc 0.34.7-test10 -server=xxx:123 -vkey=xxx -type=tls
```

### 3.3 版本号写法（重要）

版本号可以带也可以不带 `v` 前缀，脚本会自动补齐：

| 写法 | 结果 |
| --- | --- |
| `0.34.7-test7` | ✅ 自动补为 `v0.34.7-test7` |
| `v0.34.7-test7` | ✅ 原样使用 |
| `0.34.7` | ✅ 自动补为 `v0.34.7`（**装的是稳定版**） |
| `latest` | ✅ 解析到稳定版（**不是测试版**） |
| `0.34.8` 等不存在的标签 | ❌ 报错退出，**不会**静默装成别的版本 |

> ⚠️ **旧脚本的坑（已在 `v0.34.7-test7` 修复）**：`v0.34.7-test6` 及更早的脚本对 `0.34.7-test6` 这类「带后缀但不带 `v`」的写法不做补前缀处理，会去下载不存在的 `releases/download/0.34.7-test6/...`，404 之后再静默回退到 `releases/latest/download/...`，结果**装成稳定版 `v0.34.7`**，看起来成功但功能是旧的（没有标签、没有代理节点）。
>
> 现在：脚本会自动补 `v`；且**固定版本号时不再回退到 `latest`**，下载不到就直接报错退出。
>
> 📌 因此**必须使用 `v0.34.7-test7` 或 `master` 分支的 `install.sh`**（即上面命令里的地址）来安装测试版。`v0.34.7` 与 `v0.34.7-test1`~`test6` 这些旧标签里的脚本仍是旧版，用它们按版本号安装测试版会得到稳定版。

### 3.4 环境变量方式

```bash
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7-test7/install.sh | sudo env NPS_INSTALL_VERSION=v0.34.7-test10 sh -s nps
```

### 3.5 便携模式（只解压，不注册服务）

```bash
NPS_INSTALL_DIR=/opt/nps-test NPS_START=0 sh install.sh nps 0.34.7-test10
```

---

## 4. 发布包安装

下载地址把版本号换成测试版标签即可：

```
https://github.com/2016xyz/sysuahb/releases/download/v0.34.7-test10/<os>_<arch>_server.tar.gz
https://github.com/2016xyz/sysuahb/releases/download/v0.34.7-test10/<os>_<arch>_client.tar.gz
```

例如 Linux amd64：

```bash
curl -fsSL -o server.tar.gz https://github.com/2016xyz/sysuahb/releases/download/v0.34.7-test10/linux_amd64_server.tar.gz
tar xzf server.tar.gz
```

⚠️ **不要**用 `releases/latest/download/...` 下载测试版——那个地址只会给稳定版。

---

## 5. Docker 测试版

预发布不发布 `latest` 标签，必须显式指定版本号：

```bash
docker pull ghcr.io/2016xyz/sysuahb:0.34.7-test10
docker pull ghcr.io/2016xyz/sysuahb:0.34.7-test10
```

> Docker 标签用的是 **不带 `v`** 的 semver 形式（`0.34.7-test7`），而 GitHub 发布标签带 `v`（`v0.34.7-test7`），注意区分。
> `ghcr.io/2016xyz/sysuahb:latest` 始终是稳定版（`0.34.7`）。
>
> ⚠️ 镜像仓库里可能残留**已撤下的测试版标签**（例如 `0.34.7-test5` 对应的 GitHub tag 已删除）。这类镜像仍可 `docker pull` 到，但不再有对应的发布包与版本记录，**请勿使用**。

---

## 6. 验证装到的是测试版

```bash
# 二进制自报版本应与请求一致
<安装名> -version

# 稳定版 v0.34.7
# => Version: 0.34.7

# 测试版
# => Version: 0.34.7-test10
```

还可以确认测试版独有功能是否到位：

```bash
# 测试版 test4+ 才有的代理节点
strings <安装名> | grep -c ProxyNode     # > 0 即为测试版
```

Web 管理端登录后，左侧「统一代理」菜单下应能看到 **统一代理 / 代理节点 / 设置** 三项。

---

## 7. 回滚到稳定版

```bash
# 用稳定版脚本重装（会清理旧的随机名安装并生成新名字）
curl -fsSL https://raw.githubusercontent.com/2016xyz/sysuahb/v0.34.7/install.sh | sudo sh -s all v0.34.7
```

Docker 回滚：

```bash
docker pull ghcr.io/2016xyz/sysuahb:latest
```

> ⚠️ 测试版新增的配置项（如客户端标签、代理节点）在回滚到稳定版后**不会被识别**，配置会保留在文件里但界面上不可见；如需彻底回滚请先备份 `/etc/<name>/conf/`。

---

## 8. 注意事项

- 测试版为 **预发布**，不保证配置兼容性，生产环境请优先使用稳定版。
- 测试版之间的 tag 可能被删除或重建（如 `test5` 已撤下），**回滚前请确认目标版本号存在**：`git ls-remote --tags https://github.com/2016xyz/sysuahb.git`，或在 [Releases](https://github.com/2016xyz/sysuahb/releases) 页面确认。
- 报错时贴版本号（`-version` 输出）与日志，便于定位。
