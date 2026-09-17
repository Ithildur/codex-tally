# 远程访问与公开组件

无需常驻服务的分享方式见 [GitHub Pages](pages.md)。本页介绍运行中的仪表盘。

## 管理端

默认只允许本机访问。通过 SSH 转发：

```bash
ssh -L 4318:127.0.0.1:4318 用户名@服务器地址
```

然后打开 <http://localhost:4318>。本地转发端口可以更换；服务接受有效端口上的 localhost、127.0.0.1 和 [::1]。

使用域名访问时，设置 HTTPS 地址并配置反向代理：

```bash
PUBLIC_ORIGIN=https://usage.example.com ./codex-tally
```

Caddy：

```caddyfile
usage.example.com {
    reverse_proxy 127.0.0.1:4318
}
```

此时登录 Cookie 带 Secure 标记，需要通过 HTTPS 域名登录。代理不在同机时可设置 `HOST=0.0.0.0`，同时设置 `PUBLIC_ORIGIN`，并限制源站只接受代理访问。

## 开启匿名分享

```bash
PUBLIC_SHARE=1 ./codex-tally
```

管理端仍需登录。公开内容为缓存所属月份的本机总 Token、调用次数、缓存率和模型分布，不含费用、账号额度、活跃时段或会话明细。

启动时已有缓存就生成快照；每次本机缓存成功更新后重新生成。访客读取预生成内容，不能通过参数刷新数据或扩大日期范围。源站声明 5 分钟缓存，嵌入平台可能另行缓存。

| 组件 | 网页 | SVG |
| --- | --- | --- |
| 概览 | `/share` 或 `/share/overview` | `/share/overview.svg` |
| 组合 | `/share/hub?components=tokens,models` | `/share/hub.svg?components=tokens,models` |
| Token | `/share/tokens` | `/share/tokens.svg` |
| 调用次数 | `/share/calls` | `/share/calls.svg` |
| 缓存率 | `/share/cache` | `/share/cache.svg` |
| 模型分布 | `/share/models` | `/share/models.svg` |

主题参数为 `theme=auto`（默认）、`light` 或 `dark`。已有查询参数时用 `&theme=dark` 追加。

组合顺序固定为 `tokens,calls,cache,models`；不传 `components` 时显示全部四项，空值、重复项和未知项返回 400。数值等宽排列，模型表占整行，窄屏转为单列。

## 嵌入

登录后点击“分享”，选择组件、主题和格式，即可预览并复制链接、Markdown 或 iframe。链接使用浏览器当前的协议、域名和端口；从 localhost 复制的链接只能在相应本地环境访问。

```html
<iframe data-codex-usage
        src="https://usage.example.com/share/tokens?theme=light"
        title="Codex 总 Token" width="480" height="240"
        sandbox="allow-scripts" loading="lazy"
        style="display:block;width:100%;max-width:480px;border:0"></iframe>
<script async src="https://usage.example.com/share/embed.js"></script>
```

保留 `embed.js` 以自动同步高度，承载页面需允许加载该脚本。分享面板会按组件组合生成合适的初始尺寸。

```markdown
![Codex 用量](https://usage.example.com/share/overview.svg?theme=dark)
```

单项数值 SVG 为 480 × 180；模型和概览 SVG 宽 640，高度随模型行数变化。SVG 不含脚本，也不依赖外部图片或字体服务。

## 只向外网开放分享

保持默认监听地址，不设置 `PUBLIC_ORIGIN`，仅代理公开路径：

```caddyfile
usage.example.com {
    @public path /share /share/*
    handle @public {
        reverse_proxy 127.0.0.1:4318 {
            header_up Host {upstream_hostport}
        }
    }
    handle {
        respond 404
    }
}
```

管理端仍通过本机或 SSH 访问。Host 重写只用于公开路径。配置参考 Caddy 的 [handle](https://caddyserver.com/docs/caddyfile/directives/handle) 和 [reverse_proxy](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy#headers)。

公开页允许 iframe，管理页禁止 iframe。在线分享没有匿名 JSON API 或写接口。知道地址的人都能访问公开内容；移除 `PUBLIC_SHARE=1` 并重启可关闭源站，但不能撤回已下载或缓存的副本。
