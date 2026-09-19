# 参与开发

Bug 报告请附上程序版本、操作系统、复现步骤及去除敏感信息后的错误日志。请勿上传 Codex auth、原始会话或私有缓存。

功能修改建议先开 issue 说明用途。提交 PR 时说明行为变化和验证结果；尽量让一次修改只解决一个问题。

## 环境

- Go 1.27+
- Git 2.31+（同步功能）
- Node.js（前端语法检查）
- Python 3.11+（发布打包）

服务没有第三方 Go 依赖。网页为原生 HTML/CSS/JavaScript，通过 `go:embed` 内嵌，无需 npm 构建。

## 目录

```text
cmd/codex-tally/       CLI 入口
internal/dashboard/       服务、采集、缓存、分享与同步
  cli.go                  命令解析
  server.go               服务装配、鉴权和 HTTP 路由
  public/                 内嵌网页与模板
  pricing.json            内嵌价格快照
  testdata/               合成测试数据
docs/                     使用与维护文档
scripts/                  发布打包
site/                     准备公开的 usage.json
.github/workflows/        检查、Pages 部署与 Release
main.go                   根目录构建的兼容入口
```

应用实现放在一个内部包中，对外入口为 `dashboard.Run`。测试与实现同目录；默认资源由使用它们的包持有。新增包应有独立职责，不按 controller/service/repository 层数拆分。

## 构建与检查

```bash
go build -trimpath -o codex-tally ./cmd/codex-tally
go test -race ./...
go vet ./...
```

根目录的 `go build .`、`go run .` 仍可用；CI 和打包使用 `cmd/codex-tally`。两个入口共享实现及版本注入规则。

修改命令、配置、公开路径或数据格式时，同步更新相关文档。发布流程见[版本与发布](docs/releasing.md)。

中英文文案共用 `internal/dashboard/public/i18n.json`，以中文原文为键。静态页面在初始化时翻译，动态文案通过 `t()` 插入参数，分享模板通过 `Text` 翻译。新增文案同时维护英文条目；不要翻译模型标识、查询参数或公开 JSON 字段。英文使用文档见 [README.en.md](README.en.md) 和 [guide.en.md](docs/guide.en.md)。
