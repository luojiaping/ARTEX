# 在 ARTEX 中使用 FOFA

> 工具：`fofa_account` / `fofa_stats` / `fofa_search` / `fofa_host`（只读）。
> 默认绑定：四个工具都挂在 worker / planner / mainagent / auto / pentest。
> 配置：**系统配置 → FOFA** 填 Key；底层键 `fofa_key`、`fofa_base_url`（默认 `https://fofa.info`）、`fofa_rate_limit`（0.1–20 次/秒，默认 1.5）。

## 1. 何时用

- **暴露面未知**：只有主域/公司名/一个 URL，要先扩子域、IP、端口、旁站。
- **资产扩线**：从一个站点扩到同 IP、同证书、同 favicon、同标题的一批资产。
- **验证分布**：确认某指纹/组件/证书/图标在互联网上的规模，判断是否自研系统。
- **排优先级**：先看端口、标题、国家分布，再决定扫什么。

不适用：目标不在授权范围；只需要读一个已知页面（直接 WebFetch）；需要登录后数据。FOFA 是**历史快照**，不能替代存活与指纹确认。

## 2. 标准工作流（5 步）

1. `fofa_account()` — 0 消耗，看 `remain_api_query` / `remain_api_data`（`fofa_search` 结果尾部也带 `quota_left`）。
2. `fofa_stats(query, fields="port,title,domain", size=5)` — 估规模、看分布，再定 `size`。若返回 `-403`（账号无该接口），改用小 `size` 的 `fofa_search` 试拉；search 不返回总数，`size` 是本页行数。
3. `fofa_search(query, fields, size, mode)` — 小结果 `mode=page`；大批量 `mode=next`（首次 `next` 留空，之后填返回的游标）。
4. **存活/指纹确认** — 交给 worker 的 Bash（curl/httpx/nmap）或 WebFetch。FOFA 的 `title`/`server` 可能过期，不要直接当现状。
5. `insert_assets` 写回资产图（映射见 §4）。探索结论用 `record_fact`，**只有真实触发成功的漏洞**才用 `report_finding`。

```json
{"assets":[
 {"type":"ip","ip":"1.2.3.4","bound_domains":["www.example.com"],
  "open_ports":[{"port":443,"service":"https"}]},
 {"type":"service","url":"https://www.example.com","service_ip":"1.2.3.4",
  "page_title":"Example","technologies":["nginx/1.25"]}
]}
```

注意：`status_code`/`content_length`/`technologies` 必须是探测确认过的，不要从 FOFA 猜。

## 3. 工具参数速查

| 工具 | 必填 | 常用参数 | 返回 |
|---|---|---|---|
| `fofa_account` | — | 无 | JSON：username/email/isvip/vip_level/remain_api_query/remain_api_data/remain_free_point/fofa_point |
| `fofa_stats` | `query` | `fields` 默认 `port,title,domain`；`size` 默认 5（1–50，每字段 TopN） | 去重计数 + 各字段 TopN + `last_update`；**部分账号 -403** |
| `fofa_search` | `query` | `fields` 默认 13 字段；`size` 默认 50（1–10000）；`mode`=`page`\|`next`；`page`≥1；`next` 游标；`full` 仅 next，默认 false=近一年 | 头部 `mode/page/returned/next/quota_left` + markdown 表格（**无命中总数**） |
| `fofa_host` | `host` | `detail` 默认 true | JSON：host/ip/asn/org/country_name/country_code/protocol[]/port[]/category[]/product[]/domain[]/update_time；**部分账号 -403** |

`fields` 逗号分隔，只接受字母开头的字母/数字/下划线，**最多 24 个，非法字段静默丢弃**；全部非法时回落 `host,ip,port`。列顺序 = `fields` 顺序。

## 4. FOFA 字段 → ARTEX 资产映射

