#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""fofa_cli.py - 独立 FOFA 检索 CLI（仅标准库，无第三方依赖）

用法：
  set FOFA_KEY=你的key                  # Linux/macOS: export FOFA_KEY=...
  set FOFA_BASE_URL=https://fofa.info   # 可选，默认即此

  python fofa_cli.py account
  python fofa_cli.py search 'domain="example.com"' --fields host,ip,port,title --size 20
  python fofa_cli.py search 'title="Index of /"' --size 200 --mode next
  python fofa_cli.py search 'domain="example.com"' --mode next --next 1234567890
  python fofa_cli.py stats 'domain="example.com"' --fields port,title --size 10
  python fofa_cli.py host 1.2.3.4 [--no-detail] [--json]

子命令：account | search | stats | host
退出码：0 成功 / 2 用法或配置错误 / 3 FOFA 业务错误 / 4 网络错误
"""
import argparse, base64, json, os, re, sys, unicodedata
import urllib.error, urllib.parse, urllib.request

DEFAULT_BASE = "https://fofa.info"
DEFAULT_FIELDS = "host,ip,port,title,domain,server,protocol,icp,country,province,city,asn,org"
FIELD_RE = re.compile(r"^[A-Za-z][A-Za-z0-9_]*$")


def die(msg, code=3):
    print("[错误] " + msg, file=sys.stderr)
    sys.exit(code)


def norm_fields(raw):
    """与 fofa/client.go 一致：过滤非法字段、最多 24 个，空则回落 host,ip,port。"""
    out = [f.strip() for f in (raw or "").split(",") if f.strip() and FIELD_RE.match(f.strip())]
    return out[:24] or ["host", "ip", "port"]


def err_msg(payload, status):
    code = payload.get("code") or 0
    msg = str(payload.get("errmsg") or "").strip()
    if not code:
        m = re.search(r"\[(-?\d+)\]", msg)
        code = int(m.group(1)) if m else 0
    if code in (-700, -401) or status in (401, 403):
        return "API Key 无效或与账号不匹配：检查 FOFA_KEY（或「系统配置 → FOFA」）。" + msg
    if code in (-4, -6):
        return "查询额度不足：先运行 account 查看 remain_api_query / remain_api_data。" + msg
    if code == -5 or status == 429 or status >= 500:
        return "FOFA 服务端错误或请求频率超限：稍后重试，必要时降低并发 / fofa_rate_limit。" + msg
    if code == 820000:
        return "当前账号未开通连续翻页（search/next）：改用 --mode page。" + msg
    if code == -1:
        return "查询语法错误：检查引号、&&、括号与字段名。" + msg
    return "FOFA 返回错误 code=%s：%s" % (code or "?", msg or "（无 errmsg）")


def call(path, params):
    key = os.environ.get("FOFA_KEY", "").strip()
    if not key:
        die("未设置环境变量 FOFA_KEY。例如：set FOFA_KEY=你的key（Windows）", 2)
    base = os.environ.get("FOFA_BASE_URL", "").strip().rstrip("/") or DEFAULT_BASE
    q = dict(params, key=key, lang="zh-CN")
    req = urllib.request.Request(base + path + "?" + urllib.parse.urlencode(q),
                                 headers={"User-Agent": "artex-fofa-cli/1.0",
                                          "Accept": "application/json"})
    status, body = 200, b""
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            status, body = resp.status, resp.read()
    except urllib.error.HTTPError as e:
        status, body = e.code, e.read()
    except Exception as e:
        die("网络请求失败：%s（检查 FOFA_BASE_URL、代理与网络）" % e, 4)
    try:
        payload = json.loads(body.decode("utf-8", "replace"))
    except ValueError:
        die("响应不是 JSON（HTTP %s）：%s" % (status, body[:200].decode("utf-8", "replace")))
    if payload.get("error") or status >= 400:
        die(err_msg(payload, status), 3)
    return payload


def dwidth(s): return sum(2 if unicodedata.east_asian_width(ch) in "WF" else 1 for ch in s)


def clip(v, n=40):
    s = re.sub(r"\s+", " ", "" if v is None else str(v)).strip()
    return s if len(s) <= n else s[:n - 1] + "…"


def show_table(fields, rows):
    cells = [[clip(r[i] if i < len(r) else "") for i in range(len(fields))] for r in rows]
    width = [max([dwidth(f)] + [dwidth(c[i]) for c in cells]) for i, f in enumerate(fields)]
    line = lambda vs: "  ".join(v + " " * max(0, width[i] - dwidth(v)) for i, v in enumerate(vs))
    print(line(fields)); print("  ".join("-" * w for w in width))
    for c in cells:
        print(line(c))


def cmd_account(args):
    p = call("/api/v1/info/my", {})
    if args.json:
        return print(json.dumps(p, ensure_ascii=False, indent=2))
    for k in ("username", "email", "isvip", "vip_level", "remain_api_query",
              "remain_api_data", "remain_free_point", "fofa_point"):
        print("%-18s %s" % (k, p.get(k, "")))


def cmd_search(args):
    size = min(args.size, 10000) if args.size > 0 else 50
    fields = norm_fields(args.fields or DEFAULT_FIELDS)
    q = {"qbase64": base64.b64encode(args.query.strip().encode()).decode(), "fields": ",".join(fields)}
    if args.mode == "next":
        q.update(size=str(size), full="true" if args.full else "false")
        if args.next_cursor.strip():
            q["next"] = args.next_cursor.strip()
        p = call("/api/v1/search/next", q)
    else:
        if args.full:
            print("[提示] --full 只在 --mode next 生效，已忽略。", file=sys.stderr)
        q.update(page=str(max(1, args.page)), size=str(size))
        p = call("/api/v1/search/all", q)
    if args.json:
        return print(json.dumps(p, ensure_ascii=False, indent=2))
    rows, cur = p.get("results") or [], p.get("next") or ""
    cur = str(cur) if cur != "" else ""
    print("mode=%s page=%s returned=%d%s%s" % (
        args.mode, p.get("page", args.page), len(rows),
        " consumed_fpoint=%s" % p.get("consumed_fpoint") if p.get("consumed_fpoint") else "",
        " next=" + cur if cur else ""))
    show_table(fields, rows) if rows else print("（无结果）")
    if cur:
        print("\n继续翻页：--mode next --next %s" % cur)


def cmd_stats(args):
    size = min(args.size, 50) if args.size > 0 else 5
    fields = norm_fields(args.fields or "port,title,domain")
    p = call("/api/v1/search/stats", {
        "qbase64": base64.b64encode(args.query.strip().encode()).decode(),
        "fields": ",".join(fields), "size": str(size)})
    if args.json:
        return print(json.dumps(p, ensure_ascii=False, indent=2))
    print("## 去重计数")
    for k in sorted(p.get("distinct") or {}):
        print("- %s: %s" % (k, p["distinct"][k]))
    for f in fields:
        items = (p.get("aggs") or {}).get(f) or []
        if items:
            print("\n## %s Top%d" % (f, len(items)))
            for it in items:
                print("- %s: %s" % (it.get("name", ""), it.get("count", "")))
    if p.get("lastupdatetime"):
        print("\nlast_update=%s" % p["lastupdatetime"])


def cmd_host(args):
    p = call("/api/v1/host/" + urllib.parse.quote(args.host.strip(), safe=""),
             {"detail": "false" if args.no_detail else "true"})
    if args.json:
        return print(json.dumps(p, ensure_ascii=False, indent=2))
    for k in ("host", "ip", "asn", "org", "country_name", "country_code", "update_time"):
        print("%-14s %s" % (k, p.get(k, "")))
    for k in ("port", "protocol", "product", "category", "domain"):
        v = p.get(k)
        print("%-14s %s" % (k, ", ".join(map(str, v)) if isinstance(v, list) else v or ""))


def main():
    ap = argparse.ArgumentParser(description="独立 FOFA 检索 CLI（仅标准库）")
    sub = ap.add_subparsers(dest="cmd", required=True)
    a = sub.add_parser("account", help="账号与剩余额度（0 消耗）"); a.add_argument("--json", action="store_true")
    s = sub.add_parser("search", help="检索明细")
    s.add_argument("query", help="FOFA 查询语法")
    s.add_argument("--fields", default=DEFAULT_FIELDS)
    s.add_argument("--size", type=int, default=50, help="1-10000，默认 50")
    s.add_argument("--page", type=int, default=1, help="mode=page 的页码")
    s.add_argument("--mode", choices=("page", "next"), default="page")
    s.add_argument("--next", dest="next_cursor", default="", help="mode=next 的游标")
    s.add_argument("--full", action="store_true", help="mode=next 检索全部历史")
    s.add_argument("--json", action="store_true")
    t = sub.add_parser("stats", help="聚合估规模"); t.add_argument("query")
    t.add_argument("--fields", default="port,title,domain")
    t.add_argument("--size", type=int, default=5, help="每字段 TopN，1-50")
    t.add_argument("--json", action="store_true")
    h = sub.add_parser("host", help="单主机聚合"); h.add_argument("host")
    h.add_argument("--no-detail", action="store_true")
    h.add_argument("--json", action="store_true")
    args = ap.parse_args()
    for _s in (sys.stdout, sys.stderr):
        try:
            _s.reconfigure(encoding="utf-8", errors="replace")
        except Exception:
            pass
    {"account": cmd_account, "search": cmd_search,
     "stats": cmd_stats, "host": cmd_host}[args.cmd](args)


if __name__ == "__main__":
    main()
