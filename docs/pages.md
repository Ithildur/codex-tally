# GitHub Pages

本机导出统计，由 Git 提交到自己的仓库，Actions 再构建静态站点。本机可以离线，也不需要常驻服务；Pages 保留最近一次成功部署的快照。

## 首次部署

1. Fork 本项目并 clone 自己的仓库，启用 Actions。
2. 在 **Settings → Pages → Build and deployment → Source** 选择 **GitHub Actions**。
3. 下载或编译程序，配置 Git 的提交身份和远端认证。
4. 在仓库默认分支执行 `./codex-dashboard sync`。

Windows 使用 `.\codex-dashboard.exe sync`。程序不在仓库目录时，通过 `-repo` 指定路径。

工作流使用仓库的默认分支和 Actions 自带令牌，不要求额外的 GitHub token 或 Codex secret。尚无 `site/usage.json` 时跳过部署；PR 和其他分支仅检查。部署地址见 Actions，通常为 `https://<owner>.github.io/<repo>/`。

## 选择公开数据

默认公开本月总 Token、调用次数和缓存率：

```bash
./codex-dashboard sync
# 加上模型明细
./codex-dashboard sync -components tokens,calls,cache,models
# 只公开 Token
./codex-dashboard sync -components tokens
```

组件选择应在后续同步、定时任务中保持一致。分享页的勾选只改变展示，不会撤销已导出的字段。

只统计当前月份，月份边界由 `-timezone`、`TZ` 或系统时区确定。公开数值不变时保留原文件和采集时间，不产生空提交。多台电脑写入同一个快照会互相覆盖，当前不合并多设备数据。

## 离线导出

```bash
./codex-dashboard export -out site/usage.json
```

`export` 不需要网络或 Git。把文件复制到联网电脑上的仓库，再同步提交：

```bash
git add site/usage.json
git commit --only -m "Update public usage" -- site/usage.json
git push
```

## 同步与故障恢复

```bash
./codex-dashboard sync -repo /path/to/codex-tally -codex-home /path/to/.codex -log /path/to/codex-tally/.state/sync.log
```

`sync` 只管理 `site/usage.json`，使用当前分支的 upstream 和 Git 已有认证。它先导出、校验，再 fetch，最后提交和推送。其他暂存与未暂存改动保持不变；不强推、不合并、不 rebase，也不推送标签。

| 情况 | 处理 |
| --- | --- |
| 公开数值未变化 | 不创建提交 |
| fetch 失败 | 保留导出的 JSON，下次重试 |
| push 失败 | 保留本地同步提交，下次重试 |
| 远端领先或分叉 | 手动更新分支并解决冲突 |
| 有未推送的非同步提交 | 先审查并推送源码，避免同步顺带发布其他内容 |
| 待推送历史包含已取消的公开字段 | 整理这些提交后再同步 |
| fetch / push 地址不同 | 配置为同一个远端地址 |
| 提示同步锁已存在 | 确认无同步进程后，删除提示中的锁目录 |

Git hooks 和签名设置仍然生效。任务限时 10 分钟，同一仓库不能并发同步。定时任务必须能无交互完成认证与签名；日志可写到 `.state/sync.log`，长期运行需自行轮转。

日志不打印 Git 原始远端错误，避免暴露 URL 中的凭证。排查认证时，在同一用户下手动运行 `git fetch` / `git push`。各系统定时配置见[定时任务](scheduling.md)。

## 分享组件

Pages 首页可以选择组件、主题和复制格式。支持网页、SVG、Markdown 和 iframe，链接使用当前浏览器地址，兼容仓库子路径和自定义域名。

| 内容 | 相对 Pages 根目录的地址 |
| --- | --- |
| Token 网页 | `tokens/auto.html` |
| Token 图片 | `tokens/dark.svg` |
| 组合网页 | `hub/tokens-calls/light.html` |
| 组合图片 | `hub/tokens-calls-cache/auto.svg` |
| 公开数据 | `usage.json` |

主题支持 `auto`、`light`、`dark`。只生成已导出组件及其组合，下一次成功部署会删除已取消字段对应的页面。数值组件横排，窄屏纵排，模型表占整行；iframe 通过附带的 `embed.js` 调整高度。

## 本地预览

```bash
./codex-dashboard build-pages -input site/usage.json -out _site
python3 -m http.server 8080 --directory _site
```

打开 <http://localhost:8080/>。输出目录必须为空；重新构建时删除旧目录或指定新目录。无真实数据时可用 `internal/dashboard/testdata/public-usage.json` 作为输入。

构建只读取公开 JSON，拒绝未知字段、无效版本和超过 1 MiB 的输入。Actions 只上传 `_site`。Git 历史中的旧快照不会因取消组件而消失，公开范围见[安全与隐私](../SECURITY.md)。