| FOFA 字段 | ARTEX 资产类型 | 填哪个字段 | 备注 |
|---|---|---|---|
| `host` | service / subdomain / ip | `url` / `domain` / `ip` | 形如 `host:port`；带协议头才是完整 URL，优先用 `link` |
| `ip` | ip / service | `ip` / `service_ip` | 必须是纯 IP，主机名不能填 `ip` |
| `port` | ip / service(非 HTTP) | `open_ports[].port` / `port` | 数字；HTTP 端口已含在 `url` 里 |
| `domain` | root_domain / subdomain / ip | `domain` / `bound_domains` | 根域用 root_domain，子域用 subdomain |
| `title` | service | `page_title` | 单行，去 HTML 标签 |
| `server` | service | `technologies` | 如 `["nginx/1.25"]`；append 不覆盖 |
| `protocol` | service | `url` 的 scheme / `service_name` | http/https 体现在 url；ssh/mysql 等填 `service_name`+`port` |
| `icp` | root_domain / subdomain | `icp` | 原样；app 用 `app_icp` |
| `asn` | 无对应字段 | `record_fact.detail` / `add_company_scope.reason` | 用于归属判断，不入资产表 |
| `org` | 无对应字段 | `add_company_scope.reason` / `record_fact` | 组织名是企业归因线索 |
| `link` | service(HTTP) | `url` | 最可靠的 URL 字段 |
| `icon_hash` | service | `favicon_mmh3` | 只填 mmh3 数值；FOFA 的 `icon_hash` 即 favicon mmh3 |
| `cert` | 无对应字段 | `record_fact.evidence` | 确认归属/证书复用，不直接入库 |
| `header`/`banner`/`body` | 无对应字段 | `record_fact.detail` | 体积大，按需取，别整段塞资产 |

`link`/`icon_hash`/`cert`/`header`/`banner`/`body` 不在默认 fields 里，必须在 `fields` 显式请求才会返回；`cert.subject.*` 这类带点字段只能写在 query，不能作为返回字段（会被过滤）。

## 5. 查询模板（按任务类型）

**单目标扩线**
```
domain="example.com"
domain="example.com" && port="443"
ip="1.2.3.4"
```

**开放目录**
```
title="Index of /"
title="Index of /" && country="CN"
body="Directory listing for"
```

**指纹/组件**
```
app="Apache" && country="CN"
server="nginx" && port="8080"
header="X-Powered-By: ThinkPHP"
```

**证书/图标**
```
cert="example.com"
cert="example.com" && port="443"
icon_hash="-247388890"
```

**组织资产（org/asn/icp）**
```
org="Example Inc"
asn="12345"
icp="京ICP备00000000号"
org="Example Inc" && port="443"
```

组合用 `&&` / `||`，字符串加双引号；用 `!=` 排除 CDN/云厂商噪声。

## 6. 成本与限流纪律

- **顺序**：account → stats → search。不要直接上大 size。
- **计费口径**：`fofa_account` 固定 0 消耗；`fofa_host` 是聚合查询，通常不按结果行扣数据量（以 FOFA 官方计费为准）。
- **size**：先看 `stats` 的去重计数与 TopN 决定一次拿还是分页。几十条用 50–200；上千用 `next` 分批；别默认 10000。
- **page vs next**：page 适合少量、可重复定位（`page` 深翻可能被 FOFA 限制）；next 适合顺序大批量，游标更稳。next 首次留空，游标只对同一 query 有效。
- **额度**：每条结果计 1 个数据量（`remain_api_data`）；`remain_api_query` < 1000 时工具会追加 ⚠️ 告警。先算 `size` 再取。
- **不重复**：同一 query 拉过就写回资产图/事实，不要为同一结果反复查。
- **并发**：所有调用共享一个限流器（默认 1.5 次/秒），多 worker 并发会排队，别同时轰炸。

## 7. 常见错误

| 现象 | 原因 / 处理 |
|---|---|
| `FOFA 未配置` | 未填 `fofa_key`，或服务端未注入 Key 提供者；去「系统配置 → FOFA」填 |
| key 无效 / HTTP 401/403 | key 与账号不匹配或已失效；重新复制 Key |
| `查询语法错误`（-1） | 引号、`&&`、括号不配对；先用最简 query 验证再叠加条件 |
| `额度不足`（-4/-6） | 查 `fofa_account`；减小 `size` 或等额度重置 |
| `请求频率超限`（-5/429） | 客户端已自动重试 3 次；仍失败则调低 `fofa_rate_limit`、减少并发 |
| `未开通连续翻页`（820000） | 账号无 `search/next` 权限，改 `mode=page` |
| 结果为空但 stats 有数 | `page` 超出范围；`next` 游标失效/不属于该 query；`full=false` 只查近一年；query 与 stats 不完全一致 |
| 表格缺列/列错位 | `fields` 里的字段名非法被丢弃，或某行短列；按返回表头对齐 |
| 工具列表里没有 `fofa_account` / `fofa_host` | 当前版本未注册进任何 agent 工具集，需代码侧挂载；额度可先用 `fofa_search` 返回的 `quota_left` 观察 |
