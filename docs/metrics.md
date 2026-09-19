# 统计口径

## 本机统计

读取 Codex 的 `sessions` 和 `archived_sessions`，不读取对话正文用于展示。

- Token 按响应最终的 `token_usage_record` 去重统计，包含上下文压缩。旧日志兼容 `token_count`，两种记录不相加。
- 输入区分未缓存、缓存读和缓存写；推理输出不重复加入总 Token。
- 调用次数按用量记录计数，不等于用户交互轮数或 HTTP 请求次数。
- 活跃时段按记录时间聚合，不代表持续工作时长。

## 账号统计

账号页展示上游返回的每日 Token、客户端分布和额度，与本机会话分开计算。缺失字段显示未知。

“当日模型用量占比”计算为：

```text
该模型及速度的用量值 / 当天全部模型及速度的用量值之和 × 100%
```

显示保留三位小数，合计可能有舍入误差；当天总量为零时显示暂无用量。这是接口用量值的相对占比，不是 Token 占比、费用或订阅额度消耗率。账号接口没有各模型的 Token 分项，因此不计算账号费用。

## 时间与缓存

时间范围为 `[开始, 结束)`。本周从周一开始；最近 7 天包含今天及前 6 个自然日，从第一天零点统计到当前时刻。自定义 7 天窗口包含结束日期及前 6 天。

账号接口只支持按日查询，其日期时区和边界尚未核实。本机额度窗口使用接口返回的时长和重置时间，即 `[重置时间 − 窗口时长, 重置时间)`。Pro 不显示 5 小时筛选，接口未提供的窗口不显示。

| 数据 | 刷新策略 |
| --- | --- |
| 本机会话 | 每小时扫描；进入页签先读缓存，再后台更新；支持手动刷新 |
| 账号统计、顶部额度 | 成功缓存 30 分钟，失败间隔 30 秒重试，保留旧数据 |
| 额度窗口 | 跨过重置时间提前失效 |
| 在线分享 | 本机缓存成功更新后发布新快照 |
| GitHub Pages | 每次成功部署后更新 |

本机缓存持久化到 `.state-codex-tally/sessions.json`，按 Codex 目录隔离；只重读发生变化的文件，并发刷新合并处理。缓存包含用量事件和文件索引，不保存对话正文或凭证。解析版本变化时重建缓存。

账号缓存只在内存中，按账号、接口和参数隔离。页面隐藏时暂停请求，重新可见时恢复；没有打开页面时不轮询账号接口。

## 费用

本机费用依据日志中的模型和逐响应 Token 明细，按 API 单价估算。可识别的长上下文请求使用对应费率，不包含工具调用等额外费用。它不是 ChatGPT/Codex 订阅账单。

内嵌价格表核对日期为 2026-09-19，覆盖 GPT-5 起的 Codex 型号，以及可通过 API 配置使用的通用型号。以下为标准速度、短上下文价格，单位为美元 / 1M Token；`—` 表示未公布该项价格，不能当作免费。

| 模型 | 输入 | 缓存读 | 缓存写 | 输出 |
| --- | ---: | ---: | ---: | ---: |
| GPT-5 / GPT-5-Codex | 1.25 | 0.125 | — | 10 |
| GPT-5 Mini | 0.25 | 0.025 | — | 2 |
| GPT-5 Nano | 0.05 | 0.005 | — | 0.40 |
| GPT-5.1 / GPT-5.1-Codex / GPT-5.1-Codex-Max | 1.25 | 0.125 | — | 10 |
| GPT-5.1-Codex-Mini | 0.25 | 0.025 | — | 2 |
| GPT-5.2 / GPT-5.2-Codex / GPT-5.3-Codex | 1.75 | 0.175 | — | 14 |
| GPT-5.4 | 2.50 | 0.25 | — | 15 |
| GPT-5.4 Mini | 0.75 | 0.075 | — | 4.50 |
| GPT-5.4 Nano | 0.20 | 0.02 | — | 1.25 |
| GPT-5.5 | 5 | 0.50 | — | 30 |
| GPT-5.6 Sol | 4 | 0.40 | 5 | 20 |
| GPT-5.6 Terra | 2 | 0.20 | 2.50 | 12 |
| GPT-5.6 Luna | 0.20 | 0.02 | 0.25 | 1.20 |
| GPT-5.6 Cyber | 12.50 | 1.25 | 15.625 | 75 |
| GPT-6 Astra | 10 | 1 | 12.50 | 50 |

