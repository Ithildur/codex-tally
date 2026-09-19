# 配置

## 界面语言

仪表盘登录页、顶部和 Pages 首页提供语言选择。`自动` 使用浏览器语言：中文浏览器显示中文，其余显示英文。显式 URL 参数 `lang=en` 或 `lang=zh-CN` 优先于浏览器保存的选择。切换语言会重新加载页面，保留页签、时间范围以及计算器的 Token 输入、单价和模型选择。翻译文件加载失败或超过 5 秒时临时回退中文，保留语言选择；恢复后刷新即可使用原先选择的语言。语言不会改变时区、数据范围或计费方式。

终端启动提示和配置错误按 `LC_ALL`、`LC_MESSAGES`、`LANG` 的优先级选择语言，中文区域显示中文，其余显示英文。底层系统或上游错误保留原始信息。网页语言设置不改变终端输出。

## 监听地址与端口

```bash
./codex-tally --host 192.168.1.10 --port 8080
# 等价的环境变量配置
HOST=192.168.1.10 PORT=8080 ./codex-tally
```

`--host` 和 `--port` 分别覆盖 `HOST` 和 `PORT`，默认 `127.0.0.1:4318`。端口范围为 1–65535；监听指定 IP 时，该地址必须属于本机。`--host 0.0.0.0` 监听所有 IPv4 接口，`--host ::` 监听 IPv6。

使用本机实际 IP 和配置的端口访问。直接通过 IP 访问不要求设置 `PUBLIC_ORIGIN`；该配置用于 HTTPS 域名反向代理。任意域名不会因监听全部接口而自动放行。

```powershell
.\codex-tally.exe --host 192.168.1.10 --port 8080
```

`--help` 查看启动选项，`export -h` 等查看子命令选项。

## 环境变量

| 变量 | 默认值 / 用途 |
| --- | --- |
| `HOST` | 监听地址，默认 `127.0.0.1`；支持 IPv4、IPv6 和 localhost |
| `PORT` | 监听端口，默认 `4318`，范围 1–65535 |
| `PUBLIC_ORIGIN` | 远程管理端的 HTTPS 地址，如 `https://usage.example.com` |
| `PUBLIC_SHARE` | 设为 `1` 开启匿名分享 |
| `CODEX_TALLY_PASSWORD` | 固定密码，至少 8 个字符；未设置或为空时，每次启动随机生成 12 位密码并输出到控制台 |
| `DASHBOARD_STATE_DIR` | 可执行文件旁的 `.state-codex-tally` |
| `CODEX_HOME` | 当前用户的 `.codex` |
| `TZ` | 系统时区；可指定 `Asia/Hong_Kong` 等 IANA 名称 |
| `PRICING_FILE` | 自定义价格 JSON，见[费用说明](metrics.md#费用) |
| `CODEBURN_CFG` | `~/.config/codeburn/config.json` |
| `FX_CACHE` | `~/.cache/codeburn/exchange-rate.json` |
| `HTTPS_PROXY` / `HTTP_PROXY` / `NO_PROXY` | 上游 HTTP 请求的代理设置 |

启动示例：

```bash
CODEX_HOME=/path/to/.codex TZ=Asia/Hong_Kong ./codex-tally
```

Windows PowerShell：

```powershell
$env:CODEX_HOME = "$env:USERPROFILE\.codex"
$env:TZ = "Asia/Hong_Kong"
.\codex-tally.exe
```

## 数据目录

| 系统 | 默认路径 |
| --- | --- |
| Windows 原生 | `%USERPROFILE%\.codex` |
| Linux / macOS / WSL | `$HOME/.codex` |

`export` 和 `sync` 按以下顺序选择目录：`-codex-home` 参数、`CODEX_HOME` 环境变量、用户默认路径。支持空格、中文和 `~` 前缀；相对路径以工作目录为基准。服务模式使用环境变量或默认路径。

Windows 原生与 WSL 的用户目录相互独立，不自动扫描或合并。使用 WSL Codex 时，在 WSL 内运行程序，或明确指定其可访问的数据目录。

时区数据库已内嵌。CLI 的 `-timezone` 优先于 `TZ`；服务模式使用 `TZ` 或系统时区。若服务返回原生系统时区 `Local`，前端按浏览器本地时间格式化；跨时区远程访问时，应显式设置 `TZ`。

## 登录与状态

仪表盘密码与 Codex 登录凭证分开。未指定密码时，成功启动后在控制台输出“本次登录密码”，仅本次运行有效；指定 `CODEX_TALLY_PASSWORD` 后使用该密码，不输出到日志。程序不读写密码文件。

密码按 Unicode 字符数计算，中文和英文字符均计为一个字符。

升级时将原有的 `DASHBOARD_PASSWORD` 改为 `CODEX_TALLY_PASSWORD`；程序不再读取旧变量。

登录有效期为 12 小时，重启后需要重新登录。`.state-codex-tally` 只用于本机统计缓存等私有状态。旧版 `.state/password` 不再使用，可自行删除；如需沿用旧密码，将其设为 `CODEX_TALLY_PASSWORD`。

从 0.0.1 升级时，默认状态目录由 `.state` 改为 `.state-codex-tally`，旧目录不会自动迁移。可将 `sessions.json` 移入新目录复用缓存，或直接启动重新扫描；也可用 `DASHBOARD_STATE_DIR` 显式指定旧目录。不要迁移旧密码文件。

Linux/macOS 下状态目录权限为 `0700`，文件为 `0600`。Windows 使用 NTFS 继承权限，建议将程序与状态目录放在自己的用户目录。

账号统计读取 `CODEX_HOME/auth.json` 中的 access token 和 account ID，登录过期后在同一用户下运行 `codex login`。程序不修改登录文件或刷新 token。API Key 模式和 OS keychain/keyring 暂不支持；本机统计、导出和同步均不需要 auth。

服务默认只监听本机。远程访问设置见[远程访问与公开组件](sharing.md)。
