# FOFA 实战配方

> 用途：目录列表挖掘 / 凭据资产发现。查询均为 FOFA 语法；可直接粘贴的版本见 `queries/*.txt`。
> 规模为量级估计（非实测），标「需 stats」的先跑 `fofa_stats`。
> 若 `fofa_stats` 返回 `-403`（账号未开通），改用小 `size` 的 `fofa_search` 试拉；`search` 不返回命中总数。
> 记法：`=` 包含匹配，`*=` 开模糊（`*`/`?` 通配），`!=` 排除，`&&`/`||` 组合（混用加括号）。

## 0. 通用套路

侦察 → 估规模(stats) → 取明细(search) → 存活/指纹确认 → 入库

| 步 | 动作 | 工具 | 通过标准 |
|---|---|---|---|
| 1 | 选 1 个高选择性特征（icon_hash / fid / cert / 专有 header） | 手工 | 能说清「为什么是它」 |
| 2 | 只聚合不拉明细 | `fofa_stats` | 总量 ≤1e4 继续；>1e5 视为特征失效 |
| 3 | 小 size 试拉，fields 不含 body/banner | `fofa_search` | 前 50 条人工过误报 |
| 4 | 访问根路径/固定 API 核对 title、Server、指纹 | curl / httpx | 200 且特征一致 |
| 5 | 写 host/ip/port/title/server/first_seen | `insert_assets` | 可复现，带证据 URL |

- 先用 `title`/`header`/`port` 缩小，再取 `body`；`body` 是最后手段。
- 单特征必配第二特征：Sidewinder 案例单 `jarm` 有 247,319 条，加 3 个 header 特征后才收敛。
- 结果 >1e5 先怀疑特征选错，不要直接翻页烧额度。

## 1. 开放目录 / 临时文件服务器

**目标特征**

