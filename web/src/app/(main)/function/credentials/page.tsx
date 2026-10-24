"use client";

import * as React from "react";

import {
  ActivityIcon,
  ArchiveIcon,
  EyeIcon,
  EyeOffIcon,
  KeyRoundIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
  ShieldXIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import type { CredentialEntry, CredentialSource, CredentialStats } from "@/lib/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 50;

const TABS: { key: string; label: string }[] = [
  { key: "all", label: "全部" },
  { key: "llm_key", label: "LLM Key" },
  { key: "oauth", label: "OAuth" },
  { key: "proxy_node", label: "代理节点" },
  { key: "other", label: "其他" },
];

const KIND_LABEL: Record<string, string> = {
  llm_key: "LLM Key",
  oauth: "OAuth",
  api_key: "API Key",
  token: "Token",
  credential: "口令",
  private_key: "私钥",
  proxy_node: "代理节点",
  proxy_subscription: "代理订阅",
  other: "其他",
};

function statusMeta(status: string): { label: string; className: string } {
  switch (status) {
    case "valid":
      return { label: "有效", className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/30" };
    case "invalid":
      return { label: "失效", className: "bg-red-500/10 text-red-600 border-red-500/30" };
    case "unverified":
      return { label: "待验证", className: "bg-muted text-muted-foreground" };
    case "no_balance":
      return { label: "余额不足", className: "bg-amber-500/10 text-amber-600 border-amber-500/30" };
    case "rate_limited":
      return { label: "限流", className: "bg-amber-500/10 text-amber-600 border-amber-500/30" };
    case "unsupported":
      return { label: "仅登记", className: "bg-blue-500/10 text-blue-600 border-blue-500/30" };
    case "unknown":
      return { label: "未知", className: "bg-muted text-muted-foreground" };
    case "archived":
      return { label: "已归档", className: "bg-muted text-muted-foreground" };
    default:
      return { label: status || "—", className: "bg-muted text-muted-foreground" };
  }
}

function fmtTime(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("zh-CN", { hour12: false });
}

function short(value: string, max = 48): string {
  if (!value) return "—";
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

function quotaSummary(entry: CredentialEntry): string {
  const q = entry.quota;
  if (!q) return "—";
  const summary = q.summary;
  if (typeof summary === "string" && summary) return summary;
  return "—";
}

function statTone(value: number): string {
  return value > 0 ? "text-foreground" : "text-muted-foreground";
}

export default function CredentialsPage() {
  const [tab, setTab] = React.useState("all");
  const [qInput, setQInput] = React.useState("");
  const [q, setQ] = React.useState("");
  const [status, setStatus] = React.useState("");
  const [page, setPage] = React.useState(0);
  const [entries, setEntries] = React.useState<CredentialEntry[]>([]);
  const [total, setTotal] = React.useState(0);
  const [stats, setStats] = React.useState<CredentialStats | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [busy, setBusy] = React.useState("");
  const [detail, setDetail] = React.useState<CredentialEntry | null>(null);
  const [sources, setSources] = React.useState<CredentialSource[]>([]);
  const [detailOpen, setDetailOpen] = React.useState(false);
  const [revealed, setRevealed] = React.useState("");

  React.useEffect(() => {
    const t = window.setTimeout(() => {
      setQ(qInput.trim());
      setPage(0);
    }, 300);
    return () => window.clearTimeout(t);
  }, [qInput]);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const kind = tab === "all" ? undefined : tab === "other" ? "other,api_key,token,credential,private_key,proxy_subscription" : tab;
      const [list, st] = await Promise.all([
        api.credentials({ kind, q: q || undefined, status: status || undefined, limit: PAGE_SIZE, offset: page * PAGE_SIZE }),
        api.credentialStats(),
      ]);
      setEntries(list.items ?? []);
      setTotal(list.total ?? 0);
      setStats(st);
    } catch (e) {
      toast.error("加载凭据失败：" + (e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [tab, q, status, page]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const openDetail = async (entry: CredentialEntry) => {
    setDetail(entry);
    setSources([]);
    setRevealed("");
    setDetailOpen(true);
    try {
      const r = await api.credential(entry.uid);
      setDetail(r.entry);
      setSources(r.sources ?? []);
    } catch (e) {
      toast.error("读取详情失败：" + (e as Error).message);
    }
  };

  const verify = async (entry: CredentialEntry) => {
    setBusy("verify:" + entry.uid);
    try {
      const r = await api.verifyCredential(entry.uid);
      toast.success(`${KIND_LABEL[entry.kind] ?? entry.kind} → ${statusMeta(r.status).label}`);
      await load();
      if (detail?.uid === entry.uid) {
        const fresh = await api.credential(entry.uid);
        setDetail(fresh.entry);
        setSources(fresh.sources ?? []);
      }
    } catch (e) {
      toast.error("验证失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const probe = async (entry: CredentialEntry) => {
    setBusy("probe:" + entry.uid);
    try {
      const r = await api.probeProxy(entry.uid);
      toast.success(r.status === "valid" ? `节点可用 ${r.delay_ms ?? 0}ms` : `节点不可用：${r.message || r.status}`);
      await load();
    } catch (e) {
      toast.error("探测失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const archive = async (entry: CredentialEntry) => {
    if (!window.confirm(`归档凭据 ${entry.uid}？证据文件不会删除。`)) return;
    setBusy("archive:" + entry.uid);
    try {
      await api.archiveCredential(entry.uid);
      toast.success("已归档");
      setDetailOpen(false);
      await load();
    } catch (e) {
      toast.error("归档失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const reveal = async () => {
    if (!detail) return;
    if (revealed) {
      setRevealed("");
      return;
    }
    if (!window.confirm("明文只应在授权核查时查看，且会记录审计日志。继续？")) return;
    try {
      const r = await api.revealCredential(detail.uid);
      setRevealed(r.value);
    } catch (e) {
      toast.error("读取明文失败：" + (e as Error).message);
    }
  };

  const verifyPending = async () => {
    setBusy("verify-batch");
    try {
      const r = await api.verifyCredentials({ status: "unverified", limit: 20 });
      toast.success(`已验活 ${r.count} 条`);
      await load();
    } catch (e) {
      toast.error("批量验证失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const probeRecent = async () => {
    setBusy("probe-batch");
    try {
      const r = await api.probeProxies({ limit: 20 });
      toast.success(`已探测 ${r.count} 个节点`);
      await load();
    } catch (e) {
      toast.error("批量探测失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            <KeyRoundIcon className="size-5 text-muted-foreground" />
            凭据
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            扫描到的 LLM Key、OAuth、口令、私钥与代理节点。默认只显示掩码；明文查看会记审计日志。
          </p>
        </div>
        <div className="flex items-center gap-2">
          {tab === "proxy_node" ? (
            <Button variant="outline" size="sm" onClick={probeRecent} disabled={busy === "probe-batch"}>
              {busy === "probe-batch" ? <Spinner className="size-4" /> : <ActivityIcon className="size-4" />}
              探测前 20
            </Button>
          ) : null}
          <Button variant="outline" size="sm" onClick={verifyPending} disabled={busy === "verify-batch"}>
            {busy === "verify-batch" ? <Spinner className="size-4" /> : <ShieldCheckIcon className="size-4" />}
            验证待测
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            <RefreshCwIcon className={cn("size-4", loading && "animate-spin")} />
            刷新
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>凭据总数</CardDescription>
            <CardTitle className={cn("text-2xl tabular-nums", statTone(stats?.total ?? 0))}>
              {(stats?.total ?? 0).toLocaleString()}
            </CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>已验证有效</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-emerald-600">{(stats?.valid ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>待验证</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-amber-600">{(stats?.unverified ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>代理节点存活</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-blue-600">{(stats?.proxy_alive ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardHeader className="gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Tabs
              value={tab}
              onValueChange={(v) => {
                setTab(v);
                setPage(0);
              }}
            >
              <TabsList>
                {TABS.map((t) => (
                  <TabsTrigger key={t.key} value={t.key}>
                    {t.label}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            <div className="flex flex-wrap items-center gap-2">
              <div className="relative">
                <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
                <Input
                  value={qInput}
                  onChange={(e) => setQInput(e.target.value)}
                  placeholder="服务 / 厂商 / 来源"
                  className="h-8 w-56 pl-8"
                />
              </div>
              <Select
                value={status || "all"}
                onValueChange={(v) => {
                  setStatus(v === "all" ? "" : v);
                  setPage(0);
                }}
              >
                <SelectTrigger className="h-8 w-32">
                  <SelectValue placeholder="状态" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">全部状态</SelectItem>
                  <SelectItem value="valid">有效</SelectItem>
                  <SelectItem value="invalid">失效</SelectItem>
                  <SelectItem value="unverified">待验证</SelectItem>
                  <SelectItem value="no_balance">余额不足</SelectItem>
                  <SelectItem value="unsupported">仅登记</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">状态</TableHead>
                <TableHead className="w-28">类型</TableHead>
                <TableHead>厂商 / 服务</TableHead>
                <TableHead>掩码值</TableHead>
                <TableHead className="w-36">额度 / 延迟</TableHead>
                <TableHead className="max-w-[18rem]">来源</TableHead>
                <TableHead className="w-40">发现时间</TableHead>
                <TableHead className="w-44 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-muted-foreground py-10 text-center">
                    <Spinner className="mx-auto size-5" />
                  </TableCell>
                </TableRow>
              ) : entries.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-muted-foreground py-10 text-center text-sm">
                    暂无凭据。让「凭据猎手」扫描授权目标后，结果会出现在这里。
                  </TableCell>
                </TableRow>
              ) : (
                entries.map((entry) => {
                  const meta = statusMeta(entry.status);
                  return (
                    <TableRow key={entry.uid}>
                      <TableCell>
                        <Badge variant="outline" className={meta.className}>
                          {meta.label}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <span className="text-xs font-medium">{KIND_LABEL[entry.kind] ?? entry.kind}</span>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="text-sm">{entry.service || "—"}</span>
                          <span className="text-muted-foreground text-[11px]">{entry.provider || "unknown"}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <code className="text-xs">{entry.masked || "—"}</code>
                      </TableCell>
                      <TableCell>
                        {entry.proxy ? (
                          <Badge
                            variant="outline"
                            className={
                              entry.proxy.status === "valid"
                                ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-600"
                                : "border-red-500/30 bg-red-500/10 text-red-600"
                            }
                          >
                            {entry.proxy.status === "valid" ? `${entry.proxy.delay_ms ?? 0}ms` : entry.proxy.status}
                          </Badge>
                        ) : (
                          <span className="text-muted-foreground text-xs">{quotaSummary(entry)}</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {entry.source_url && !entry.source_url.startsWith("legacy://") ? (
                          <a
                            href={entry.source_url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-muted-foreground hover:text-foreground text-xs underline-offset-2 hover:underline"
                            title={entry.source_url}
                          >
                            {short(entry.source_url, 40)}
                          </a>
                        ) : (
                          <span className="text-muted-foreground text-xs" title={entry.source_url}>
                            {entry.source_url ? short(entry.source_url, 40) : "—"}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-xs">{fmtTime(entry.found_at)}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => void openDetail(entry)}>
                            详情
                          </Button>
                          {entry.kind === "proxy_node" ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              disabled={busy === "probe:" + entry.uid}
                              onClick={() => void probe(entry)}
                            >
                              {busy === "probe:" + entry.uid ? <Spinner className="size-3" /> : "探测"}
                            </Button>
                          ) : ["llm_key", "oauth", "api_key", "token"].includes(entry.kind) ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              disabled={busy === "verify:" + entry.uid}
                              onClick={() => void verify(entry)}
                            >
                              {busy === "verify:" + entry.uid ? <Spinner className="size-3" /> : "验证"}
                            </Button>
                          ) : null}
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-muted-foreground h-7 px-2"
                            disabled={busy === "archive:" + entry.uid}
                            onClick={() => void archive(entry)}
                          >
                            <ArchiveIcon className="size-3.5" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
          <div className="flex items-center justify-between border-t px-4 py-2">
            <span className="text-muted-foreground text-xs">
              共 {total.toLocaleString()} 条 · 第 {page + 1}/{pages} 页
            </span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={page <= 0 || loading} onClick={() => setPage((p) => p - 1)}>
                上一页
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={page + 1 >= pages || loading}
                onClick={() => setPage((p) => p + 1)}
              >
                下一页
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Dialog open={detailOpen} onOpenChange={setDetailOpen}>
        <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <KeyRoundIcon className="size-4" />
              {detail ? `${KIND_LABEL[detail.kind] ?? detail.kind} · ${detail.service || detail.provider}` : "凭据详情"}
            </DialogTitle>
            <DialogDescription>
              {detail ? `${detail.uid} · 发现于 ${fmtTime(detail.found_at)}` : ""}
            </DialogDescription>
          </DialogHeader>
          {detail ? (
            <div className="flex flex-col gap-4 text-sm">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant="outline" className={statusMeta(detail.status).className}>
                  {statusMeta(detail.status).label}
                </Badge>
                <span className="text-muted-foreground text-xs">{detail.status_detail || "—"}</span>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-muted-foreground text-xs">厂商</div>
                  <div>{detail.provider || "unknown"}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">风险</div>
                  <div>{detail.risk || "—"}</div>
                </div>
                <div className="col-span-2">
                  <div className="text-muted-foreground text-xs">掩码值</div>
                  <div className="flex items-center gap-2">
                    <code className="bg-muted rounded px-2 py-1 text-xs">{detail.masked}</code>
                    <Button variant="outline" size="sm" className="h-6 px-2 text-xs" onClick={() => void reveal()}>
                      {revealed ? <EyeOffIcon className="size-3.5" /> : <EyeIcon className="size-3.5" />}
                      {revealed ? "隐藏明文" : "查看明文"}
                    </Button>
                  </div>
                  {revealed ? (
                    <pre className="bg-muted mt-2 max-h-40 overflow-auto rounded p-2 text-xs break-all whitespace-pre-wrap">
                      {revealed}
                    </pre>
                  ) : null}
                </div>
                {detail.proxy ? (
                  <div className="col-span-2">
                    <div className="text-muted-foreground text-xs">代理节点</div>
                    <div className="text-xs">
                      {detail.proxy.protocol}://{detail.proxy.server}:{detail.proxy.port} · {detail.proxy.status}
                      {detail.proxy.delay_ms ? ` · ${detail.proxy.delay_ms}ms` : ""}
                    </div>
                  </div>
                ) : null}
                <div className="col-span-2">
                  <div className="text-muted-foreground text-xs">来源</div>
                  {detail.source_url && !detail.source_url.startsWith("legacy://") ? (
                    <a
                      href={detail.source_url}
                      target="_blank"
                      rel="noreferrer"
                      className="text-xs break-all underline-offset-2 hover:underline"
                    >
                      {detail.source_url}
                    </a>
                  ) : (
                    <div className="text-xs break-all">{detail.source_url || "—"}</div>
                  )}
                </div>
                <div className="col-span-2">
                  <div className="text-muted-foreground text-xs">证据</div>
                  <div className="text-xs break-all">{detail.evidence_path || "—"}</div>
                </div>
                {detail.context ? (
                  <div className="col-span-2">
                    <div className="text-muted-foreground text-xs">判断依据</div>
                    <div className="text-xs whitespace-pre-wrap">{detail.context}</div>
                  </div>
                ) : null}
                {detail.quota ? (
                  <div className="col-span-2">
                    <div className="text-muted-foreground text-xs">额度 / 响应</div>
                    <pre className="bg-muted max-h-48 overflow-auto rounded p-2 text-[11px] whitespace-pre-wrap">
                      {JSON.stringify(detail.quota, null, 2)}
                    </pre>
                  </div>
                ) : null}
                {sources.length > 0 ? (
                  <div className="col-span-2">
                    <div className="text-muted-foreground text-xs">其他来源</div>
                    <div className="flex flex-col gap-1">
                      {sources.map((s, i) => (
                        <a
                          key={`${s.url}-${i}`}
                          href={s.url}
                          target="_blank"
                          rel="noreferrer"
                          className="text-xs break-all underline-offset-2 hover:underline"
                        >
                          {s.url}
                        </a>
                      ))}
                    </div>
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}
          <DialogFooter className="gap-2">
            {detail && detail.kind !== "proxy_node" && ["llm_key", "oauth", "api_key", "token"].includes(detail.kind) ? (
              <Button
                variant="outline"
                size="sm"
                disabled={busy === "verify:" + detail.uid}
                onClick={() => void verify(detail)}
              >
                <ShieldCheckIcon className="size-4" />
                验活
              </Button>
            ) : null}
            {detail && detail.kind === "proxy_node" ? (
              <Button
                variant="outline"
                size="sm"
                disabled={busy === "probe:" + detail.uid}
                onClick={() => void probe(detail)}
              >
                <ActivityIcon className="size-4" />
                探测
              </Button>
            ) : null}
            {detail ? (
              <Button variant="outline" size="sm" onClick={() => void archive(detail)}>
                <ArchiveIcon className="size-4" />
                归档
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
