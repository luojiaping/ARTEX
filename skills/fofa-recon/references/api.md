# FOFA API 参考

> FOFA API v1；默认 `base_url=https://fofa.info`；全部 GET。以下端点均需 `key`，除 info/my 外可选 `lang`；`key` 只走 query string。

## 1. 认证与通用参数

| 参数 | 说明 |
|---|---|
| `key` | 必填；账号 API Key。禁止入日志/截图/URL |
| `lang` | `zh-CN`（默认）/`en`；只影响 errmsg 文案 |
| `qbase64` | 查询语句**标准 Base64**（非 URL-safe），入 query 前 URL 编码 |
| `fields` | 逗号分隔，默认 `host,ip,port`；顺序=results 列顺序 |
| `full` | 小写 `true`/`false` |

`base_url` 可换反代/镜像，路径不变。响应含 `error`；失败另有 `errmsg`，部分带 `code`。

## 2. 端点总表

| 端点 | 方法 | 用途 | 关键参数 | 消耗 |
|---|---|---|---|---|
| `/api/v1/info/my` | GET | 账号与余额 | `key` | 0 查询 |
| `/api/v1/search/all` | GET | 分页搜索 | `qbase64,page,size,fields` | 1 查询 + N 行数据 |
| `/api/v1/search/next` | GET | 游标深翻页 | `qbase64,size,next,full,fields` | 1 查询 + N 行数据 |
| `/api/v1/search/stats` | GET | 聚合、估规模 | `qbase64,fields,size` | 1 查询（待验证） |
| `/api/v1/host/{host}` | GET | 主机/IP/域名聚合 | `detail` | 1 查询（待验证） |

## 3. 逐端点说明

### 3.1 `/api/v1/info/my`

| 参数 | 说明 |
|---|---|
| `key` | 必填 |

| 字段 | 说明 |
|---|---|
| `username`/`email` | 用户名/邮箱 |
| `isvip`/`vip_level`/`is_verified` | 会员/等级/认证 |
| `fcoin`/`fofa_point`/`remain_free_point` | F币/F点/剩余免费点 |
| `remain_api_query`/`remain_api_data` | 剩余查询次数/数据条数 |

```json
{"error":false,"username":"demo","remain_api_query":99432,"remain_api_data":790305}
```
不消耗查询额度。

### 3.2 `/api/v1/search/all`

| 参数 | 说明 |
|---|---|
| `qbase64` | 必填，标准 Base64 |
| `page` | 可选，默认 1 |
| `size` | 可选，默认 100，最大 10000 |
| `fields` | 可选 |

| 字段 | 说明 |
|---|---|
| `mode` | 模式，如 `extended` |
| `page` | 回显 |
| `size` | **本页返回行数**（不是命中总数） |
| `consumed_fpoint`/`required_fpoints` | 本次消耗/所需 F 点 |
| `query` | 原始语句（非 Base64） |
| `results` | 二维数组，列序=fields |
| `next` | 可能不存在（待验证） |

```json
{"error":false,"mode":"extended","page":1,"size":2,"query":"app=\"Apache\"","results":[["1.1.1.1:80","1.1.1.1","80"]]}
```
编码：`base64.b64encode(q.encode()).decode()`，再由 HTTP 层编码。深翻页受 ES `max_result_window`（默认 1 万）约束，成本随偏移增长；超过约 1 万条改用 `next`。

### 3.3 `/api/v1/search/next`

| 参数 | 说明 |
|---|---|
| `qbase64` | 必填 |
| `size` | 可选，默认 100，**最大 10000** |
| `next` | 可选，游标；不传=第一页 |
| `full` | 可选，是否获取全部数据，默认 `false`；范围待验证 |
| `fields` | 可选 |

| 字段 | 说明 |
|---|---|
| `page`/`size`/`query` | 同 `search/all` |
| `results` | 二维数组 |
| `next` | 游标，字符串或数字 |

```json
{"error":false,"results":[["1.1.1.1:80","1.1.1.1","80"]],"next":1734567890123}
```
大规模实时拉取；失败可用同一 `next` 重试，不丢页。`next` 为空/缺失或结果为空即结束；游标有效期待验证。

### 3.4 `/api/v1/search/stats`

| 参数 | 说明 |
|---|---|
| `qbase64` | 必填 |
| `fields` | 必填，统计字段，逗号分隔 |
| `size` | 可选，聚合桶数，默认 5（客户端）；上限待验证 |

| 字段 | 说明 |
|---|---|
| `distinct` | 字段→去重计数，如 `{"ip":1717}` |
| `aggs` | 字段→`[{name,count}]` Top 桶 |
| `lastupdatetime` | 最后更新时间 |

```json
{"error":false,"distinct":{"ip":1717},"aggs":{"title":[{"name":"登录","count":35}]}}
```
`distinct` 是去重计数，不是精确总命中；用它估规模、判断是否超出数据额度，再决定 `search` 的 `size` 或收窄语句。

### 3.5 `/api/v1/host/{host}`

| 参数 | 说明 |
|---|---|
| `host` | 必填，路径参数：IP/域名，需 URL 转义 |
| `detail` | 可选，`true` 返回详细信息，默认 `false` |

