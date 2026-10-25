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
  SlidersHorizontalIcon,
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { api } from "@/lib/api";
import type {
  CredentialEntry,
  CredentialFacets,
  CredentialProbe,
  CredentialProvider,
  CredentialSource,
} from "@/lib/types";
import { cn } from "@/lib/utils";

const PAGE_SIZE = 50;

const PCLASS: { value: string; label: string }[] = [
  { value: "all", label: "全部" },
  { value: "key", label: "密钥类" },
  { value: "oauth", label: "OAuth" },
  { value: "relay", label: "中转" },
  { value: "proxy", label: "代理" },
  { value: "generic", label: "通用" },
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

const STATUS_LABEL: Record<string, string> = {
  valid: "可用",
  invalid: "失效",
  unverified: "未探测",
  no_balance: "余额不足",
  rate_limited: "限流",
  unknown: "未知",
  unsupported: "仅登记",
  archived: "已归档",
};

const QUOTA_LABEL: Record<string, string> = {
  none: "—",
  balance: "余额",
  credits: "Credits",
  windows: "额度窗口",
  rate_headers: "限流头",
  relay: "中转额度",
  deepseek_balance: "DeepSeek 余额",
  moonshot_balance: "Moonshot 余额",
  siliconflow_user: "SiliconFlow 账户",
  openrouter_key: "OpenRouter Credits",
  codex_wham: "Codex 用量",
  claude_usage: "Claude 用量",
  kimi_coding: "Kimi Coding",
  zhipu_coding: "GLM Coding",
  minimax_coding: "MiniMax 套餐",
  opencode_go_usage: "OpenCode 用量",
};

const RISK_LABEL: Record<string, string> = {
  P0: "P0 严重",
  P1: "P1 高",
  P2: "P2 中",
  P3: "P3 低",
  unrated: "未评级",
};

function statusMeta(status: string): { label: string; className: string } {
  switch (status) {
    case "valid":
      return { label: "可用", className: "border-emerald-500/30 bg-emerald-500/10 text-emerald-600" };
    case "invalid":
      return { label: "失效", className: "border-red-500/30 bg-red-500/10 text-red-600" };
    case "no_balance":
      return { label: "余额不足", className: "border-amber-500/30 bg-amber-500/10 text-amber-600" };
    case "rate_limited":
      return { label: "限流", className: "border-amber-500/30 bg-amber-500/10 text-amber-600" };
    case "unknown":
      return { label: "未知", className: "bg-muted text-muted-foreground" };
    case "unsupported":
      return { label: "仅登记", className: "border-blue-500/30 bg-blue-500/10 text-blue-600" };
    case "unverified":
      return { label: "未探测", className: "bg-muted text-muted-foreground" };
    default:
      return { label: status || "—", className: "bg-muted text-muted-foreground" };
  }
}

function effectiveStatus(e: CredentialEntry): string {
  return e.manual_status || e.status;
}

function effectiveLabel(e: CredentialEntry): string {
  if (!e.manual_status) return statusMeta(e.status).label;
  const known = STATUS_LABEL[e.manual_status];
  if (known) return `手动 · ${known}`;
  if (/^ok\b/i.test(e.manual_status)) return "手动 · 可用";
  return "手动标记";
}

function manualSelectValue(status: string): string {
  if (!status) return "auto";
  return ["valid", "no_balance", "rate_limited", "invalid", "unknown"].includes(status) ? status : "custom";
}

function fmtTime(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("zh-CN", { hour12: false });
}

function short(value: string, max = 44): string {
  if (!value) return "—";
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

function quotaText(e: CredentialEntry): string {
  const q = e.quota;
  if (q && typeof q.summary === "string" && q.summary) return q.summary;
  if (e.quota_kind && e.quota_kind !== "none") return QUOTA_LABEL[e.quota_kind] ?? e.quota_kind;
  return "—";
}

function providerLabel(e: CredentialEntry): string {
  return e.provider_name || e.provider || "未识别";
}

function pclassLabel(value: string): string {
  return PCLASS.find((x) => x.value === value)?.label ?? (value || "未分类");
}

function selectedCount(selected: string[]): number {
  return selected.length;
}

export default function CredentialsPage() {
  const [facets, setFacets] = React.useState<CredentialFacets | null>(null);
  const [providers, setProviders] = React.useState<CredentialProvider[]>([]);
  const [items, setItems] = React.useState<CredentialEntry[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [busy, setBusy] = React.useState("");
  const [page, setPage] = React.useState(0);
  const [qInput, setQInput] = React.useState("");
  const [filters, setFilters] = React.useState({
    pclass: "",
    provider: "",
    variant: "",
    status: "",
    risk: "",
    q: "",
    quotaSupported: false,
    manual: false,
  });
  const [selected, setSelected] = React.useState<string[]>([]);

  const [detailOpen, setDetailOpen] = React.useState(false);
  const [detail, setDetail] = React.useState<CredentialEntry | null>(null);
  const [sources, setSources] = React.useState<CredentialSource[]>([]);
  const [probes, setProbes] = React.useState<CredentialProbe[]>([]);
  const [revealed, setRevealed] = React.useState("");
  const [variantDraft, setVariantDraft] = React.useState("");
  const [baseDraft, setBaseDraft] = React.useState("");

  React.useEffect(() => {
    const t = window.setTimeout(() => {
      setFilters((f) => (f.q === qInput.trim() ? f : { ...f, q: qInput.trim() }));
      setPage(0);
    }, 300);
    return () => window.clearTimeout(t);
  }, [qInput]);

  React.useEffect(() => {
    api
      .credentialProviders()
      .then((r) => setProviders(r.items ?? []))
      .catch(() => {});
  }, []);

  const load = React.useCallback(async () => {
    setLoading(true);
    try {
      const [list, f] = await Promise.all([
        api.credentials({
          pclass: filters.pclass || undefined,
          provider: filters.provider || undefined,
          variant: filters.variant || undefined,
          status: filters.status || undefined,
          risk: filters.risk || undefined,
          q: filters.q || undefined,
          quota_supported: filters.quotaSupported || undefined,
          manual: filters.manual || undefined,
          limit: PAGE_SIZE,
          offset: page * PAGE_SIZE,
        }),
        api.credentialFacets(),
      ]);
      setItems(list.items ?? []);
      setTotal(list.total ?? 0);
      setFacets(f);
    } catch (e) {
      toast.error("加载凭据失败：" + (e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [filters, page]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const providerOptions = facets?.by_provider ?? [];
  const variantOptions = (facets?.by_variant ?? []).filter(
    (v) => !filters.provider || v.provider_name === filters.provider,
  );
  const statusOptions = facets?.by_status ?? [];
  const riskOptions = facets?.by_risk ?? [];

  const allSelected = items.length > 0 && items.every((e) => selected.includes(e.uid));
  const verifiableSelected = items.filter(
    (e) => selected.includes(e.uid) && ["llm_key", "oauth", "api_key", "token"].includes(e.kind),
  );

  const toggleAll = (checked: boolean | "indeterminate") => {
    if (checked === true) setSelected(items.map((e) => e.uid));
    else setSelected([]);
  };

  const openDetail = async (entry: CredentialEntry) => {
    setDetail(entry);
    setSources([]);
    setProbes([]);
    setRevealed("");
    setVariantDraft(entry.variant || "");
    setBaseDraft(typeof entry.extra?.base_url === "string" ? entry.extra.base_url : "");
    setDetailOpen(true);
    try {
      const r = await api.credential(entry.uid);
      setDetail(r.entry);
      setSources(r.sources ?? []);
      setProbes(r.probes ?? []);
      setVariantDraft(r.entry.variant || "");
      setBaseDraft(typeof r.entry.extra?.base_url === "string" ? r.entry.extra.base_url : "");
    } catch (e) {
      toast.error("读取详情失败：" + (e as Error).message);
    }
  };

  const verifyOne = async (entry: CredentialEntry) => {
    setBusy("verify:" + entry.uid);
    try {
      const r = await api.verifyCredential(entry.uid);
      toast.success(`${providerLabel(entry)} → ${statusMeta(r.status).label}`);
      await load();
      if (detail?.uid === entry.uid) await openDetail(entry);
    } catch (e) {
      toast.error("验活失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const probeOne = async (entry: CredentialEntry) => {
    setBusy("probe:" + entry.uid);
    try {
      const r = await api.probeProxy(entry.uid);
      toast.success(r.status === "valid" ? `节点可用 ${r.delay_ms ?? 0}ms` : `节点不可用：${r.message || r.status}`);
      await load();
      if (detail?.uid === entry.uid) await openDetail(entry);
    } catch (e) {
      toast.error("探测失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const verifySelected = async () => {
    if (verifiableSelected.length === 0) {
      toast.error("选中的条目里没有可验活的 LLM 凭据");
      return;
    }
    const uids = verifiableSelected.slice(0, 20).map((e) => e.uid);
    setBusy("batch-verify");
    try {
      const r = await api.verifyCredentials({ uids });
      toast.success(`已验活 ${r.count} 条`);
      setSelected([]);
      await load();
    } catch (e) {
      toast.error("批量验活失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const saveStatus = async (status: string) => {
    if (!detail) return;
    setBusy("status");
    try {
      const next = await api.setCredentialStatus(detail.uid, status);
      setDetail(next);
      toast.success(status ? `已标记「${STATUS_LABEL[status] ?? status}」` : "已恢复跟随探活");
      await load();
    } catch (e) {
      toast.error("保存手动状态失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const saveVariant = async () => {
    if (!detail) return;
    setBusy("variant");
    try {
      const next = await api.setCredentialVariant(detail.uid, variantDraft === "auto" ? "" : variantDraft, baseDraft);
      setDetail(next);
      toast.success("形态已保存，状态已重置为未探测");
      await load();
    } catch (e) {
      toast.error("保存形态失败：" + (e as Error).message);
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

  const archive = async (entry: CredentialEntry) => {
    if (!window.confirm(`归档凭据 ${entry.uid}？证据文件不会删除。`)) return;
    setBusy("archive:" + entry.uid);
    try {
      await api.archiveCredential(entry.uid);
      toast.success("已归档");
      setDetailOpen(false);
      setSelected((prev) => prev.filter((uid) => uid !== entry.uid));
      await load();
    } catch (e) {
      toast.error("归档失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const resetFilters = () => {
    setFilters({
      pclass: "",
      provider: "",
      variant: "",
      status: "",
      risk: "",
      q: "",
      quotaSupported: false,
      manual: false,
    });
    setQInput("");
    setPage(0);
  };

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const detailProvider = providers.find((p) => p.id === detail?.provider);
  const detailVariants = detailProvider?.variants ?? [];
  const detailStatus = detail ? effectiveStatus(detail) : "";

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            <KeyRoundIcon className="text-muted-foreground size-5" />
            凭据
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            精细化分类：大类、厂商、形态、可用性、额度与探测历史。默认只显示掩码。
          </p>
        </div>
        <div className="flex items-center gap-2">
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
            <CardTitle className="text-2xl tabular-nums">{(facets?.total ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>LLM 密钥类</CardDescription>
            <CardTitle className="text-2xl tabular-nums">{(facets?.llm_key ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>OAuth</CardDescription>
            <CardTitle className="text-2xl tabular-nums">{(facets?.oauth ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>可用</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-emerald-600">{(facets?.valid ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>余额不足 / 限流</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-amber-600">{(facets?.warn ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>未探测</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-amber-600">{(facets?.unverified ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>支持额度</CardDescription>
            <CardTitle className="text-2xl tabular-nums">{(facets?.quota_supported ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardDescription>代理节点存活</CardDescription>
            <CardTitle className="text-2xl tabular-nums text-blue-600">{(facets?.proxy_alive ?? 0).toLocaleString()}</CardTitle>
          </CardHeader>
        </Card>
      </div>

      <Card>
        <CardHeader className="gap-3">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Tabs
              value={filters.pclass || "all"}
              onValueChange={(v) => {
                setFilters((f) => ({ ...f, pclass: v === "all" ? "" : v, variant: "" }));
                setPage(0);
                setSelected([]);
              }}
            >
              <TabsList>
                {PCLASS.map((p) => {
                  const n =
                    p.value === "all"
                      ? facets?.total
                      : facets?.by_pclass.find((x) => x.value === p.value)?.n;
                  return (
                    <TabsTrigger key={p.value} value={p.value}>
                      {p.label}
                      {typeof n === "number" && n > 0 ? (
                        <span className="text-muted-foreground ml-1 text-[10px]">{n}</span>
                      ) : null}
                    </TabsTrigger>
                  );
                })}
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
              <Button variant="ghost" size="sm" onClick={resetFilters}>
                <SlidersHorizontalIcon className="size-4" />
                重置
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={filters.provider || "all"}
              onValueChange={(v) => {
                setFilters((f) => ({ ...f, provider: v === "all" ? "" : v, variant: "" }));
                setPage(0);
              }}
            >
              <SelectTrigger className="h-8 w-44">
                <SelectValue placeholder="全部厂商" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部厂商</SelectItem>
                {providerOptions.map((p) => (
                  <SelectItem key={p.value} value={p.value}>
                    {(p.provider_name || p.value) + " (" + p.n + ")"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={filters.variant || "all"}
              onValueChange={(v) => {
                setFilters((f) => ({ ...f, variant: v === "all" ? "" : v }));
                setPage(0);
              }}
              disabled={!filters.provider}
            >
              <SelectTrigger className="h-8 w-44">
                <SelectValue placeholder="全部形态" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部形态</SelectItem>
                {variantOptions.map((v) => {
                  const variant = v.value.includes(":") ? v.value.split(":").slice(1).join(":") : v.value;
                  return (
                    <SelectItem key={v.value} value={variant}>
                      {(v.label || variant) + " (" + v.n + ")"}
                    </SelectItem>
                  );
                })}
              </SelectContent>
            </Select>

            <Select
              value={filters.status || "all"}
              onValueChange={(v) => {
                setFilters((f) => ({ ...f, status: v === "all" ? "" : v }));
                setPage(0);
              }}
            >
              <SelectTrigger className="h-8 w-36">
                <SelectValue placeholder="全部状态" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部状态</SelectItem>
                {statusOptions.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {(STATUS_LABEL[s.value] ?? s.value) + " (" + s.n + ")"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select
              value={filters.risk || "all"}
              onValueChange={(v) => {
                setFilters((f) => ({ ...f, risk: v === "all" ? "" : v }));
                setPage(0);
              }}
            >
              <SelectTrigger className="h-8 w-32">
                <SelectValue placeholder="风险" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部风险</SelectItem>
                {riskOptions.map((r) => (
                  <SelectItem key={r.value} value={r.value}>
                    {(RISK_LABEL[r.value] ?? r.value) + " (" + r.n + ")"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <label className="text-muted-foreground flex items-center gap-2 text-xs">
              <Switch
                checked={filters.quotaSupported}
                onCheckedChange={(v) => {
                  setFilters((f) => ({ ...f, quotaSupported: v }));
                  setPage(0);
                }}
              />
              仅支持额度
            </label>
            <label className="text-muted-foreground flex items-center gap-2 text-xs">
              <Switch
                checked={filters.manual}
                onCheckedChange={(v) => {
                  setFilters((f) => ({ ...f, manual: v }));
                  setPage(0);
                }}
              />
              仅手动标记
            </label>
          </div>

          {selected.length > 0 ? (
            <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-xs">
              <span>已选 {selectedCount(selected)} 条</span>
              <Button size="sm" variant="outline" className="h-7" onClick={() => void verifySelected()} disabled={busy === "batch-verify"}>
                {busy === "batch-verify" ? <Spinner className="size-3.5" /> : <ShieldCheckIcon className="size-3.5" />}
                批量验活（{verifiableSelected.length}）
              </Button>
              <Button size="sm" variant="ghost" className="h-7" onClick={() => setSelected([])}>
                清空选择
              </Button>
            </div>
          ) : null}
        </CardHeader>

        <CardContent className="overflow-x-auto p-0">
          <Table className="w-full table-fixed">
            <TableHeader>
              <TableRow>
                <TableHead className="w-8">
                  <Checkbox
                    checked={allSelected ? true : selected.length > 0 ? "indeterminate" : false}
                    onCheckedChange={toggleAll}
                  />
                </TableHead>
                <TableHead className="w-20">状态</TableHead>
                <TableHead className="w-64 xl:w-52">厂商 / 形态</TableHead>
                <TableHead className="hidden xl:table-cell">服务 / 目标</TableHead>
                <TableHead className="hidden w-32 xl:table-cell">掩码</TableHead>
                <TableHead className="hidden w-28 xl:table-cell">额度</TableHead>
                <TableHead className="hidden w-32 xl:table-cell">最近探测</TableHead>
                <TableHead className="w-36 text-right">操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-muted-foreground py-10 text-center">
                    <Spinner className="mx-auto size-5" />
                  </TableCell>
                </TableRow>
              ) : items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="text-muted-foreground py-10 text-center text-sm">
                    没有匹配的凭据。
                  </TableCell>
                </TableRow>
              ) : (
                items.map((entry) => {
                  const meta = statusMeta(effectiveStatus(entry));
                  return (
                    <TableRow key={entry.uid}>
                      <TableCell>
                        <Checkbox
                          checked={selected.includes(entry.uid)}
                          onCheckedChange={(checked) => {
                            setSelected((prev) =>
                              checked === true ? [...prev, entry.uid] : prev.filter((uid) => uid !== entry.uid),
                            );
                          }}
                        />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col gap-1">
                          <Badge variant="outline" className={cn(meta.className, "max-w-full truncate")}>
                            {effectiveLabel(entry)}
                          </Badge>
                          {entry.manual_status ? (
                            <span className="text-muted-foreground text-[10px]">手动标记</span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col gap-1">
                          <div className="flex items-center gap-1.5">
                            <Badge variant="outline" className="text-[10px]">
                              {pclassLabel(entry.pclass)}
                            </Badge>
                            <span className="truncate text-sm">{providerLabel(entry)}</span>
                          </div>
                          {entry.variant_label || entry.variant ? (
                            <span className="text-muted-foreground text-[11px]">{entry.variant_label || entry.variant}</span>
                          ) : null}
                          <span className="text-muted-foreground truncate text-[11px] xl:hidden" title={entry.service}>
                            {entry.service || KIND_LABEL[entry.kind] || entry.kind}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="hidden xl:table-cell">
                        <div className="flex max-w-[22rem] flex-col">
                          <span className="truncate text-sm" title={entry.service || entry.kind}>
                            {entry.service || KIND_LABEL[entry.kind] || entry.kind}
                          </span>
                          <span className="text-muted-foreground truncate text-[11px]" title={entry.source_url}>
                            {short(entry.source_url, 52)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="hidden xl:table-cell">
                        <code className="text-xs">{entry.masked || "—"}</code>
                      </TableCell>
                      <TableCell className="hidden xl:table-cell">
                        <div className="flex flex-col">
                          <span className="text-xs">{quotaText(entry)}</span>
                          {entry.quota_kind && entry.quota_kind !== "none" ? (
                            <span className="text-muted-foreground text-[10px]">
                              {QUOTA_LABEL[entry.quota_kind] ?? entry.quota_kind}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className="text-muted-foreground hidden text-xs xl:table-cell">{fmtTime(entry.checked_at)}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-1">
                          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={() => void openDetail(entry)}>
                            详情
                          </Button>
                          {["llm_key", "oauth", "api_key", "token"].includes(entry.kind) ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              disabled={busy === "verify:" + entry.uid}
                              onClick={() => void verifyOne(entry)}
                            >
                              {busy === "verify:" + entry.uid ? <Spinner className="size-3" /> : "验活"}
                            </Button>
                          ) : null}
                          {entry.kind === "proxy_node" ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              disabled={busy === "probe:" + entry.uid}
                              onClick={() => void probeOne(entry)}
                            >
                              {busy === "probe:" + entry.uid ? <Spinner className="size-3" /> : "探测"}
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
              <Button variant="outline" size="sm" disabled={page + 1 >= pages || loading} onClick={() => setPage((p) => p + 1)}>
                下一页
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Sheet open={detailOpen} onOpenChange={setDetailOpen}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          <SheetHeader>
            <SheetTitle className="flex flex-wrap items-center gap-2">
              <KeyRoundIcon className="size-4" />
              {detail ? providerLabel(detail) : "凭据详情"}
              {detail ? (
                <Badge variant="outline" className={statusMeta(detailStatus).className}>
                  {effectiveLabel(detail)}
                </Badge>
              ) : null}
            </SheetTitle>
            <SheetDescription>
              {detail ? `${detail.uid} · ${detail.service || KIND_LABEL[detail.kind] || detail.kind}` : ""}
            </SheetDescription>
          </SheetHeader>

          {detail ? (
            <div className="flex flex-col gap-5 px-4 pb-4 text-sm">
              <div className="flex flex-wrap gap-2">
                {["llm_key", "oauth", "api_key", "token"].includes(detail.kind) ? (
                  <Button size="sm" variant="outline" disabled={busy === "verify:" + detail.uid} onClick={() => void verifyOne(detail)}>
                    <ShieldCheckIcon className="size-4" />
                    验活
                  </Button>
                ) : null}
                {detail.kind === "proxy_node" ? (
                  <Button size="sm" variant="outline" disabled={busy === "probe:" + detail.uid} onClick={() => void probeOne(detail)}>
                    <ActivityIcon className="size-4" />
                    探测
                  </Button>
                ) : null}
                <Button size="sm" variant="outline" onClick={() => void reveal()}>
                  {revealed ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
                  {revealed ? "隐藏明文" : "查看明文"}
                </Button>
                <Button size="sm" variant="outline" onClick={() => void archive(detail)}>
                  <ArchiveIcon className="size-4" />
                  归档
                </Button>
              </div>

              {revealed ? (
                <pre className="bg-muted max-h-48 overflow-auto rounded p-2 text-xs break-all whitespace-pre-wrap">
                  {revealed}
                </pre>
              ) : null}

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <div className="text-muted-foreground text-xs">大类</div>
                  <div>{pclassLabel(detail.pclass)}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">类型</div>
                  <div>{KIND_LABEL[detail.kind] ?? detail.kind}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">厂商</div>
                  <div>{providerLabel(detail)}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">形态</div>
                  <div>{detail.variant_label || detail.variant || "—"}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">服务族</div>
                  <div>{detail.service_family || "—"}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">协议</div>
                  <div>{detail.protocol || detail.proxy?.protocol || "—"}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">风险</div>
                  <div>{(RISK_LABEL[detail.risk] ?? detail.risk) || "—"}</div>
                </div>
                <div>
                  <div className="text-muted-foreground text-xs">额度类型</div>
                  <div>{QUOTA_LABEL[detail.quota_kind] ?? detail.quota_kind ?? "—"}</div>
                </div>
                <div className="col-span-2">
                  <div className="text-muted-foreground text-xs">来源</div>
                  <div className="break-all">{detail.source_url || "—"}</div>
                </div>
                <div className="col-span-2">
                  <div className="text-muted-foreground text-xs">证据</div>
                  <div className="break-all">{detail.evidence_path || "—"}</div>
                </div>
                {detail.context ? (
                  <div className="col-span-2">
                    <div className="text-muted-foreground text-xs">上下文 / 判断依据</div>
                    <div className="whitespace-pre-wrap">{detail.context}</div>
                  </div>
                ) : null}
              </div>

              <div className="rounded-md border p-3">
                <div className="mb-2 text-sm font-medium">分类与探测配置</div>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>
                    <span className="text-muted-foreground">命中规则：</span>
                    {String(detail.classification?.matched_by ?? "—")}
                  </div>
                  <div>
                    <span className="text-muted-foreground">置信度：</span>
                    {typeof detail.classification?.confidence === "number"
                      ? `${Math.round((detail.classification.confidence as number) * 100)}%`
                      : "—"}
                  </div>
                  <div className="col-span-2 break-all">
                    <span className="text-muted-foreground">Base：</span>
                    {String(detail.classification?.base ?? detail.extra?.base_url ?? "—")}
                  </div>
                  <div className="col-span-2">
                    <span className="text-muted-foreground">Model：</span>
                    {String(detail.classification?.model ?? "—")}
                  </div>
                  <div className="col-span-2">
                    <span className="text-muted-foreground">Avail：</span>
                    {String(detail.classification?.avail ?? "—")}
                  </div>
                </div>
              </div>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="rounded-md border p-3">
                  <div className="mb-2 text-sm font-medium">手动状态</div>
                  <Select
                    value={manualSelectValue(detail.manual_status)}
                    onValueChange={(v) => {
                      if (v === "custom") return;
                      void saveStatus(v === "auto" ? "" : v);
                    }}
                    disabled={busy === "status"}
                  >
                    <SelectTrigger className="h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">跟随探活</SelectItem>
                      <SelectItem value="valid">可用</SelectItem>
                      <SelectItem value="no_balance">余额不足</SelectItem>
                      <SelectItem value="rate_limited">限流</SelectItem>
                      <SelectItem value="invalid">失效</SelectItem>
                      <SelectItem value="unknown">未知</SelectItem>
                      {detail.manual_status && manualSelectValue(detail.manual_status) === "custom" ? (
                        <SelectItem value="custom">手动：{short(detail.manual_status, 20)}</SelectItem>
                      ) : null}
                    </SelectContent>
                  </Select>
                  <p className="text-muted-foreground mt-2 text-[11px]">
                    手动状态只影响展示，不覆盖探活历史。
                  </p>
                </div>

                <div className="rounded-md border p-3">
                  <div className="mb-2 text-sm font-medium">形态 / 调用入口</div>
                  <Select value={variantDraft || "auto"} onValueChange={setVariantDraft}>
                    <SelectTrigger className="h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">自动识别</SelectItem>
                      {detailVariants.map((v) => (
                        <SelectItem key={v.id} value={v.id}>
                          {v.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Input
                    value={baseDraft}
                    onChange={(e) => setBaseDraft(e.target.value)}
                    placeholder="自定义 Base URL（可选）"
                    className="mt-2 h-8 text-xs"
                  />
                  <Button
                    size="sm"
                    variant="outline"
                    className="mt-2 h-7"
                    disabled={busy === "variant"}
                    onClick={() => void saveVariant()}
                  >
                    {busy === "variant" ? <Spinner className="size-3.5" /> : null}
                    保存形态
                  </Button>
                </div>
              </div>

              <div>
                <div className="mb-2 flex items-center gap-2 text-sm font-medium">
                  <ActivityIcon className="size-4" />
                  探测历史
                </div>
                <div className="overflow-x-auto rounded-md border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-36">时间</TableHead>
                        <TableHead className="w-20">来源</TableHead>
                        <TableHead className="w-24">状态</TableHead>
                        <TableHead className="w-16">HTTP</TableHead>
                        <TableHead className="w-20">耗时</TableHead>
                        <TableHead>摘要</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {probes.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={6} className="text-muted-foreground py-4 text-center text-xs">
                            暂无探测历史
                          </TableCell>
                        </TableRow>
                      ) : (
                        probes.map((p) => (
                          <TableRow key={p.id}>
                            <TableCell className="text-muted-foreground text-xs">{fmtTime(p.ts)}</TableCell>
                            <TableCell className="text-xs">{p.source}</TableCell>
                            <TableCell className="text-xs">{STATUS_LABEL[p.status] ?? p.status}</TableCell>
                            <TableCell className="text-xs">{p.http || "—"}</TableCell>
                            <TableCell className="text-xs">{p.ms ? `${p.ms}ms` : "—"}</TableCell>
                            <TableCell className="max-w-[20rem] truncate text-xs" title={p.summary}>
                              {p.summary || "—"}
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </div>

              {detail.quota ? (
                <div>
                  <div className="mb-1 text-sm font-medium">额度快照</div>
                  <pre className="bg-muted max-h-48 overflow-auto rounded p-2 text-[11px] whitespace-pre-wrap">
                    {JSON.stringify(detail.quota, null, 2)}
                  </pre>
                </div>
              ) : null}

              {sources.length > 0 ? (
                <div>
                  <div className="mb-1 text-sm font-medium">其他来源</div>
                  <div className="flex flex-col gap-1 text-xs">
                    {sources.map((s, i) => (
                      <span key={`${s.url}-${i}`} className="break-all">
                        {s.url}
                      </span>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}

          <SheetFooter className="flex-row justify-end gap-2 px-4">
            <Button variant="outline" size="sm" onClick={() => setDetailOpen(false)}>
              关闭
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
