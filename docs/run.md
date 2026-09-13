# **启动指南**

> 💡 **关于进程名**：发布包默认二进制名为 `sysuahb`（服务端）/ `sysficb`（客户端），重命名后安装即可（服务名、安装路径、日志文件均跟随新名字）。若通过一键脚本安装，进程名为随机生成的 `sys????`，下文示例中的 `sysuahb`/`sysficb` 请替换为你实际的名字。

## 1. NPS 服务器

下载并解压 **NPS 服务器端** 压缩包，进入解压后的文件夹。

### **1.1 执行安装**
#### **Linux / macOS**
```bash
sudo ./sysuahb install

# Support custom config path
./sysuahb -conf_path="/app/nps"
./sysuahb install -conf_path="/app/nps"
```
#### **Windows**
以 **管理员身份** 运行 `cmd` 或 `PowerShell`，进入安装目录：
```powershell
sysuahb.exe install

# Support custom config path
.\sysuahb.exe -conf_path="D:\test\nps"
.\sysuahb.exe install -conf_path="D:\test\nps"
```

---

### **1.2 启动服务**
#### **Linux / macOS**
```bash
sudo sysuahb start
```
#### **Windows**
```powershell
sysuahb.exe start
```

📌 **安装后的二进制文件及配置目录**：
- **Windows**
  - 配置文件目录：`C:\Program Files\sysuahb`
  - 二进制路径：安装目录（当前文件夹）
- **Linux / macOS**
  - 配置文件目录：`/etc/sysuahb`
  - 二进制路径：`/usr/bin/sysuahb`

📌 **停止/重启服务**
```bash
sysuahb stop      # Stop service
sysuahb restart   # Restart service
```

📌 **卸载 NPS**
```bash
sysuahb uninstall
```

> **⚠️ Windows 用户请勿删除当前目录下的二进制文件！** `sysuahb.exe` 必须保持在 **原始解压目录** 内，否则无法运行。

---

### **1.3 日志与调试**
📌 **如果发现未启动成功**
- **停止服务后手动运行调试**
  ```bash
  sysuahb stop
  ./sysuahb   # Linux/macOS 运行
  sysuahb.exe  # Windows 运行
  ```
- **查看日志**  
  📌 **日志具体位置在 `sysuahb.conf` 里配置**
  - **Windows**: 运行目录下的 `sysuahb.log`
  - **Linux/macOS**: `/var/log/sysuahb.log`

---

### **1.4 访问 Web 管理端**
- 打开浏览器，访问：
  ```
  http://<服务器IP>:8080
  ```
  （默认 Web 端口为 `8080`）
- 登录：
  ```
  用户名: admin
  密码: 123
  ```
  **⚠️ 正式使用请修改默认密码！**

- **创建客户端** 以便后续连接。

---

### **1.5 手动注册为系统服务（多开适用）**
📌 **直接执行 `install` 命令即可** **自动注册 NPS 为系统服务**。只有需要运行多个实例才需要参考以下内容。

