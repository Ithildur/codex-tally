# 版本与发布

## 版本规则

使用不带 `v` 前缀的严格 SemVer：

| 示例 | 类型 |
| --- | --- |
| `0.1.0` | 正式版 |
| `0.1.0-rc.1` | 预发布，不设为 Latest |
| `0.1.0+build.7` | 正式版，含构建标识 |

不接受 `v0.1.0`、`0.1` 或 `0.1.0-rc.01`。主、次、补丁版本和纯数字预发布标识不允许前导零。

版本通过 `-ldflags "-X main.buildVersion=..."` 注入，`codex-tally --version` 或 `version` 子命令输出版本。普通源码构建默认为 `0.0.0-dev`。

## 发布步骤

先提交并推送源码，等待 CI 通过，再单独推送 tag：

```bash
git push origin main
git tag -a 0.1.0 -m "Release 0.1.0"
git push origin 0.1.0
```

分支名和版本号按实际情况替换。发布要求工作区干净、HEAD 恰好对应一个有效版本 tag。

`.github/workflows/release.yml` 依次完成：

1. Windows、Linux、macOS 的测试与静态检查。
2. 五个系统与架构组合的交叉编译和打包。
3. 上传 SHA256 校验文件，创建 GitHub Release 并生成发布说明。

只有 tag 的 `push` 事件发布 Release。Actions 手动运行只上传构建产物，所选提交也必须有唯一 tag。工作流使用当前仓库的 `GITHUB_TOKEN`，fork 启用 Actions 后可直接使用。

已经发布的 tag 不应移动。Release 已存在时发布失败，不自动覆盖附件。

## 产物

Windows、Linux 提供 amd64 / arm64；macOS 仅提供 Apple Silicon（arm64）版本：

```text
codex-tally_<版本>_<系统>_<架构>.tar.gz
codex-tally_<版本>_windows_<架构>.zip
SHA256SUMS
```

macOS 的系统名为 `darwin`。压缩包内的程序为 `codex-tally` / `codex-tally.exe`，并包含许可证和使用文档；不包含登录凭证、缓存或用量快照。当前不做代码签名或 macOS 公证。

所有压缩包（包括 Windows ZIP）都包含一层 `codex-tally/` 目录：

```text
codex-tally/
  codex-tally          # Windows 为 codex-tally.exe
  LICENSE
  README.md
  README.en.md
  CONTRIBUTING.md
  SECURITY.md
  docs/
```

英文使用文档为 `README.en.md` 和 `docs/guide.en.md`，与中文文档一同打包。

解压后进入该目录启动程序，默认缓存保存在该目录下的 `.state-codex-tally/`。

0.0.1 的程序名为 `codex-dashboard`，升级后需更新服务和定时任务中的执行路径。源码构建入口相应改为 `cmd/codex-tally`；根目录的 `go build .` 仍可用。

## 本地打包

需要 Go 1.27+、Python 3.11+；按 tag 打包还需要 Git。

```bash
python3 scripts/package.py --use-git-tag --release
# 开发构建
python3 scripts/package.py --version 0.0.0-dev --out /tmp/codex-tally-build
```

Windows 用 `py -3` 替换 `python3`，并使用本机输出路径。脚本构建上述五个目标，默认输出到已忽略的 `release/`；输出目录必须为空。普通用户运行程序无需 Python。