| 特征 | 判据 | 说明 |
|---|---|---|
| 目录索引页 | title/body 含 `Index of /`、`Directory listing for /` | Apache/nginx/python |
| 服务器实现 | `Server: SimpleHTTP` / nginx / Apache / Caddy | 决定目录页样式 |
| 文件清单 | 目录页把文件名列进 body | 可反查 .env/id_rsa/*.sql |
| 时间线 | 目录与文件 mtime | 判断临时服务器活跃期 |
| 证书 | 自签或默认证书 | 可拓同源资产 |

**查询模板**

| 查询 | 规模 | 误报 | size |
|---|---|---|---|
| `title="Index of /"` | 1e6+ | 中 | 50 |
| `body="<h1>Index of /</h1>"` | 1e6 | 中 | 50 |
| `header="SimpleHTTP"` | 1e4–1e5 | 低 | 100 |
| `body="Directory listing for /"` | 1e4–1e5 | 低 | 100 |
| `title="Index of /" && header="Server: nginx"` | 1e5–1e6 | 低 | 100 |
| `body="[To Parent Directory]"` | 1e3–1e4 | 低 | 50 |
| `title="Index of /" && body=".env"` | 1e3–1e4 | 中 | 50 |
| `title="Index of /" && body="id_rsa"` | 1e2–1e3 | 低 | 50 |
| `body="APP_KEY=" && body="DB_PASSWORD="` | 需 stats | 中 | 50 |
| `body="-----BEGIN OPENSSH PRIVATE KEY-----"` | 1e3 | 低 | 50 |
| `title="Index of /" && body=".sql"` | 1e3–1e4 | 中 | 50 |
| `title="Index of /" && body=".zip"` | 1e5+ | 高 | 50 |
| `title="Index of /" && body=".bak"` | 1e3–1e4 | 中 | 50 |
| `body="Powered by h5ai"` | 1e3 | 低 | 50 |

> 完整 21 条（含 .zip/.bak/.git、IIS、Directory Lister、File Browser、地区/时间收窄）见 `queries/open-directory.txt`。

**误报与降噪**
- `Index of /` 可能是自定义首页、教程截图页、蜜罐；必须访问确认列表真实可点。
- `body=".env"` 会命中讲 .env 的博客；加 `title="Index of /"` 或 `header="SimpleHTTP"` 收紧。
- CDN 前置站多为统一错误页，用 `header!="Server: cloudflare"` 排除。
- 同 IP 多端口重复：按 `ip+title` 去重；`type="subdomain"` 只看网站类。
- 只保留 `status_code="200"` 且目录有 ≥1 个可读文件的结果。

**拿到结果后的动作**
1. 先只取 `host,ip,port,title,server,protocol,lastupdatetime`，不取 body。
2. 访问根目录，记录状态码、title、Server、首层文件名（只读，不递归下载）。
3. 命中 `.env`/`id_rsa`/`*.sql` 时只记文件名、大小、mtime；需取证取前 4KB + SHA256，不整包下载。
4. 用 `cert=` / `icon_hash=` / `fid=` 回头拓同源资产（见 §4）。
5. 入库：host、ip、port、path、title、server、first_seen、last_seen、evidence_url、risk_tag。
6. 涉第三方受害者数据：停止下载，保留最小证据，走披露流程。

## 2. AI / LLM 服务暴露

| 服务 | 默认端口 | 识别点 |
|---|---|---|
| Ollama | 11434 | 根路径返回 `Ollama is running` |
| Open WebUI | 3000/8080 | title `Open WebUI`，body `open-webui` |
| vLLM | 8000 | body/文档含 `vLLM`，`/v1/models` |
| LM Studio | 1234 | OpenAI 兼容接口 |
| one-api / new-api | 3000 | title `One API` / `New API` |
| LocalAI / Text Generation WebUI / LibreChat / AnythingLLM / Dify | 3000/7860/3080 | title 即产品名 |

**查询模板**

| 查询 | 规模 | 误报 | size |
|---|---|---|---|
| `body="Ollama is running"` | 1e4–1e5 | 低 | 50 |
| `port="11434" && protocol="http"` | 1e5 | 中 | 50 |
| `title="Open WebUI"` | 1e3–1e4 | 低 | 50 |
| `body="open-webui" && title="Open WebUI"` | 1e3 | 低 | 50 |
| `title="One API"` | 1e3 | 低 | 100 |
| `title="New API"` | 1e3 | 中 | 100 |
| `title="LocalAI"` | 1e2–1e3 | 低 | 50 |
| `title="Text Generation WebUI"` | 1e2–1e3 | 低 | 50 |
| `port="1234" && protocol="http"` | 1e4 | 高 | 50 |
| `title="LibreChat"` | 1e2–1e3 | 低 | 50 |
| `title="AnythingLLM"` | 1e2–1e3 | 低 | 50 |
| `body="vLLM"` | 需 stats | 中 | 50 |

> 完整 16 条见 `queries/ai-services.txt`。

**验证方式（只 GET，不推理）**
- Ollama：`/api/tags`（模型列表）、`/api/version`；OpenAI 兼容：`/v1/models`；Open WebUI：`/api/config`。
- 判据：200 + JSON 结构正确；401/403 说明有鉴权，到此为止。
- 官方站/文档/GitHub 会命中同名 title；用登录页特征或排除官方域降噪。
- 频率 ≤1 req/s，单目标 ≤3 次请求。

**风险边界**
- 允许：根页、`/api/tags`、`/api/version`、`/v1/models`、`/api/config`。
- 禁止：`/api/chat`、`/api/generate`、`/api/pull`、`/api/delete`、`/v1/chat/completions`；不下载模型、不试 key、不批量调用。
- 公网可访问 ≠ 可滥用；模型名只作暴露证据，不做内容生成。

## 3. 面板与代理基础设施

| 面板 | 特征 | 备注 |
|---|---|---|
| x-ui / 3x-ui | title `x-ui`，body `3x-ui` | 登录页，非 API |
| V2Board | title `V2Board` | 机场前台 |
| Marzban | title `Marzban` | 默认 8000 |
| Hiddify | title `Hiddify` | 管理端 |
| v2rayA | title `v2rayA` | Web 管理 |
| Clash 系 | title `Clash`，body `yacd`/`metacubexd` | 控制器默认 9090 |
| sing-box | body `sing-box` | 需实测 |

**查询模板**

| 查询 | 规模 | 误报 | size |
|---|---|---|---|
| `title="x-ui"` | 1e4 | 中 | 100 |
| `body="3x-ui"` | 1e3–1e4 | 低 | 100 |
| `title="V2Board"` | 1e3 | 低 | 100 |
| `title="Marzban"` | 1e2–1e3 | 低 | 50 |
| `title="Hiddify"` | 1e2–1e3 | 低 | 50 |
| `title="v2rayA"` | 1e3 | 低 | 50 |
| `title="Clash"` | 需 stats | 高 | 50 |
| `body="yacd"` | 1e3 | 中 | 50 |
| `body="metacubexd"` | 1e3 | 低 | 50 |
| `port="9090" && title="Clash"` | 1e3 | 中 | 50 |
| `body="sing-box"` | 需 stats | 中 | 50 |

> 完整 14 条见 `queries/panels-proxies.txt`。

**常见误报**
- `Clash` 是常用英文词（游戏、文章、活动页），必须配 `port="9090"` 或 dashboard body。
- `x-ui` 会命中 GitHub/教程/镜像页；加 `status_code="200"`、`type="subdomain"`。
- `New API` / `One API` 会被产品文档、SDK 示例命中。
- 机场面板常套 CDN：先看 `header` 是否 Cloudflare，再判断真实入口。
- 只验证登录页可达与版本横幅；不试默认口令、不爆破、不调用订阅接口。

## 4. 证书与图标拓线

| 字段 | 匹配对象 | 稳定性 | 用法 |
|---|---|---|---|
| `cert="x"` | 证书全文（CN/SAN/O/有效期/序列号） | 高 | 最宽，先捞再收 |
| `cert.subject.cn="x"` | 证书 CN | 高 | 精确到主机名 |
| `cert.subject.org="x"` | 证书 O | 高 | 按组织聚类，注意共享证书 |
| `cert.issuer.cn="R3"` | 颁发者 CN | 低 | 单独用会爆量，必配第二特征 |
| `cert.is_valid` / `cert.is_match` | 有效性 / 与 host 是否匹配 | — | 降噪 |
| `icon_hash="-1438236046"` | favicon mmh3（有符号十进制） | 高 | 同套系统/面板 |
| `fid="..."` | FOFA 聚合特征 ID | 中 | 最像同模板，但不可读 |
| `jarm="..."` | TLS 握手指纹 | 中 | 单用 24 万条（Sidewinder），必配 header |

**四步拓线**
1. 从已知资产取最稳定特征：优先 `cert.subject.org` / `cert.subject.cn`，其次 `icon_hash`、`fid`。
2. `fofa_stats` 估规模；>1e4 就加第二特征（`jarm`、`header`、`port`、`country`）。
3. 做包含性验证：`已知特征 && (domain="已知1" || domain="已知2")`，确认已知 IOC 落在结果内。
4. 存活确认后入库；把最终表达式存成规则，用于长期监控。

**示例**
- 同证书组织：`cert.subject.org="Example Inc" && cert.is_valid=true`
- 同图标面板：`icon_hash="-1438236046" && port="22"`
- 同模板：`fid="qObSo5JrR9FIkIqprX7XKg=="`
- 证书有效期反查：`cert="Internet Widgits Pty Ltd" && cert="2023-06-23 15:59 UTC"`
- 同错误页：`header_hash="-324809210" && icon_hash=""`
- 同 JS：`js_md5="a13f7f208ba534681deadb1ec7a2e54a"`

**FID 注意**：FID 不可读，先看结果页 Top2 样本的 title/domain 再决定加入或排除；排除用一键多选，别手写。

## 5. 资产刷新

**5.1 定期复核自己的资产**
1. 建基线：`domain="x.com"` ∪ `cert="x.com"` ∪ `org="X Corp"`，去重入库。
2. 每周增量：`domain="x.com" && after="2026-10-01"`，只拉 host/port/title/lastupdatetime。
3. 对比快照：新增 host:port、title 变化、证书更换 → 告警。
4. 降噪：`is_domain=true`、`cert.is_valid=true && cert.is_match=true`。

**5.2 资产被 FOFA 收录后的处置**
1. **不要先删资产**：FOFA 是扫描器，删掉后旧数据转为历史数据、不再更新，反而长期留存。
2. 换页面：把敏感内容替换为正常页或 404。
3. 强制刷新：点「Force Refresh」→ 下载校验 txt → 放到该 IP:端口/域名的根目录 → 再点一次 → 最长 48 小时生效（需证明归属）。
4. 无归属证明：替换页面后等下一轮扫描自动覆盖。

## 6. 成本控制

计费：`fofa_search` 每返回 1 行计 1 数据量；`fofa_stats` 只算查询次数；size 默认 50，单页上限 10000；大批量用 `mode=next`，别深翻 page。

| 类型 | 例子 | 成本 | 策略 |
|---|---|---|---|
| 便宜 | `icon_hash`、`fid`、`cert.subject.cn`、`domain`、`jarm`+header | 低（结果少） | 首选入口 |
| 中等 | `title=`、`header=`、`port`+`protocol` | 中 | 配 `country`/`after` 收窄 |
| 贵 | 裸 `body=`、`title="Index of /"` 全量、`port="80"` | 高（百万行） | 先 stats 再加特征 |

**大字段（body/banner/cert/header/structinfo/icon）**
- 默认 fields 不含它们；导出含大字段时单次上限 10 万条。
- 只在候选 ≤1000 行时取；取前用 stats + title/header 缩小。
- 需要全文分析再走页面下载：单次上限 1000 万条，约 1 小时 40 分。

**经验值**：triage size=50；确认后的窄查询 100–200；>1000 必须 next + 明确用途。

## 7. 来源

- `Basic scenario/Threat Hunting in Open Directories.md`（开放目录、FortiBleed、`host=` 溯源）
- `Basic scenario/How to use FOFA to discover free LLM resources_.md`（Ollama、`/api/tags`）
- `Basic scenario/Practical FOFA Asset Expansion- Sidewinder APT.md`（jarm+header 收敛、包含性验证）
- `Basic scenario/A Comprehensive Guide to Threat Intelligence Discovery Using FOFA.md`（证书/JARM/header/banner 拓线、误报处理）
- `Basic scenario/Practical FOFA Asset Discovery- COLDRIVER.md`（banner+port+cert）
- `Basic scenario/Practical FOFA Asset Expansion- APT-C-23 Android Malware.md`（status_code、body 组合）
- `Basic scenario/How to Independently Refresh Your Assets on FOFA.md`（强制刷新）
- `Basic scenario/Asset Inventory from a God's Perspective.md`（资产梳理、fid/icon_hash/sdk_hash）
- `Basic scenario/Elevating FOFA Search Efficiency through Novel Approaches.md`（FID 预览）
- `Cheatsheet/FOFA规则白皮书.md`、`Get Started with FOFA/A Beginner's Guide.md`（type、is_domain）
- 同目录 `query-syntax.md` / `integration.md`（字段与工具约定）