#### **Linux（Systemd）**
📌 **自动安装的服务文件为 `sysuahb.service`**
创建 `systemd` 配置文件（路径：`/etc/systemd/system/sysuahb.service`）：
```ini
[Unit]
Description=NPS Intranet Penetration Server
ConditionFileIsExecutable=/usr/bin/sysuahb
Requires=network.target
After=network-online.target syslog.target

[Service]
LimitNOFILE=65536
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/sysuahb "service"
Restart=always
RestartSec=120

[Install]
WantedBy=multi-user.target
```
**启用并启动服务**
```bash
systemctl enable sysuahb
systemctl start sysuahb
```
📌 **卸载 NPS 服务**
```bash
systemctl stop sysuahb
systemctl disable sysuahb
rm /etc/systemd/system/sysuahb.service
systemctl daemon-reload
```
> **不会使用 `systemctl`？** 请参考 [Systemd 官方文档](https://docs.redhat.com/zh-cn/documentation/red_hat_enterprise_linux/9/html/configuring_basic_system_settings/managing-system-services-with-systemctl_managing-systemd#starting-a-system-service_managing-system-services-with-systemctl)。

---

#### **Windows（SC 命令）**
📌 **Windows 手动注册服务**
以 **管理员身份** 运行 `PowerShell`：
```powershell
cmd /c 'sc create Nps1 binPath= "D:\NPS\sysuahb.exe -conf_path=D:\NPS\" DisplayName= "NPS Server 1" start= auto'
```
**启动服务**
```powershell
sc start Nps1
```
**删除服务**
```powershell
sc stop Nps1
sc delete Nps1
```
> **Windows 注册系统服务后，如需更新，必须先手动停止所有运行的服务。**
> 
> **[微软SC命令指南](https://learn.microsoft.com/zh-cn/windows-server/administration/windows-commands/sc-create)**

---

## 2. NPC 客户端

下载并解压 **NPC 客户端** 压缩包，进入解压目录。

---

### **2.1 获取启动命令**
- **进入 Web 管理端**
- **点击客户端前的 `+` 号**
- **复制启动命令**

---

### **2.2 直接运行（测试用）**
#### **Linux**
```bash
./sysficb -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tls,tcp -log=off
```
#### **Windows**
```powershell
sysficb.exe -server="xxx:123,yyy:456" -vkey="xxx,yyy" -type="tcp,tls" -log="off"
```
> **⚠️ PowerShell 运行时，请用双引号括起命令参数！**

---

### **2.3 安装服务并启动 (支持连接多个服务端)**
#### **Linux**
```bash
./sysficb install -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tls,tcp -log=off
./sysficb start
```
#### **Windows**
```powershell
sysficb.exe install -server="xxx:123,yyy:456" -vkey="xxx,yyy" -type="tcp,tls" -log="off"
sysficb.exe start
```
> **⚠️ PowerShell 运行时，请用双引号括起命令参数！**

📌 **安装后的二进制文件及配置目录**：
- **Windows**
  - 配置文件目录：`C:\Program Files\sysficb`
  - 二进制路径：安装目录（当前文件夹）
- **Linux**
  - 配置文件目录：`/etc/sysficb`
  - 二进制路径：`/usr/bin/sysficb`

> **⚠️ Windows 用户请勿删除当前目录下的二进制文件！** `sysficb.exe` 必须保持在 **原始解压目录** 内，否则无法运行。

📌 **卸载 NPC**
```bash
sysficb uninstall
```

---

### **2.4 手动注册为系统服务（多开适用）**
📌 **直接执行 `install` 命令即可** **自动注册 NPC 为系统服务**。现在支持单实例命令行配置 **多开** 不需要下面手动管理多个实例了。

#### **Linux（Systemd）**
📌 **自动安装的服务文件为 `sysficb.service`**
创建 `systemd` 配置文件（路径：`/etc/systemd/system/sysficb.service`）：
```ini
[Unit]
Description=NPS Intranet Penetration Client
ConditionFileIsExecutable=/usr/bin/sysficb
Requires=network.target
After=network-online.target syslog.target

[Service]
LimitNOFILE=65536
StartLimitInterval=5
StartLimitBurst=10
ExecStart=/usr/bin/sysficb "-server=xxx:123,yyy:456" "-vkey=xxx,yyy" "-type=tcp,tls" "-debug=false" "-log=off"
Restart=always
RestartSec=120

[Install]
WantedBy=multi-user.target
```
**启用并启动服务**
```bash
systemctl enable sysficb
systemctl start sysficb
```
📌 **卸载 NPC 服务**
```bash
systemctl stop sysficb
systemctl disable sysficb
rm /etc/systemd/system/sysficb.service
systemctl daemon-reload
```
> **不会使用 `systemctl`？** 请参考 [Systemd 官方文档](https://docs.redhat.com/zh-cn/documentation/red_hat_enterprise_linux/9/html/configuring_basic_system_settings/managing-system-services-with-systemctl_managing-systemd#starting-a-system-service_managing-system-services-with-systemctl)。

---

#### **Windows（SC 命令）**
📌 **Windows 手动注册服务**
以 **管理员身份** 运行 `PowerShell`：
```powershell
cmd /c 'sc create Npc1 binPath= "D:\tools\sysficb.exe -server=xxx:123,yyy:456 -vkey=xxx,yyy -type=tls,tcp -log=off -debug=false" DisplayName= "NPS Client 1" start= auto'
```
**启动服务**
```powershell
sc start Npc1
```
**删除服务**
```powershell
sc stop Npc1
sc delete Npc1
```
> **Windows 注册系统服务后，如需更新，必须先手动停止所有运行的服务。**
> 
> **[微软SC命令指南](https://learn.microsoft.com/zh-cn/windows-server/administration/windows-commands/sc-create)**

---

## 3. 版本检查
- 服务器端版本：
  ```bash
  nps -version
  ```
- 客户端版本：
  ```bash
  npc -version
  ```
  
---

## 4. 配置管理
- **客户端连接后，在 Web 界面配置穿透服务**
- 参考 [使用示例](/example)