来源：[OpenAI 价格表](https://developers.openai.com/api/docs/pricing)和各型号的官方模型文档；每条记录的来源保存在 [pricing.json](../internal/dashboard/pricing.json) 的 `source` 中。Spark 没有核实到公开美元单价，保留为未知，不套用 GPT-5.3-Codex 的价格。

这些是核对日的价格快照，不会自动更新，也不按历史调用日期还原过去的价格。Sol 使用当前促销价格，官方承诺至少持续到 2026-11-21。`gpt-5.6` 对应 Sol；Daybreak Blue / Red 别名在核对日分别对应 Sol / Cyber。日期后缀 `-YYYYMMDD`、`-YYYY-MM-DD` 使用同型号价格；显式配置的完整型号优先。

### 长上下文与 Fast

GPT-5.4、GPT-5.5、GPT-5.6 系列和 Astra 按单次请求的完整输入量选择价格档位。阈值为 **大于 272,000 输入 Token**，包括缓存读写；输出不参与阈值判断。超过阈值时，该请求的全部输入、缓存读和缓存写乘 2，全部输出乘 1.5。不是只对超出部分加价，也不使用 session 累计 Token 或模型最大上下文窗口判断。GPT-5.4 Mini / Nano 和更早的型号没有套用此规则。

例如 Astra 单次输入 300,000（其中缓存读 250,000）、输出 10,000 Token：费用为 `50,000 × $20/1M + 250,000 × $2/1M + 10,000 × $75/1M = $2.25`。同样的量分散到多个未超阈值的请求，费用不同。压缩后的请求重新按其实际输入量判断。

输入量来自最终 `token_usage_record.usage.input_tokens`，或旧日志的 `last_token_usage.input_tokens`。只有累计值、无法还原单次输入的旧记录按标准价估算。升级后本机解析缓存会自动重建。规则见 [Astra](https://developers.openai.com/api/docs/models/gpt-6-astra)、[Sol](https://developers.openai.com/api/docs/models/gpt-5.6-sol) 和 [API 价格表](https://developers.openai.com/api/docs/pricing)。

本机现有 session 日志没有记录实际处理请求的 `service_tier`，因此不自动叠加 Fast 费用，也不读取当前配置来推断历史速度。API Fast 可能降级到标准处理，应以响应返回的档位为准。API 美元价格与 Codex 订阅额度倍率不同：例如 GPT-5.6 的 API Fast 为标准费率的 2 倍，而 Codex Fast 消耗额度为 2.5 倍；不能混用。见 [API Fast](https://developers.openai.com/api/docs/guides/fast-mode) 和 [Codex 速度](https://learn.chatgpt.com/docs/agent-configuration/speed)。

计算器按填写的单价对 Token 总量试算，导入区间汇总不会把整个区间当作一次长上下文请求。

价格读取顺序为：自定义 `PRICING_FILE` 覆盖同名模型；内嵌表提供默认价格；其他模型从 `~/.cache/codeburn/litellm-pricing.json` 补充。没有价格的模型显示未知，保留可计算部分。支持 CodeBurn 货币配置及匹配的汇率缓存，无匹配汇率时使用 USD。

自定义文件的价格单位是美元/Token，每次读取统计时重新加载：

```json
{
  "updatedAt": "2026-09-19",
  "models": {
    "gpt-6-astra": {
      "inputCostPerToken": 0.00001,
      "cacheReadCostPerToken": 0.000001,
      "cacheWriteCostPerToken": 0.0000125,
      "outputCostPerToken": 0.00005,
      "source": "https://developers.openai.com/api/docs/models/gpt-6-astra",
      "longContext": {
        "aboveInputTokens": 272000,
        "inputMultiplier": 2,
        "outputMultiplier": 1.5
      }
    }
  }
}
```

将 `PRICING_FILE` 指向该文件。同名模型整条替换；省略 `longContext` 表示始终使用填写的单价。缺失的缓存价格不会当作零。项目没有接入自动价格查询服务；新增模型通过该文件补充。
