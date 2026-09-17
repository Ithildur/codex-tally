# Codex Tally

本机 Codex 用量仪表盘。查看 Token、费用估算、活跃时段和账号额度，也可以把选定的统计导出到 GitHub Pages。

- Windows、Linux、macOS，单个可执行文件。
- 本机会话统计与账号统计分开显示。
- 分享单个组件或组合面板，支持网页、SVG、Markdown 和 iframe。
- GitHub Pages 展示离线快照，本机无需开放端口。

## 安装

从本仓库的 [Releases](https://github.com/Ithildur/codex-tally/releases) 下载并解压对应文件：

| 系统 | 文件标识 |
| --- | --- |
| Windows | `windows_amd64.zip` / `windows_arm64.zip` |
| Linux | `linux_amd64.tar.gz` / `linux_arm64.tar.gz` |
| macOS | `darwin_amd64.tar.gz` / `darwin_arm64.tar.gz` |

Apple Silicon 选择 `arm64`，Intel / AMD 64 位处理器选择 `amd64`。Release 附带 `SHA256SUMS`；程序暂未做 Windows/macOS 代码签名。

### 从源码构建

需要 Go 1.27+，没有第三方 Go 依赖或前端构建步骤。

```bash
git clone https://github.com/Ithildur/codex-tally.git
cd codex-tally
go build -trimpath -o codex-dashboard ./cmd/codex-dashboard
```

Windows 将输出文件名改为 `codex-dashboard.exe`。使用 Pages 同步时，请先 fork，再 clone 自己的仓库。

## 快速开始

在运行 Codex 的同一用户下启动：

```bash
./codex-dashboard
```

Windows PowerShell 使用 `.\codex-dashboard.exe`。打开 <http://localhost:4318>，用程序旁 `.state/password` 中的密码登录：

```bash
cat .state/password
```

PowerShell 使用 `Get-Content .state/password`。程序默认读取当前用户的 `.codex`，可通过 `CODEX_HOME` 指定其他目录。账号统计需要文件形式的 `auth.json`；本机统计不需要登录凭证。

`./codex-dashboard --version` 查看版本。下载的程序运行时不需要 Go、Node 或 Python；`sync` 另需 Git 2.31+。

## 发布到 GitHub Pages

1. Fork 并 clone 自己的仓库，启用 Actions。
2. 在 **Settings → Pages → Build and deployment** 中选择 **GitHub Actions**。
3. 将程序放到仓库目录，配置好 Git 身份和认证，在默认分支执行：

```bash
./codex-dashboard sync
```

默认公开本月 Token、调用次数和缓存率。模型明细需显式选择。同步只提交 `site/usage.json`，Pages 地址见 Actions 部署结果。

[同步与分享设置](docs/pages.md) · [每 30 分钟自动同步](docs/scheduling.md)

## 文档

- [配置、数据目录与登录](docs/configuration.md)
- [GitHub Pages 与同步](docs/pages.md)
- [定时任务](docs/scheduling.md)
- [远程访问与公开组件](docs/sharing.md)
- [统计口径、缓存与费用](docs/metrics.md)
- [版本与发布](docs/releasing.md)
- [开发与贡献](CONTRIBUTING.md)
- [安全与隐私](SECURITY.md)

费用是按模型 Token 单价计算的估算值，不是订阅账单。账号统计使用 Codex 登录接口，可能随上游变化。

## 许可证

[GPL-3.0](LICENSE)。
