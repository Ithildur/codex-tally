# 配置

## 环境变量

| 变量 | 默认值 / 用途 |
| --- | --- |
| `HOST` | `127.0.0.1` |
| `PORT` | `4318` |
| `PUBLIC_ORIGIN` | 远程管理端的 HTTPS 地址，如 `https://usage.example.com` |
| `PUBLIC_SHARE` | 设为 `1` 开启匿名分享 |
| `DASHBOARD_PASSWORD` | 固定密码，至少 16 字符；未设置或为空时，每次启动随机生成并输出到控制台 |
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

仪表盘密码与 Codex 登录凭证分开。未指定密码时，成功启动后在控制台输出“本次登录密码”，仅本次运行有效；指定 `DASHBOARD_PASSWORD` 后使用该密码，不输出到日志。程序不读写密码文件。

登录有效期为 12 小时，重启后需要重新登录。`.state-codex-tally` 只用于本机统计缓存等私有状态。旧版 `.state/password` 不再使用，可自行删除；如需沿用旧密码，将其设为 `DASHBOARD_PASSWORD`。

从 0.0.1 升级时，默认状态目录由 `.state` 改为 `.state-codex-tally`，旧目录不会自动迁移。可将 `sessions.json` 移入新目录复用缓存，或直接启动重新扫描；也可用 `DASHBOARD_STATE_DIR` 显式指定旧目录。不要迁移旧密码文件。

Linux/macOS 下状态目录权限为 `0700`，文件为 `0600`。Windows 使用 NTFS 继承权限，建议将程序与状态目录放在自己的用户目录。

账号统计读取 `CODEX_HOME/auth.json` 中的 access token 和 account ID，登录过期后在同一用户下运行 `codex login`。程序不修改登录文件或刷新 token。API Key 模式和 OS keychain/keyring 暂不支持；本机统计、导出和同步均不需要 auth。

服务默认只监听本机。远程访问设置见[远程访问与公开组件](sharing.md)。
