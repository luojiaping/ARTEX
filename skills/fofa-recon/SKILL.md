---
name: fofa-recon
description: 用 FOFA 网络空间测绘做授权范围内的资产发现与攻击面扩线。适用于目标暴露面未知、需要按域名/IP/端口/证书/图标/组织/指纹扩展资产，或需要先估规模再决定扫描优先级时。覆盖 account → stats → search → 存活确认 → 写回资产图的标准流程、查询语法、成本控制与错误处置。
license: MIT
compatibility: 需要 ARTEX 的 fofa_account / fofa_stats / fofa_search / fofa_host 原生工具；Key 在「系统配置 → FOFA」配置。
---

# FOFA 资产侦察（fofa-recon）

FOFA 是**历史快照**，不是实时扫描器。它的价值是「在动手之前，先把目标可能暴露的面铺开」；它的风险是「查询烧额度、结果过期、把别人的资产算进来」。

## 何时用

- 只拿到一个主域 / 公司名 / 单个 URL，需要扩子域、IP、端口、旁站
- 从一个站点扩到同证书、同 favicon、同标题、同 C 段的一批资产
- 确认某指纹 / 组件 / 面板在互联网上的规模，判断是自研还是通用系统
- 先看端口、标题、国家分布，再决定先扫谁

**不适用**：目标不在授权范围；只需要读一个已知页面（直接用 WebFetch）；需要登录后数据；需要实时端口状态（FOFA 可能过期，必须自己探活）。

## 四条铁律

1. **只在授权范围内查**。FOFA 能查全互联网，但任务只能碰授权目标；用 `domain=`、`ip=`、`cert=`、`org=` 把范围钉死，不要用大范围模糊词裸查。
2. **先估规模，再取数据**。永远先 `fofa_stats`，再按结果决定 `size`；不要一上来 `size=10000`。
3. **Key 不出现在输出里**。工具不会回显 Key；你也不要把 Key 写进报告、命令、截图或 `record_fact`。
4. **FOFA 结果必须验证**。`title`/`server`/`port` 都可能过期；写回资产图前，能做存活/指纹确认就先做，不能做的在 `record_fact` 里标注 `inferred`。

## 标准工作流（5 步）

```
1. fofa_account()                       # 0 消耗，看剩余查询/数据额度
2. fofa_stats(query, fields, size=5)    # 估规模 + 看分布，决定值不值得拉
3. fofa_search(query, fields, size)     # 拉明细：小结果 page，大批量 next
4. 存活/指纹确认                          # Bash(curl/httpx/nmap) 或 WebFetch
5. insert_assets(...)                   # 写回资产图；结论写 record_fact
```

### 1) 查额度

```
fofa_account()
```

返回 `remain_api_query`（请求次数）与 `remain_api_data`（返回行数）。两者是分开扣的：
- 翻一页扣 1 次查询；
- 返回 N 行扣 N 个数据量。

`remain_api_query < 1000` 时 `fofa_search` 会在输出里追加 ⚠️ 告警。

### 2) 估规模

```
fofa_stats(query="domain=\"example.com\"", fields="port,title,server", size=5)
```

`distinct` 是去重计数，不是精确总命中；用它判断「几十条 / 几千条 / 几万条」，再决定 `size` 与 `page|next`。

> 有些账号/套餐没有开通 `stats`（返回 `-403 访问权限不足`）。这时改用小 `size` 的 `fofa_search` 试拉，
> 或直接按经验值从 `size=50` 开始；**不要因为 stats 失败就放弃或换大 size 硬拉**。
> `search` 接口本身**不返回命中总数**（返回的 `size` 是本页行数），别把它当总量。

### 3) 取明细

小结果、需要重复定位：

```
fofa_search(query="domain=\"example.com\"", fields="host,ip,port,title,server", size=100, mode="page", page=1)
```

大批量、顺序拉取：

```
fofa_search(query="domain=\"example.com\"", fields="host,ip,port,title", size=1000, mode="next")
# 之后把返回头里的 next=... 填回 next 参数继续
```

> `page` 是 from/size 深翻页，成本随偏移增长，超过约 1 万条容易被 FOFA 限制；
> `next` 是 search-after 游标，适合大批量，失败可用同一游标重放。

### 4) 验证

FOFA 的 `title`/`server`/`status_code` 只是快照。对高价值目标至少做一次：

```bash
curl -sk -m 10 -o /dev/null -w '%{http_code}\n' https://HOST:PORT/
```

