---
name: credential-hunting
description: 授权范围内的只读凭据挖掘闭环：目录/配置文件取证 → AI 判定 LLM Key/OAuth/成对凭据/私钥/代理节点 → 加密台账去重 → 最小验活 → 报告。适用于开放目录、暴露配置、面板/订阅文件、配置备份等目标。
license: MIT
compatibility: 需要 hunt_batch / hunt_fetch / cred_submit / cred_list / cred_verify / proxy_submit / proxy_probe / hunt_report；可选 insert_assets / list_assets。
---

# 授权凭据挖掘（credential-hunting）

目标不是“抓得多”，而是**在授权范围内找到真凭据、保留证据、最小验证、可复核入库**。

## 输入

| 输入 | 处理 |
|---|---|
| URL 列表 | 直接 hunt_batch |
| 资产图 | `list_assets` 取 service URL，再 hunt_batch |
| FOFA 结果 | 资产搜索 agent 交过来的 host:port/link |
| 单个文件 URL | 直接 hunt_fetch |

先复述范围，并确认 `authorization_note`（谁授权、覆盖哪些目标）。用户说“只看列表 / 先别验证 / 先别入库”时，**停在该阶段**。

## 阶段 1：目录扫描

```
hunt_batch(urls=[...], authorization_note="...", max_entries=60)
```

返回每个目标的：
- `is_listing`：是否识别为列目录
- `candidates`：按分值排序的候选文件
- `evidence_path`：目录页证据

先看候选，不要把所有文件都抓一遍。列目录页本身不是凭据。

## 阶段 2：选文件

优先级从高到低：

1. **密钥/凭据文件**：`.env`、`.env.*`、`id_rsa`、`id_ed25519`、`.git-credentials`、`*.key`、`*.pem`、`*.p12`
2. **应用配置**：`config.json/yaml`、`settings.*`、`auth.json`、`.credentials.json`、`providers.json`、`secrets.*`、`application.*`
3. **代理/订阅**：`clash*.yaml`、`mihomo*.yaml`、`xray*.json`、`sing-box*.json`、`sub*`、`subscribe*`、`nodes.*`、`*.ovpn`、`*.conf`
4. **面板/数据库**：`docker-compose.yml`、`wp-config.php`、`database.php`、`.npmrc`、`.pypirc`、`*.sql`、`*.db`、`*.bak`
5. 其他：日志、备份、压缩包（只在明确指向配置时抓）

**不要抓**：图片、CSS、JS、字体、音视频、普通静态页。

## 阶段 3：取证

```
hunt_fetch(url, authorization_note="...", run_id="...")
```

- 返回最多 8KB 文本 + 证据路径。
- 需要看完整内容时用 Read 读证据文件；同一 URL 不重复抓。
- 只读 GET；不访问 `/admin`、`/api` 等无关路径；不扫端口。
- 遇到 401/403：记“需认证”，不爆破、不绕 WAF。

## 阶段 4：AI 判定

对每个候选凭据做**四问**：

1. **值完整吗**？非截断（`...`）、非占位符（`xxx`、`your-`、`changeme`、`<...>`、`${...}`）。
2. **同组字段整包了吗**？
   - OAuth/Codex/Claude：`access_token` + `refresh_token` + `id_token` + `account_id` 整包。
   - AK/SK、client_id/client_secret、用户名/密码必须成对。
3. **来源/用途确定吗**？`source_url` + `evidence_path` 必填；能确定厂商或可写 `base_url`。
4. **是 IM 机器人吗**？Telegram/Discord/飞书/微信/钉钉/Slack/QQ Bot 一律只汇报、不入库。

判定依据要写进 `context`：文件、字段、上下文含义。判不准就标“存疑”，不硬塞。

## 阶段 5：入库

```
cred_submit(entries=[
  {value, kind, provider?, service, source_url, evidence_path, risk?, context?}
])
```

- `kind`：`llm_key | oauth | api_key | token | password | private_key | proxy_node | proxy_subscription | other`
- OAuth/令牌包把完整 JSON 放 `package`（或 `value`），不要只取一行。
- 代理节点/订阅走 `proxy_submit`，不要塞进 `cred_submit`。
- 返回 `inserted / duplicate / excluded / skipped`；`duplicate` 不要再换目标重试。
- 入库前可 `cred_list` 查重；同一值只留一条，来源由服务端追加。

## 阶段 6：最小验活

```
cred_verify(uids=[...])          # LLM Key / OAuth
proxy_probe(uids=[...])          # 代理节点
```

- LLM：先 `GET /models`（不消耗生成额度），不可用才 `max_tokens=1` 的 hi 请求。
- 401/403 判失效；402 余额不足；429 限流；5xx/网络错误判未知。
- **失败不重试**；同一 key 不反复验。
- 密码、私钥、SSH、数据库、云 AK：**只登记，不主动登录**。
- 用户说“先别验证” → 全部停在 `unverified`。

## 阶段 7：报告

```
hunt_report(task_id=..., limit=50)
```

报告结构：

```markdown
## 凭据挖掘报告 · <范围>
- 目标：n 个（列目录 m 个）
- 抓取文件：n 个
- 入库：n（新增 a / 重复 b / 排除 c / 跳过 d）
- 类型分布：llm_key x、oauth y、proxy_node z、其他 w
- 验活：有效 x / 失效 y / 未知 z
- 代理节点：存活 x / 失败 y
- 仅登记未验活：n
- 证据：...
```

只写真实结果；没有的写 0 或“未做”。

## 硬规则

1. **授权优先**：只处理用户明确给出的目标；范围外只报告。
2. **只读**：GET/HEAD；不爆破、不上传、不写入、不利用、不登录。
3. **最小验证**：LLM 最多 1 token；代理只访问 204。
4. **不泄密**：对话/报告只用 `masked`；不要把明文复述到聊天或 Bash 命令里。
5. **不碰 IM**：机器人 Token 一律不入库。
6. **整包/成对**：OAuth 整包、AK/SK 成对。
7. **不造假**：额度未知写未知，节点不通写 invalid。
8. **不重复**：同值只留一条。

## 失败处理

| 情况 | 处理 |
|---|---|
| `hunt_batch` 全部非列目录 | 报告“无开放目录”，不硬抓首页 |
| 403/406/WAF | 记为在线/需认证，不绕、不爆破 |
| 文件过大/二进制 | 只保留证据路径，不把二进制塞进上下文 |
| 凭据格式对但来源不明 | 只汇报，不入库 |
| `cred_verify` 网络错误 | 标 unknown，不重试 |
| mihomo 不可用 | 代理节点标 error，报告原因 |
| 额度/页数超限 | 停止，报告已完成部分 |
