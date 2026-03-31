# IPSwitch

一个基于 `Wails v2 + Go + 原生 HTML/CSS/JavaScript` 的桌面网络配置切换工具，支持 Windows、macOS 和 Linux。

项目目标是提供一个简单直接的图形界面，用于快速切换网卡的 `静态 IP` 和 `DHCP`，并支持保存常用网络配置，适合家庭、公司、实验室、宿舍等多场景切换使用。

## 功能特性

- 自动读取本机网卡列表
- 显示网卡状态、描述、MAC 地址
- 一键读取当前网卡配置
- 支持静态 IP / DHCP 模式切换
- 支持填写 IP、子网掩码、网关、首选 DNS、备用 DNS
- 支持保存、载入、删除历史配置
- 本地 JSON 持久化存储
- 内置管理员权限检测
- 操作日志实时显示
- 原生桌面界面，无浏览器依赖

## 技术栈

- 后端: `Go`
- 桌面框架: `Wails v2`
- 前端: `HTML + CSS + JavaScript`
- 系统调用:
  - Windows: `PowerShell`、`netsh`
  - macOS: `networksetup`、`ifconfig`
  - Linux: `nmcli`（NetworkManager）
- 配置存储: `JSON`

## 运行环境

- Windows 10 / Windows 11
- macOS
- Linux（需安装并启用 `NetworkManager/nmcli`）
- Go 1.21+
- Node.js
- Wails CLI
- Microsoft Edge WebView2 Runtime

## 安装依赖

### 1. 安装 Wails CLI

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

如果 `wails` 命令不可用，确认 `GOPATH\\bin` 已加入 `PATH`。

### 2. 安装项目依赖

```powershell
go mod tidy
```

本项目前端使用原生静态资源，没有额外的 npm 前端依赖。

## 开发运行

```powershell
wails dev
```

如果只想快速验证 Go 编译，也可以使用：

```powershell
go build -tags production -ldflags='-H windowsgui' -o IPSwitch.exe .
```

注意：

- 直接使用 `go build .` 会触发 Wails 的构建标签校验错误
- 正确的手工编译方式必须带 `-tags production`

## 生产打包

```powershell
wails build
```

默认构建产物位置：

```text
build/bin/IPSwitch.exe
```

## GitHub Actions

仓库已包含两个 GitHub Actions 工作流：

- `Build`：在 `push` 到 `main/master`、`pull_request`、手动触发时执行 Windows 和 macOS 构建校验
- `Release`：在推送 `v*` tag 时分别构建 Windows 和 macOS 版本，macOS 使用通用二进制生成 dmg，附带校验文件并统一发布 GitHub Release

发布示例：

```powershell
git tag v0.1.1
git push origin v0.1.1
```

如果仓库根目录存在对应版本的说明文件，例如：

```text
RELEASE_NOTES_v0.1.1.md
```

Release 工作流会优先使用该文件作为发布说明；否则自动生成 GitHub Release Notes。

## 使用说明

1. 以管理员身份启动程序
2. 选择目标网卡
3. 点击“读取当前网卡配置”查看当前网络参数
4. 选择模式
5. 如果使用静态 IP，填写 IP、子网掩码、网关、DNS
6. 点击“一键应用配置”
7. 如有需要，可输入配置名称并保存为历史配置

## 权限说明

修改网卡配置需要管理员权限。

如果没有管理员权限：

- 可以打开程序
- 可以读取网卡和历史配置
- 不能成功应用静态 IP 或 DHCP 切换

建议始终使用管理员/root 权限启动。

## 配置存储

历史配置默认保存到：

```text
%AppData%\IPSwitch\profiles.json
```

## 项目结构

```text
ip-switch/
├─ frontend/
│  ├─ index.html
│  ├─ app.js
│  └─ app.css
├─ main.go
├─ go.mod
├─ go.sum
├─ wails.json
└─ README.md
```

## 实现说明

### 后端

`main.go` 负责：

- Wails 应用启动
- 网卡枚举
- 当前网卡配置读取
- 静态 IP / DHCP 切换
- 历史配置读写
- 管理员权限检测
- 隐藏 PowerShell / netsh 子进程窗口

### 前端

`frontend/` 负责：

- 网卡选择与展示
- 模式切换
- 表单输入
- 历史配置管理
- 操作日志显示
- 响应式界面布局

## 已知说明

- Windows 通过 `PowerShell + netsh` 操作网络配置
- macOS 通过 `networksetup` 操作网络配置
- macOS 在普通用户启动时会在应用配置阶段弹出系统管理员授权框，无需整个应用以 `sudo` 启动
- Linux 当前通过 `NetworkManager/nmcli` 操作网络配置；未启用 NetworkManager 的发行版暂不支持
- 构建时如果 Wails CLI 版本高于 `go.mod` 中的 Wails 依赖版本，CLI 会给出版本提示，但不一定影响构建
- 某些第三方网卡驱动如果自身返回异常描述文本，界面中可能仍会显示不规范描述；当前已尽量通过 UTF-8 输出规避常见乱码问题

## 常见问题

### 1. 打开后提示 Wails build tags 错误

请不要使用：

```powershell
go build .
```

请使用：

```powershell
go build -tags production -ldflags='-H windowsgui' -o IPSwitch.exe .
```

或直接：

```powershell
wails build
```

### 2. 打开程序时弹出 PowerShell 窗口

当前版本已在后端隐藏 PowerShell / netsh 子进程窗口。如果你仍遇到此问题，请确认使用的是最新构建产物：

```text
build/bin/IPSwitch.exe
```

### 3. 应用配置失败

优先检查：

- 是否以管理员身份运行
- IP、子网掩码、网关、DNS 格式是否正确
- 网卡当前是否可用

## 开源计划建议

如果你准备公开发布，建议补充以下内容：

- LICENSE
- .gitignore
- 项目截图
- 发布页说明
- 版本号和更新日志

## License

暂未添加。开源前建议补充 `LICENSE` 文件。
