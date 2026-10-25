---
name: insecure-source-code-management
description: >-
  Source control and artifact exposure (.git, .svn, .hg, backups, .env). Use
  when recon finds VCS paths, 403 on hidden dirs, or backup/config leaks
  during authorized testing. 适用于 .git/.svn/.hg、备份与 .env 等源码/配置泄漏的发现与恢复（授权测试）。
---

> **ARTEX 使用边界（优先于下文）**
> 仅在已授权、范围明确的目标上使用。主动探测前先读 `scan-opsec`，流量走
> `127.0.0.1:7891` 代理；默认只做只读、单点、低频、可回滚的最小验证。
> 禁止批量爆破/撞库、破坏性写入、真实支付或删除、写 webshell、反弹 shell、
> 污染共享连接池；发现异常立即停止。下文的完整攻击链只用于理解与构造最小证明。

# SKILL: Insecure Source Code Management

> **AI LOAD INSTRUCTION**: This skill covers detection and recovery of exposed version-control metadata, common backup artifacts, and related misconfigurations. Use only in **authorized** assessments. Treat recovered credentials and URLs as sensitive; do not exfiltrate real data beyond scope. For broad discovery workflow, cross-load [fofa-recon](../fofa-recon/SKILL.md) and [api-recon](../api-recon/SKILL.md).

## 0. QUICK START

High-value paths to probe first (GET or HEAD, respect rate limits):

```http
/.git/HEAD
/.git/config
/.svn/entries
/.svn/wc.db
/.hg/requires
/.bzr/README
/.DS_Store
/.env
```

**Routing note**: quickly probe these paths first; for full recon workflow, load methodology from `fofa-recon` and `api-recon` before deeper testing.

---

## 1. GIT EXPOSURE

### Detection

- **`/.git/HEAD`** — valid repo often returns plain text like:

```text
ref: refs/heads/main
```

- **`/.git/config`** — may expose `remote.origin.url`, user identity, or embedded credentials.
- **`/.git/index`**, **`/.git/objects/`** — partial object store access enables reconstruction with the right tools.

### 403 vs 404

- **`404`** — path likely absent or fully blocked at the edge.
- **`403` on `/.git/`** — directory may **exist** but listing is denied; still try direct file URLs:

```http
/.git/HEAD
/.git/config
/.git/logs/HEAD
/.git/refs/heads/main
```

A **403 on the directory** plus **200 on `HEAD`** strongly indicates exposure.

### Recovery tools (open source)

- **`arthaud/git-dumper`** — dumps reachable `.git` tree when individual files are fetchable.
- **`internetwache/GitTools`** — Dumper, Extractor, Finder modules for partial/corrupt dumps.
- **`WangYihang/GitHacker`** — alternative recovery when standard dumpers miss edge cases.

### Key files to prioritize

| Path | Why it matters |
|------|----------------|
| `.git/config` | Remotes, credentials, hooks paths |
| `.git/logs/HEAD` | Commit history, reflog-style leakage |
| `.git/refs/heads/*` | Branch tips, commit SHAs |
| `.git/packed-refs` | Packed branch/tag refs |
| `.git/objects/**` | Object blobs for reconstruction |

---

## 2. SVN EXPOSURE

### Detection

- **SVN before 1.7**: **`/.svn/entries`** — XML or text metadata listing paths and revisions.
- **SVN ≥ 1.7**: **`/.svn/wc.db`** — SQLite working copy database (`PRAGMA table_info` after download).

Example probe:

```http
GET /.svn/entries HTTP/1.1
GET /.svn/wc.db HTTP/1.1
```

### Recovery

- **`anantshri/svn-extractor`** — automated extraction from exposed `.svn`.
- **Manual**: download `wc.db`, query with `sqlite3` for file paths and checksums, then request **`/.svn/pristine/`** blobs if exposed.

---

## 3. MERCURIAL EXPOSURE

### Detection

- **`/.hg/requires`** — small text file listing repository features; confirms Mercurial metadata.

```http
GET /.hg/requires HTTP/1.1
GET /.hg/store/ HTTP/1.1
```

### Recovery

- **`sahildhar/mercurial_source_code_dumper`** — dumps repository when store paths are reachable.

---

## 4. OTHER LEAKS

### Bazaar (Bzr)

- Probe **`/.bzr/README`** and **`/.bzr/branch-format`** for Bazaar metadata.

### macOS `.DS_Store`

- **`/.DS_Store`** can encode directory and filename listings.
- Tools: **`gehaxelt/ds-store`**, **`lijiejie/ds_store_exp`** — parse `.DS_Store` offline.

### Backup and config artifacts

Probe (adjust for app root and naming conventions):

```text
/.env
/backup.zip
/backup.tar.gz
/wwwroot.rar
/backup.sql
/config.php.bak
/.config.php.swp
```

### Web server misconfiguration signal (example: NGINX)

- **`location /.git { deny all; }`** — may return **403** for `/.git/` while still allowing or denying specific subpaths depending on rules.
- **403 on a protected location** can **confirm the route exists**; always distinguish from **404** on non-existent paths.

---

## 5. DECISION TREE

1. **Probe `/.git/HEAD`** → `ref: refs/heads/` pattern? → run **git-dumper / GitTools / GitHacker**; review `config` and `logs/HEAD` for secrets.
2. **Else probe `/.svn/wc.db` or `entries`** → success? → **svn-extractor** or manual `wc.db` + pristine recovery.
3. **Else probe `/.hg/requires`** → success? → **mercurial dumper**.
4. **Else probe `/.bzr/README`** → Bazaar tooling or manual path walk.
5. **Parallel**: fetch **`/.DS_Store`**, **`/.env`**, common **backup extensions** on app root and parent paths.
6. **Interpret status codes**: **403 on directory** + **200 on specific files** → treat as **high priority** for file-by-file extraction.

---

## 6. RELATED ROUTING

- From **[fofa-recon](../fofa-recon/SKILL.md)** — scope-safe asset discovery before deep VCS tests.
- From **[api-recon](../api-recon/SKILL.md)** — endpoint inventory and evidence handling.

**Note**: coordinate with recon skills—set scope and request rate first, then run targeted VCS/backup validation.
