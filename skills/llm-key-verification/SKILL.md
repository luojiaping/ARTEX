---
name: llm-key-verification
description: LLM API Key / OAuth 包的最小验活与额度查询：厂商识别、/models 优先、1-token 兜底、状态码判定、余额接口。适用于 cred_verify 前后的人工复核与失败解释。
license: MIT
compatibility: 需要 cred_verify；额度查询由服务端按厂商自动执行。
---

# LLM Key 验活（llm-key-verification）

## 原则

- **最小请求**：优先 `GET /models`（通常不计生成额度）；只有 404/405 才发 `max_tokens=1` 的 `hi`。
- **不重试**：401/403 直接判失效；429 记限流；5xx/网络错误记未知。
- **不泄密**：不要把 key 拼进 Bash、日志或报告；只看工具返回的 `masked`。
- **只读**：不调用生成类接口做业务请求，不拉取用户数据。

## 厂商识别

服务端按 `service / context / source_url` 和密钥前缀分类，常见：

| 前缀/特征 | 厂商 | 默认 base |
|---|---|---|
| `sk-or-` | OpenRouter | `https://openrouter.ai/api/v1` |
| `sk-ant-api` | Anthropic | `https://api.anthropic.com/v1` |
| `sk-ant-oat` | Claude OAuth | usage 接口 |
| `gsk_` | Groq | `https://api.groq.com/openai/v1` |
| `nvapi-` | NVIDIA NIM | `https://integrate.api.nvidia.com/v1` |
| `xai-` | xAI | `https://api.x.ai/v1` |
| `AIza` | Gemini | `https://generativelanguage.googleapis.com/v1beta` |
| `gho_` / `ghu_` | GitHub Copilot OAuth | `https://api.github.com/copilot_internal/user` |

服务名关键词（`deepseek`、`moonshot/kimi`、`glm/zhipu`、`dashscope/qwen`、`minimax`、`ark/doubao`、`siliconflow`、`stepfun`、`xf-yun`、`sensenova`、`mimo`、`opencode`、`lkeap`）也会命中。

**中转/兼容网关**：service/context 里出现“中转、网关、relay、one-api、new-api、vllm”等，且能从上下文提取非官方域名时，服务端会推断 `base_url`。推断不出就让用户补 `base_url`，不要瞎猜。

## 请求形态

### OpenAI 兼容
```
GET  {base}/models
POST {base}/chat/completions
Authorization: Bearer <key>
{"model":"<model>","messages":[{"role":"user","content":"hi"}],"max_tokens":1,"stream":false}
```

### Anthropic
```
GET  {base}/models
POST {base}/messages
x-api-key: <key>
anthropic-version: 2023-06-01
{"model":"claude-3-haiku-20240307","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}
```

### Gemini
```
GET  {base}/models?key=<key>
POST {base}/models/<model>:generateContent?key=<key>
{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":1}}
```

### OAuth 包
从 JSON 里取 `access_token`（或 `tokens.access_token`），只读调用：
- Codex：`https://chatgpt.com/backend-api/wham/usage`
- Claude：`https://api.anthropic.com/api/oauth/usage`
- Copilot：`https://api.github.com/copilot_internal/user`

**不要刷新 token**（refresh 会改账号状态）；access_token 过期就判 unknown/失效。

## 判定表

| 结果 | 含义 | 后续 |
|---|---|---|
| 200（models 或 1-token chat） | valid | 查余额（支持时） |
| 401/403 | invalid | 不再试 |
| 402 | no_balance | 不再试 |
| 429 | rate_limited | 不重试，隔一段时间再说 |
| 5xx / 网络错误 | unknown | 不重试 |
| 400/404 且无可用端点 | unknown | 检查 base_url/model |

## 额度接口（服务端自动）

| 厂商 | 接口 |
|---|---|
| DeepSeek | `GET {base}/user/balance` |
| Moonshot/Kimi | `GET {base}/users/me/balance` |
| SiliconFlow | `GET {base}/user/info` |
| OpenRouter | `GET {base}/credits` |

其余厂商没有统一余额 API：显示“可用（未取到额度）”，不要编造数字。

## 何时只登记不验活

- 密码、私钥、SSH key、数据库连接串、云 AK/SK：**不主动登录/调用**。
- 用户明确说“先别验证”：全部留在 `unverified`。
- 同一 key 已验过且状态未过期：不要重复消耗。

## 报告写法

```
- 有效：OpenRouter × 2、DeepSeek × 1
- 失效：Groq × 3（401）
- 限流：1
- 未知：2（网络错误）
- 仅登记未验活：私钥 4、SSH 口令 2
```