| 字段 | 说明 |
|---|---|
| `host`/`ip` | 主机名/IP |
| `asn`/`org` | ASN/组织 |
| `country_name`/`country_code` | 国家名/国家码 |
| `protocol`/`port` | 协议数组/端口数组 |
| `category`/`product`/`domain` | 分类/产品/关联域名数组 |
| `update_time` | 更新时间 |

```json
{"error":false,"host":"example.com","ip":"1.1.1.1","asn":13335,"protocol":["http","https"],"port":[80,443]}
```
`detail=true` 的 banner/header/cert 扩展字段待验证。

## 4. 错误码与处置

| code / errmsg | 含义 | 动作 | 重试 |
|---|---|---|---|
| `-700`（`[-700] 账号无效`） | Key 无效/失效 | 停用并核对 Key | 否 |
| `-5` / HTTP 429 | 频率超限 | 指数退避 1/2/4s，降 QPS | 是 |
| `-4` | 查询额度不足 | 停止 search；用缓存/页面下载 | 否 |
| `-6` | 权限/配额不足（文案待验证） | 查会员权限与余额 | 否 |
| `-1` | 语法/参数错误 | 校验 qbase64、fields、page/size | 否 |
| `-403` | 访问权限不足（stats/host 等接口未开通） | 改用 search；勿重试 | 否 |
| `820000`（code） | 未开通连续翻页 | 改用 `search/all` 或开通；勿循环 | 否 |
| 401/403 | 认证/权限失败 | 核对 Key 与权限 | 否 |
| 5xx/超时 | 服务端/网络故障 | 退避；`next` 可原样重放 | 是 |

以 `code` 优先；无 `code` 时从 `errmsg` 提取 `[(-?\d+)]`，勿匹配文案。`error=true` 也可能 HTTP 200，须解析响应体。

## 5. 限流、额度与翻页策略

- **查询额度 vs 数据量额度**：`remain_api_query` 按请求次数扣，翻一页算一次；`remain_api_data` 按返回行数扣。10 万条用 100 条/页=100 次查询+10 万行；1 万条/页=10 次查询+10 万行。数据额度相同，查询额度差 10 倍。
- **深翻页优先 `next`**：`search/all` 是 from/size，成本随偏移增长，受 ES 默认 1 万窗口限制，易超时/错页；`next` 是 search-after 游标，无深偏移，失败可用同一游标重试。
- **先 stats 再 search**：用 `distinct`+`aggs` 估规模；接近 `remain_api_data` 时先收窄或改页面下载。
  若 stats 返回 `-403 访问权限不足`（部分套餐），改用小 `size` 的 search 试拉；**search 不返回命中总数**，其 `size` 是本页行数。
- **单页 size**：轻字段 1000–5000；带 `banner/header/cert/body/structinfo/icon` 用 100–500，避免超大响应或触发客户端 64 MiB 上限。
- **限速**：官方未给统一 QPS；ARTEX 客户端默认 1.5 req/s、60s 超时、3 次重试（`client.go:36-38,64`）。
- **超大数据**：页面下载单次最高 1000 万条（约 1h40m，压缩包）；含大字段单次最多 10 万条。不走 API 额度，但非 API 端点。

## 6. 与官方 SDK 的差异/坑

- **认证**：v1 用 `key`；旧博客的 `email` 可省略。Key 在 URL 中，日志/代理/异常必须脱敏。
- **`nextid` vs `next`**：博客称 `nextid`，实际请求与响应字段均为 `next`。
- **`next` 类型**：可能是 JSON 数字或字符串，统一 `str()` 后回传。
- **错误体**：HTTP 200 + `{"error":true}` 是常态；`code` 可能缺失，从 `errmsg` 的 `[-700]` 提取，别只看 HTTP 状态。
- **`full`**：必须小写 `true`/`false`；Python `str(True)` 会得到 `True`。
- **Base64**：标准 Base64；`+`、`/`、`=` 交给 HTTP 层编码，避免双重编码。
- **gzip**：`requests`/Go 默认解压；手写 HTTP 且发 `Accept-Encoding: gzip` 时须自行解压，否则 JSON 失败。
- **`fields`**：`results` 是数组的数组；列序错位不报错，只会静默取错值。字段名白名单 `[A-Za-z][A-Za-z0-9_]*`。
- **重试**：`next` 重放安全；`search/all` 超时重试可能重复/跳页，落库按 `host` 或 `ip:port` 去重。

## 7. 来源

- `D:\fofa\Awesome-FOFA\Basic scenario\Best Approach for Fetching Large Amounts of Asset in FOFA.md:10,14,19-21,30,50`；中文版 `D:\fofa\Awesome-FOFA\Basic scenario\Basic scenario_ZH\FOFA大数据量拉取的最佳方案.md:10,15,20-22,30,55`
- `D:\Artex\ARTEX\fofa\client.go:30-38,64,92-100,102-145,147-166,168-182,184-220,222-289`；`types.go:11-23,25-51,59-82,84-111,116-131`；`client_test.go:23-43,45-72,74-89,91-111,113-131,140-165`
