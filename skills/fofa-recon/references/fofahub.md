# FofaHub 与 AI 能力边界

> 结论先行：FofaHub 是 FOFA 数据之上的**可视化工作流（积木块）编排层**，不是一组 API。除"取数"外，其余环节都要在本地重建；现有资料未记载 FofaHub 的公开接口。

## 1. FofaHub 是什么

- **入口/形态**：FOFA 个人中心 →「开放实验室」中的工作流编辑器，拖拽积木块拼流程。
- **定位**：以 FOFA 查询结果为输入，向下串 HTTP 请求、Render DOM、文本分类、截图、ZQ（过滤）、JQ（改写）、输出（网址导航 / Excel）。
- **与裸 API 的关系**：只有 `Fetch FOFA` 块真正调用 FOFA API（选字段、设条数）。条数 >100 时改用你自己的 API 额度。渲染、截图、导航页是平台算力，不对外暴露。
- **计费**：50 F 点/分钟，单次 <10 秒不扣费（资料时点，可能调整）。这印证它是"平台服务"而非"API 封装"。
- **接口现状**：无公开的编排/执行 API → 按**平台专有，无公开 API** 处理。

## 2. 四个能力逐个拆解

### 2.1 登录页发现（Login Discovery）

- **做什么**：FOFA 语句 + 条数 → 取 `host` → 补全 scheme → GET 探活 → 渲染 DOM → 关键词分类（登陆/Login/Password）→ 截图 → 输出网址导航。共 11 块。
- **输入/输出**：输入查询语句与 size；输出可点击导航卡片，字段为 `title`、`screenshot_base64`、`url`、`lastupdatetime`。
- **API 复现**：**部分**。
  - 取数：FOFA 搜索 API（`qbase64` + `fields=host` + `size`；大批量用 `/api/v1/search/next` 持续翻页）。
  - 自建：Playwright/Chromium 渲染 → 关键词判定 → 截图转 base64 → 生成自己的 HTML 索引页。
  - 平台专有：网址导航 UI。
- **坑**：① 只留 `http_status_code==200` 会漏掉返回 401/403 的登录页；② 渲染文本里的 "login" 可能只是页脚链接，会误报；③ 慢在渲染不在取数（10 条约 1 分 05 秒，50 条约 3 分钟）。

### 2.2 独立 IP 筛选（Filter Independent IP）

- **做什么**：只取 `ip` 字段 → 按 ip 去重 → 扁平化，一行一个 IP。共 3 块。
- **判断逻辑**：**字符串级唯一化**，不是"独立/非共享/非 CDN"判定。名字有误导性，别拿它做归属或 CDN 识别。
- **API 复现**：**完全可复现**。`fields=ip` 拉取 → 本地 set 去重 → 逐行输出。
- **缺口**：去重仅限本次返回集（分页需全局去重）；去重会丢 host/port/domain 映射，需要映射就取 `host,ip,port`。

### 2.3 产品聚合（Product Aggregation）

- **做什么**：上传资产清单（一行一个），批量查每个资产的产品与产品分类，聚合成 CSV。
- **输出形态**：`ip | products | categories`，如 `60.28.163.107 | Bootstrap,Log4j2,NGINX,Oracle-JAVA,… | 中间件,其他企业应用,办公自动化系统（OA）,开发框架,…`。
- **API 复现**：**不能**。产品与分类来自 FOFA 自有的产品指纹/分类体系，资料中没有公开查询接口。
- **近似方案**：自建指纹库（header/body/favicon/icon_hash）做识别。但分类口径对不齐，只能叫"自建资产识别"，不能称"FOFA 产品聚合"。
- **另注**：输入是**上传文件**而非查询语句，本身就是平台块形态，API 无对应物。

### 2.4 AI Finder / LLM 资产