确认存活、确认标题/指纹、确认是不是 CDN/蜜罐。验证结果用 `record_fact` 记录（`confidence=observed`）。

### 5) 写回资产图

`insert_assets` 的字段映射见 `references/integration.md`。最重要的三条：

- `ip` 必须是纯 IP；主机名不能填 `ip`
- `host:port` 不是完整 URL；优先用 FOFA 的 `link` 字段填 `url`
- `status_code`/`content_length`/`technologies` 必须是**探测确认过**的，不要从 FOFA 猜

## 工具速查

| 工具 | 必填 | 常用参数 | 返回 |
|---|---|---|---|
| `fofa_account` | — | — | 账号、会员等级、剩余查询/数据额度 |
| `fofa_stats` | `query` | `fields`（默认 `port,title,domain`）、`size`（TopN，默认 5） | `distinct` 去重计数 + `aggs` TopN；**部分账号 -403** |
| `fofa_search` | `query` | `fields`、`size`（默认 50，最大 10000）、`mode=page\|next`、`page`、`next`、`full` | 本页行数 + 游标 + 额度（**不返回总数**） |
| `fofa_host` | `host` | `detail`（默认 true） | ASN/ORG/端口/协议/产品；**部分账号 -403** |

`fields` 用逗号分隔，只接受字母开头的字母/数字/下划线，最多 24 个，非法字段静默丢弃；列顺序 = `fields` 顺序。

## 查询模式（先抄再改）

**单目标扩线**
```
domain="example.com"
domain="example.com" && port="443"
ip="1.2.3.4"
```

**开放目录 / 临时文件服务器**
```
title="Index of /"
body="Directory listing for"
header="Content-Type: text/html" && title="Index of"
```

**指纹 / 组件**
```
app="Apache" && country="CN"
server="nginx" && port="8080"
header="X-Powered-By: ThinkPHP"
```

**证书 / 图标**
```
cert="example.com"
cert="example.com" && port="443"
icon_hash="-247388890"
```

**组织 / ASN / ICP**
```
org="Example Inc"
asn="12345"
icp="京ICP备00000000号"
```

更多模板见 `assets/queries/`；语法细节见 `references/query-syntax.md`。

## 成本纪律

- 顺序固定：`account → stats → search`，不要跳过 stats
- `size`：几十条用 50–200；上千用 `next` 分批；**不要默认 10000**
- 大字段（`banner`/`header`/`body`/`cert`）只在确实需要时取，它们会放大响应体积
- 同一 query 拉过就写回资产图/事实，不要反复查同一批数据
- 所有调用共享一个限流器（默认 1.5 次/秒）；多 worker 并发会排队，不要同时轰炸

## 常见错误

| 现象 | 原因 / 处理 |
|---|---|
| `FOFA 未配置 API Key` | 去「系统配置 → FOFA」填 Key；或检查 `fofa_key` 设置 |
| `账号无效`（-700） | Key 失效或与账号不匹配；重新复制 |
| `查询语法错误`（-1） | 引号/`&&`/括号不配对；先用最简 query 验证再叠加 |
| `额度不足`（-4/-6） | `fofa_account` 看余额；减小 `size` 或收窄 query |
| `请求频率超限`（-5/429） | 客户端已自动重试 3 次；仍失败则调低 `fofa_rate_limit` |
| `未开通连续翻页`（820000） | 账号无 `search/next` 权限，改 `mode=page` |
| `访问权限不足`（-403） | 账号/套餐未开通 `stats`/`host`；改用 `fofa_search`，不要反复重试 |
| 结果为空但 stats 有数 | page 超范围 / next 游标失效 / `full=false` 只查近一年 |
| 表格缺列或错位 | `fields` 里有非法字段被丢弃；按表头对齐 |

## 参考文件

- `references/api.md` — 端点、参数、返回结构、错误码、限流与配额
- `references/query-syntax.md` — 操作符、模糊搜索、type、证书、组合范式
- `references/field-reference.md` — 字段全表与选字段建议
- `references/recipes.md` — 开放目录、LLM 服务、面板、证书拓线、资产刷新
- `references/fofahub.md` — FofaHub 能力边界（哪些不能 API 化）
- `references/integration.md` — 与 ARTEX 工具/资产图的对接细节
- `references/pitfalls.md` — 误报、配额烧穿、翻页坑
- `scripts/fofa_cli.py` — 独立 CLI，用于手工核对（`FOFA_KEY=... python fofa_cli.py account`）
- `assets/queries/` — 可直接粘贴的查询模板
