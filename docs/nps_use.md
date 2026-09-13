# 使用

📌 **提示**
- **使用 Web 管理模式时，`nps` 服务器必须在项目根目录运行，否则无法正确加载配置文件。**
- **安装后可执行文件位置可能会发生变化；若通过一键脚本安装，进程名是随机的（`sys`+4位字母），可在 `/etc/sys????` 中根据 `conf/sysuahb.conf`（服务端）或 `conf/sysficb.conf`（客户端）标记找到名字。**

---

## 1. Web 管理界面

📌 **访问 Web 界面**
- 在浏览器中输入 `http://公网IP:8081`（ **默认端口 `8081`** ）
- **默认管理员账号/密码**
  - 用户名：`admin`
  - 密码：`123`（请**修改默认密码**以确保安全，新增TOTP支持）

📌 **Web 界面功能**
- **客户端管理**（添加、删除、编辑隧道）
- **域名转发**（管理 HTTP/HTTPS 代理）
- **流量统计**
- **用户管理**
- **系统配置**
- **日志查看**
- **在线文档**

---

## 2. 服务端配置文件重载

📌 **适用于**
- **修改部分 `sysuahb.conf` 配置后，无需重启即可生效**
- **支持的参数**
  - `allow_user_login`
  - `auth_crypt_key`
  - `auth_key`
  - `web_username`
  - `web_password`
  - **未来将支持更多参数**

### **Linux/macOS**
```bash
sudo sysuahb reload
```

### **Windows**
```powershell
sysuahb.exe reload
```

---

## 3. 服务端停止或重启

### **Linux/macOS**
```bash
sudo sysuahb stop    # Stop sysuahb
sudo sysuahb restart # Restart NPS
```

### **Windows**
```powershell
sysuahb.exe stop    # Stop sysuahb
sysuahb.exe restart # Restart NPS
```

---

## 4. 服务端更新
📌 **适用于**
- **升级至最新版本**
- **修复已知 Bug**
- **获取最新功能**

### **步骤**
1. **先停止 `nps`**
   ```bash
   sudo sysuahb stop # Linux/macOS
   sysuahb.exe stop  # Windows
   ```
2. **执行更新**
   ```bash
   sudo sysuahb update # Linux/macOS
   sysuahb.exe update  # Windows
   ```
3. **重新启动 `nps`**
   ```bash
   sudo sysuahb start # Linux/macOS
   sysuahb.exe start  # Windows
   ```

📌 **如果更新失败**
- **手动下载最新版本**：[🔗 GitHub Releases](https://github.com/2016xyz/sysuahb/releases/latest)
- **覆盖原有 `nps` 二进制文件和 `web` 目录**
- **安装 `nps` 后可执行文件路径可能会改变，使用以下命令查找**
  ```bash
  whereis sysuahb
  ```

---

## 5. 手动覆盖 NPS 可执行文件

📌 **适用于**
- **手动下载 `nps` 二进制文件**
- **`update` 更新失败时**

### **Linux/macOS**
```bash
sudo systemctl stop sysuahb   # Stop sysuahb
whereis sysuahb               # Find sysuahb install path
sudo cp sysuahb /usr/bin/sysuahb  # Replace old binary
sudo chmod +x /usr/bin/sysuahb # Ensure executable permission
sudo systemctl start sysuahb  # Start sysuahb
```

### **Windows**
```powershell
Stop-Service sysuahb   # Stop sysuahb
Copy-Item -Path "PATH_TO_NEW_NPS_EXE" -Destination "PATH_TO_OLD_NPS_EXE_DIR" -Force
Start-Service sysuahb  # Start sysuahb
```

📌 **如遇 `权限不足`，请以管理员身份运行 `PowerShell` 终端执行。**

---

✅ **如需更多帮助，请查看 [文档](https://github.com/djylb/nps) 或提交 [GitHub Issues](https://github.com/djylb/nps/issues) 反馈问题。**