- **做什么**：输入 FOFA 语句（默认 30 条）→ 自动筛选 → 找"可在线调用"的 AI 模型接口 → 网址导航输出，每块一个模型。30 条约 1–2 分钟。
- **验证链路**：资料只写"自动筛选"，**未公开判定细节**。同仓 Archive 的无效口径是：看不到 AI 对话界面、访问不了、500/403、要登录/邀请码/验证码 → 更像"渲染后看有没有可用对话界面"，不等于严格的模型 API 验证。
- **复现建议（非文档记载，需实测）**：按特征/端口取候选，再 `GET /api/tags`（Ollama，看 `models` 数组）或 `GET /v1/models`（OpenAI 兼容）；401 视为需鉴权；只有 200 且结构合法才算"可调用"。**只探测，不发推理请求。**
- **API 复现**：**部分**。取数可 API；探测/验证要自写 prober；导航 UI 平台专有。
- ⚠️ **易混**：FOFA AI+（输入企业名，多 agent 出资产关系图）是另一产品，不是 FofaHub。按等级限任务数（个人 1 / 专业 5 / 商业 10 / 企业 50 每天，测试期政策），下载还消耗数据额度；无公开 API。

## 3. 能力对照表

| FofaHub 能力 | 裸 API 可复现 | 复现方式 | 缺口 |
|---|---|---|---|
| Fetch FOFA（取数） | 是 | 官方搜索 API，`fields` 指定字段 | 额度/分页；>100 条走自己 key |
| 独立 IP 筛选 | 是 | 取 `ip` → 本地去重 → 逐行 | 仅返回集内去重；丢映射 |
| 登录页发现 | 部分 | API 取 host + 自建渲染/分类/截图 | 导航 UI、渲染与截图算力 |
| AI Finder | 部分 | API 取候选 + 自写 tags/models 探测 | 判定口径未公开；导航 UI |
| 产品聚合 | **不能** | 自建指纹库近似 | FOFA 产品/分类库与口径 |
| 网址导航 / 截图输出 | **不能** | 自建 HTML 索引 + 截图 | 平台专有输出块 |
| 工作流编排与计费 | **不能** | 自建脚本/编排器代替 | F 点计费、平台执行环境 |
| FOFA AI+（对照，非 FofaHub） | **不能** | 无 | 多 agent 图谱、任务配额 |

## 4. 对 ARTEX 的启示

- **固化为 agent 固定流程**：`查询 → 取数 → URL 归一化 → 探活（2xx/3xx/401 都算候选）→ 去重 → 证据落盘 → 报告`。登录页发现做成 fofa-recon 标准子流程：Playwright 渲染 + 关键词 + 截图 + 自建 HTML 索引，替代平台导航页。独立 IP 去重直接脚本化。
- **只能人工**：查询语句与授权范围确认；截图/页面语义复核（压误报）；AI 模型是否可用、是否允许实际调用；产品分类口径的最终判断。
- **平台专有，不要假装能 API 化**：工作流编排与计费、Render DOM/截图即服务、网址导航输出、产品/分类指纹库、FOFA AI+ 图谱与配额。技能文档统一写「本地重建（local reimplementation）」，禁止写「调用 FofaHub API」。
- **证据纪律**：任何"可调用/存在登录页"的结论必须附请求与响应证据；无证据只写"疑似"。

## 5. 来源

1. `FofaHub/Login Discovery.md`、`Filter Independent IP.md`、`Product Aggregation.md`、`AI Finder.md`
2. `FofaHub/FofaHub_ZH/`：登陆页面查找、独立IP筛选、资产聚合、大模型查找
3. `FofaHub/Archive/Unleash the Power of Workflow….md` 与 `Archive_ZH/探索网络世界的利器（认知篇）.md`（开放实验室、计费、无效站点口径）
4. `FOFA+AI/Next-Generation Attack Surface Inventory - Free Trial Available!.md`（FOFA AI+ 配额）
5. `Basic scenario/Best Approach for Fetching Large Amounts of Asset in FOFA.md`（`/api/v1/search/next` 参数）
