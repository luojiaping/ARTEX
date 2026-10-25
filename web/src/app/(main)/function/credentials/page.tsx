"use client";

import * as React from "react";

import {
  ActivityIcon,
  ArchiveIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ExternalLinkIcon,
  EyeIcon,
  EyeOffIcon,
  KeyRoundIcon,
  LayoutGridIcon,
  ListIcon,
  RefreshCwIcon,
  SearchIcon,
  ShieldCheckIcon,
  SlidersHorizontalIcon,
  XIcon,
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

const PAGE_SIZE = 24;
const VIEW_KEY = "artex.credentials.view";

const PCLASS = [
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
  deepseek_balance: "DeepSeek 余额",
  moonshot_balance: "Moonshot 余额",
  siliconflow_user: "SiliconFlow 账户",
  openrouter_key: "OpenRouter Credits",
  stepfun_balance: "StepFun 余额",
  aihubmix_balance: "AiHubMix 余额",
  commandcode_balance: "Command Code Credits",
  ollama_balance: "Ollama 余额",
  zhipu_coding: "GLM Coding",
  kimi_coding: "Kimi Coding",
  minimax_coding: "MiniMax 套餐",
  opencode_go_usage: "OpenCode Go",
  codex_wham: "Codex 用量",
  claude_usage: "Claude 用量",
  copilot_user: "Copilot",
};

const RISK_LABEL: Record<string, string> = {
  P0: "P0",
  P1: "P1",
  P2: "P2",
  P3: "P3",
  unrated: "未评级",
};

const PROTOCOL_LABEL: Record<string, string> = {
  chat: "Chat",
  responses: "Responses",
  anthropic: "Anthropic",
  gemini: "Gemini",
};

const SOURCE_LABEL: Record<string, string> = {
  legacy: "历史",
  verify: "本次",
  manual: "手动",
  both: "历史",
};

type ViewMode = "cards" | "list";

type Filters = {
  pclass: string;
  provider: string;
  variant: string;
  status: string;
  risk: string;
  protocol: string;
  q: string;
  quotaSupported: boolean;
  manual: boolean;
};

const EMPTY_FILTERS: Filters = {
  pclass: "",
  provider: "",
  variant: "",
  status: "",
  risk: "",
  protocol: "",
  q: "",
  quotaSupported: false,
  manual: false,
};

function statusMeta(status: string): { label: string; className: string; bar: string } {
  switch (status) {
    case "valid":
      return { label: "可用", className: "border-emerald-500/30 bg-emerald-500/10 text-emerald-600", bar: "bg-emerald-500" };
    case "invalid":
      return { label: "失效", className: "border-red-500/30 bg-red-500/10 text-red-600", bar: "bg-red-500" };
    case "no_balance":
      return { label: "余额不足", className: "border-amber-500/30 bg-amber-500/10 text-amber-600", bar: "bg-amber-500" };
    case "rate_limited":
      return { label: "限流", className: "border-amber-500/30 bg-amber-500/10 text-amber-600", bar: "bg-amber-500" };
    case "unknown":
      return { label: "未知", className: "bg-muted text-muted-foreground", bar: "bg-muted-foreground/40" };
    case "unsupported":
      return { label: "仅登记", className: "border-blue-500/30 bg-blue-500/10 text-blue-600", bar: "bg-blue-500" };
    case "unverified":
      return { label: "未探测", className: "bg-muted text-muted-foreground", bar: "bg-muted-foreground/30" };
    default:
      return { label: status || "—", className: "bg-muted text-muted-foreground", bar: "bg-muted-foreground/30" };
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

function fmtShort(value?: string): string {
  if (!value) return "未探测";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
}

function short(value: string, max = 44): string {
  if (!value) return "—";
  return value.length > max ? `${value.slice(0, max - 1)}…` : value;
}

function pclassLabel(value: string): string {
  return PCLASS.find((x) => x.value === value)?.label ?? (value || "未分类");
}

function providerLabel(e: CredentialEntry): string {
  return e.provider_name || e.provider || "未识别";
}

function classificationBase(e: CredentialEntry): string {
  const cls = e.classification;
  if (cls && typeof cls.base === "string" && cls.base) return cls.base;
  if (typeof e.extra?.base_url === "string") return e.extra.base_url;
  return "";
}

function hostOf(value: string): string {
  if (!value) return "—";
  try {
    return new URL(value).hostname;
  } catch {
    return value.replace(/^https?:\/\//, "").split("/")[0] || "—";
  }
}

function quotaText(e: CredentialEntry): string {
  const q = e.quota;
  if (q && typeof q.summary === "string" && q.summary) return q.summary;
  if (e.quota_kind && e.quota_kind !== "none") return QUOTA_LABEL[e.quota_kind] ?? e.quota_kind;
  return "—";
}

function quotaWindows(e: CredentialEntry): Record<string, unknown>[] {
  const q = e.quota;
  if (!q || !Array.isArray(q.windows)) return [];
  return (q.windows as unknown[]).filter((x): x is Record<string, unknown> => Boolean(x) && typeof x === "object");
}

function windowsOf(quota?: Record<string, unknown>): Record<string, unknown>[] {
  if (!quota || !Array.isArray(quota.windows)) return [];
  return (quota.windows as unknown[]).filter((x): x is Record<string, unknown> => Boolean(x) && typeof x === "object");
}

function num(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string") {
    const n = Number(value);
    return Number.isFinite(n) ? n : 0;
  }
  return 0;
}

function clsString(e: CredentialEntry, key: string): string {
  const v = e.classification?.[key];
  return typeof v === "string" ? v : "";
}

function clsArray(e: CredentialEntry, key: string): unknown[] {
  const v = e.classification?.[key];
  return Array.isArray(v) ? v : [];
}

const PROVIDER_COLORS = ["#58a6ff", "#a371f7", "#3fb950", "#d29922", "#f778ba", "#39c5cf", "#ff7b72", "#bc8cff"];

const PROVIDER_ICONS: Record<string, { file: string; mono?: boolean }> = {
  openai: { file: "openai.svg", mono: true },
  anthropic: { file: "anthropic.svg", mono: true },
  claude_oauth: { file: "anthropic.svg", mono: true },
  codex_oauth: { file: "openai.svg", mono: true },
  copilot: { file: "copilot-color.svg" },
  google_oauth: { file: "google-color.svg" },
  nous_oauth: { file: "nousresearch.svg", mono: true },
  deepseek: { file: "deepseek-color.svg" },
  kimi: { file: "kimi.svg", mono: true },
  kimi_oauth: { file: "kimi.svg", mono: true },
  glm: { file: "zhipu-color.svg" },
  dashscope: { file: "qwen-color.svg" },
  openrouter: { file: "openrouter.svg", mono: true },
  gemini: { file: "gemini-color.svg" },
  xai: { file: "xai.svg", mono: true },
  minimax: { file: "minimax-color.svg" },
  nvidia: { file: "nvidia-color.svg" },
  groq: { file: "groq.svg", mono: true },
  opencode: { file: "opencode.svg", mono: true },
  mimo: { file: "xiaomimimo.svg" },
  ark: { file: "volcengine-color.svg" },
  tencent: { file: "tencentcloud-color.svg" },
  lkeap: { file: "tencentcloud-color.svg" },
  huawei: { file: "huaweicloud-color.svg" },
  baidu: { file: "baiducloud-color.svg" },
  mistral: { file: "mistral-color.svg" },
  together: { file: "together-color.svg" },
  fireworks: { file: "fireworks-color.svg" },
  modelscope: { file: "modelscope-color.svg" },
  aihubmix: { file: "aihubmix-color.svg" },
  commandcode: { file: "commandcode.svg", mono: true },
  kilo: { file: "kilo.svg", mono: true },
  clinepass: { file: "cline.svg", mono: true },
  ollama: { file: "ollama.svg", mono: true },
  relay: { file: "newapi-color.svg" },
  sensenova: { file: "sensenova-color.svg" },
  xfyun: { file: "iflytekcloud-color.svg" },
  "302ai": { file: "ai302-color.svg" },
  cherryin: { file: "cherryin-color.svg" },
};

function providerColor(id: string): string {
  let hash = 0;
  for (const ch of id || "unknown") hash = (hash * 31 + ch.charCodeAt(0)) | 0;
  return PROVIDER_COLORS[Math.abs(hash) % PROVIDER_COLORS.length];
}

function ProviderMark({ provider, name }: { provider: string; name: string }) {
  const icon = PROVIDER_ICONS[provider];
  if (icon) {
    return (
      <span
        className="bg-background flex size-7 flex-none items-center justify-center rounded-md border"
        title={name || provider}
      >
        <img
          src={`/providers/${icon.file}`}
          alt=""
          className={cn("size-4 object-contain", icon.mono && "dark:brightness-0 dark:invert")}
        />
      </span>
    );
  }
  const color = providerColor(provider || name);
  const text = (name || provider || "?").replace(/[^\p{L}\p{N}]/gu, "").slice(0, 1).toUpperCase();
  return (
    <span
      className="flex size-7 flex-none items-center justify-center rounded-md border text-[11px] font-semibold"
      style={{ color, borderColor: `${color}55`, background: `${color}16` }}
      title={name || provider}
    >
      {text || "?"}
    </span>
  );
}

function StatusBadge({ entry }: { entry: CredentialEntry }) {
  const meta = statusMeta(effectiveStatus(entry));
  const label =
    entry.last_probe_source === "legacy" && !entry.manual_status
      ? `${effectiveLabel(entry)} · 历史`
      : effectiveLabel(entry);
  return (
    <Badge variant="outline" className={cn("max-w-full truncate", meta.className)} title="历史 = 旧控制台结论，不是本次实测">
      {label}
    </Badge>
  );
}

function QuotaBars({ windows }: { windows: Record<string, unknown>[] }) {
  if (windows.length === 0) return null;
  return (
    <div className="flex flex-col gap-1.5">
      {windows.slice(0, 3).map((w, i) => {
        const used = Math.min(100, Math.max(0, num(w.used)));
        const tone = used >= 90 ? "bg-red-500" : used >= 70 ? "bg-amber-500" : "bg-emerald-500";
        return (
          <div key={`${String(w.name)}-${i}`} className="flex items-center gap-2 text-[11px]">
            <span className="text-muted-foreground w-14 flex-none truncate">{String(w.name ?? "窗口")}</span>
            <div className="bg-muted h-1.5 min-w-0 flex-1 overflow-hidden rounded-full">
              <div className={cn("h-full rounded-full", tone)} style={{ width: `${used}%` }} />
            </div>
            <span className="text-muted-foreground w-10 flex-none text-right tabular-nums">{Math.round(used)}%</span>
          </div>
        );
      })}
    </div>
  );
}

function CredentialCard({
  entry,
  selected,
  busy,
  onSelect,
  onOpen,
  onVerify,
  onProbe,
  onArchive,
}: {
  entry: CredentialEntry;
  selected: boolean;
  busy: string;
  onSelect: (checked: boolean) => void;
  onOpen: () => void;
  onVerify: () => void;
  onProbe: () => void;
  onArchive: () => void;
}) {
  const meta = statusMeta(effectiveStatus(entry));
  const base = classificationBase(entry);
  const windows = quotaWindows(entry);
  const verifiable = ["llm_key", "oauth", "api_key", "token"].includes(entry.kind);
  return (
    <div
      className={cn(
        "group bg-card relative flex cursor-pointer flex-col gap-2.5 rounded-lg border p-3 pl-4 shadow-xs transition-colors hover:border-foreground/25",
        selected && "border-primary/60 ring-primary/20 ring-1",
      )}
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <span className={cn("absolute inset-y-2 left-1 w-[3px] rounded-full", meta.bar)} />
      <div className="flex items-start gap-2">
        <div onClick={(e) => e.stopPropagation()}>
          <Checkbox checked={selected} onCheckedChange={(v) => onSelect(v === true)} className="mt-0.5" />
        </div>
        <ProviderMark provider={entry.provider} name={providerLabel(entry)} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate text-sm font-medium">{providerLabel(entry)}</span>
            {entry.variant_label || entry.variant ? (
              <Badge variant="outline" className="max-w-[9rem] truncate text-[10px]">
                {entry.variant_label || entry.variant}
              </Badge>
            ) : null}
            <span className="ml-auto flex-none">
              <StatusBadge entry={entry} />
            </span>
          </div>
          <div className="text-muted-foreground mt-0.5 truncate text-xs" title={entry.service}>
            {entry.service || KIND_LABEL[entry.kind] || entry.kind}
          </div>
        </div>
      </div>

      <div className="bg-muted/30 flex items-center gap-2 rounded-md border px-2 py-1.5">
        <code className="min-w-0 flex-1 truncate text-[11px]">{entry.masked || "—"}</code>
        <span className="text-muted-foreground max-w-[45%] flex-none truncate text-[10px]" title={base}>
          {hostOf(base)}
        </span>
      </div>

      {windows.length > 0 ? <QuotaBars windows={windows} /> : null}

      <div className="text-muted-foreground flex min-w-0 items-center gap-2 text-[11px]">
        <span className="truncate" title={quotaText(entry)}>
          {quotaText(entry)}
        </span>
        <span className="ml-auto flex-none tabular-nums">{fmtShort(entry.checked_at)}</span>
      </div>

      <div className="flex items-center gap-1" onClick={(e) => e.stopPropagation()}>
        <Button size="sm" variant="ghost" className="h-7 px-2" onClick={onOpen}>
          详情
        </Button>
        {verifiable ? (
          <Button size="sm" variant="ghost" className="h-7 px-2" disabled={busy === `verify:${entry.uid}`} onClick={onVerify}>
            {busy === `verify:${entry.uid}` ? <Spinner className="size-3" /> : "验活"}
          </Button>
        ) : null}
        {entry.kind === "proxy_node" ? (
          <Button size="sm" variant="ghost" className="h-7 px-2" disabled={busy === `probe:${entry.uid}`} onClick={onProbe}>
            {busy === `probe:${entry.uid}` ? <Spinner className="size-3" /> : "探测"}
          </Button>
        ) : null}
        <Badge variant="outline" className="text-muted-foreground ml-1 text-[10px]">
          {pclassLabel(entry.pclass)}
        </Badge>
        <Button
          size="icon"
          variant="ghost"
          className="text-muted-foreground ml-auto size-7"
          disabled={busy === `archive:${entry.uid}`}
          onClick={onArchive}
          title="归档"
        >
          <ArchiveIcon className="size-3.5" />
        </Button>
      </div>
    </div>
  );
}

function StatTile({
  label,
  value,
  active,
  tone,
  onClick,
}: {
  label: string;
  value: number;
  active: boolean;
  tone?: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "bg-card rounded-lg border p-3 text-left transition-colors hover:border-foreground/25",
        active && "border-primary/60 ring-primary/15 ring-1",
      )}
    >
      <div className="text-muted-foreground text-[11px]">{label}</div>
      <div className={cn("mt-1 text-xl font-semibold tabular-nums", tone)}>{value.toLocaleString()}</div>
    </button>
  );
}

export default function CredentialsPage() {
  const [view, setView] = React.useState<ViewMode>("cards");
  const [facets, setFacets] = React.useState<CredentialFacets | null>(null);
  const [providers, setProviders] = React.useState<CredentialProvider[]>([]);
  const [items, setItems] = React.useState<CredentialEntry[]>([]);
  const [total, setTotal] = React.useState(0);
  const [loading, setLoading] = React.useState(true);
  const [busy, setBusy] = React.useState("");
  const [page, setPage] = React.useState(0);
  const [qInput, setQInput] = React.useState("");
  const [more, setMore] = React.useState(false);
  const [filters, setFilters] = React.useState<Filters>(EMPTY_FILTERS);
  const [selected, setSelected] = React.useState<string[]>([]);

  const [detailOpen, setDetailOpen] = React.useState(false);
  const [detail, setDetail] = React.useState<CredentialEntry | null>(null);
  const [sources, setSources] = React.useState<CredentialSource[]>([]);
  const [probes, setProbes] = React.useState<CredentialProbe[]>([]);
  const [revealed, setRevealed] = React.useState("");
  const [variantDraft, setVariantDraft] = React.useState("");
  const [baseDraft, setBaseDraft] = React.useState("");

  React.useEffect(() => {
    const saved = window.localStorage.getItem(VIEW_KEY);
    if (saved === "cards" || saved === "list") setView(saved);
  }, []);

  const setViewMode = (mode: ViewMode) => {
    setView(mode);
    window.localStorage.setItem(VIEW_KEY, mode);
  };

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
          protocol: filters.protocol || undefined,
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
  const protocolOptions = facets?.by_protocol ?? [];

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
    setBaseDraft(classificationBase(entry));
    setDetailOpen(true);
    try {
      const r = await api.credential(entry.uid);
      setDetail(r.entry);
      setSources(r.sources ?? []);
      setProbes(r.probes ?? []);
      setVariantDraft(r.entry.variant || "");
      setBaseDraft(classificationBase(r.entry));
    } catch (e) {
      toast.error("读取详情失败：" + (e as Error).message);
    }
  };

  const verifyOne = async (entry: CredentialEntry) => {
    setBusy(`verify:${entry.uid}`);
    try {
      const r = await api.verifyCredential(entry.uid);
      toast.success(`${providerLabel(entry)} → ${statusMeta(r.status).label}`);
      await load();
      if (detail?.uid === entry.uid) {
        const fresh = await api.credential(entry.uid);
        setDetail(fresh.entry);
        setProbes(fresh.probes ?? []);
        setSources(fresh.sources ?? []);
      }
    } catch (e) {
      toast.error("验活失败：" + (e as Error).message);
    } finally {
      setBusy("");
    }
  };

  const probeOne = async (entry: CredentialEntry) => {
    setBusy(`probe:${entry.uid}`);
    try {
      const r = await api.probeProxy(entry.uid);
      toast.success(r.status === "valid" ? `节点可用 ${r.delay_ms ?? 0}ms` : `节点不可用：${r.message || r.status}`);
      await load();
      if (detail?.uid === entry.uid) {
        const fresh = await api.credential(entry.uid);
        setDetail(fresh.entry);
        setProbes(fresh.probes ?? []);
        setSources(fresh.sources ?? []);
      }
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
    setBusy(`archive:${entry.uid}`);
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
    setFilters(EMPTY_FILTERS);
    setQInput("");
    setPage(0);
  };

  const setFilter = <K extends keyof Filters>(key: K, value: Filters[K]) => {
    setFilters((f) => ({ ...f, [key]: value }));
    setPage(0);
  };

  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const detailProvider = providers.find((p) => p.id === detail?.provider);
  const detailVariants = detailProvider?.variants ?? [];
  const detailStatus = detail ? effectiveStatus(detail) : "";
  const detailWindows = windowsOf(detail?.quota);
  const detailEndpoints = detail ? clsArray(detail, "endpoints") : [];
  const detailModels = detail ? clsArray(detail, "models") : [];
  const keysURL = detail ? clsString(detail, "keys_url") || detailProvider?.keys_url || "" : "";
  const website = detail ? clsString(detail, "website") || detailProvider?.website || "" : "";

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            <KeyRoundIcon className="text-muted-foreground size-5" />
            凭据
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            厂商、形态、可用性、额度与探测历史。默认只显示掩码，验活需手动触发。
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="bg-muted flex items-center rounded-md p-0.5">
            <Button
              variant={view === "cards" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2"
              onClick={() => setViewMode("cards")}
              title="卡片视图"
            >
              <LayoutGridIcon className="size-4" />
            </Button>
            <Button
              variant={view === "list" ? "secondary" : "ghost"}
              size="sm"
              className="h-7 px-2"
              onClick={() => setViewMode("list")}
              title="列表视图"
            >
              <ListIcon className="size-4" />
            </Button>
          </div>
          <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
            <RefreshCwIcon className={cn("size-4", loading && "animate-spin")} />
            刷新
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2 md:grid-cols-3 xl:grid-cols-6">
        <StatTile
          label="凭据总数"
          value={facets?.total ?? 0}
          active={!filters.pclass && !filters.status && !filters.quotaSupported && !filters.provider && !filters.q}
          onClick={resetFilters}
        />
        <StatTile
          label="可用"
          value={facets?.valid ?? 0}
          active={filters.status === "valid"}
          tone="text-emerald-600"
          onClick={() => setFilter("status", filters.status === "valid" ? "" : "valid")}
        />
        <StatTile
          label="余额不足 / 限流"
          value={facets?.warn ?? 0}
          active={filters.status === "warn"}
          tone="text-amber-600"
          onClick={() => setFilter("status", filters.status === "warn" ? "" : "warn")}
        />
        <StatTile
          label="未探测"
          value={facets?.unverified ?? 0}
          active={filters.status === "unverified"}
          onClick={() => setFilter("status", filters.status === "unverified" ? "" : "unverified")}
        />
        <StatTile
          label="支持额度"
          value={facets?.quota_supported ?? 0}
          active={filters.quotaSupported}
          onClick={() => setFilter("quotaSupported", !filters.quotaSupported)}
        />
        <StatTile
          label="代理节点存活"
          value={facets?.proxy_alive ?? 0}
          active={filters.pclass === "proxy" && filters.status === "valid"}
          tone="text-blue-600"
          onClick={() => {
            const on = filters.pclass === "proxy" && filters.status === "valid";
            setFilters((f) => ({ ...f, pclass: on ? "" : "proxy", status: on ? "" : "valid" }));
            setPage(0);
          }}
        />
      </div>

      <Card className="gap-0">
        <CardHeader className="gap-3 border-b pb-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
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
              <Button variant="outline" size="sm" onClick={() => setMore((v) => !v)}>
                <SlidersHorizontalIcon className="size-4" />
                更多
              </Button>
              <Button variant="ghost" size="sm" onClick={resetFilters}>
                <XIcon className="size-4" />
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
              <SelectTrigger className="h-8 w-48">
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
              onValueChange={(v) => setFilter("variant", v === "all" ? "" : v)}
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
              onValueChange={(v) => setFilter("status", v === "all" ? "" : v)}
            >
              <SelectTrigger className="h-8 w-36">
                <SelectValue placeholder="全部状态" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部状态</SelectItem>
                <SelectItem value="warn">余额不足 / 限流</SelectItem>
                {statusOptions.map((s) => (
                  <SelectItem key={s.value} value={s.value}>
                    {(STATUS_LABEL[s.value] ?? s.value) + " (" + s.n + ")"}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {more ? (
              <>
                <Select
                  value={filters.risk || "all"}
                  onValueChange={(v) => setFilter("risk", v === "all" ? "" : v)}
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
                <Select
                  value={filters.protocol || "all"}
                  onValueChange={(v) => setFilter("protocol", v === "all" ? "" : v)}
                >
                  <SelectTrigger className="h-8 w-32">
                    <SelectValue placeholder="协议" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">全部协议</SelectItem>
                    {protocolOptions.map((p) => (
                      <SelectItem key={p.value} value={p.value}>
                        {(PROTOCOL_LABEL[p.value] ?? p.value) + " (" + p.n + ")"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <label className="text-muted-foreground flex items-center gap-2 text-xs">
                  <Switch
                    checked={filters.quotaSupported}
                    onCheckedChange={(v) => setFilter("quotaSupported", v)}
                  />
                  仅支持额度
                </label>
                <label className="text-muted-foreground flex items-center gap-2 text-xs">
                  <Switch checked={filters.manual} onCheckedChange={(v) => setFilter("manual", v)} />
                  仅手动标记
                </label>
              </>
            ) : null}
          </div>

          {selected.length > 0 ? (
            <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-xs">
              <span>已选 {selected.length} 条</span>
              <Button
                size="sm"
                variant="outline"
                className="h-7"
                onClick={() => void verifySelected()}
                disabled={busy === "batch-verify"}
              >
                {busy === "batch-verify" ? <Spinner className="size-3.5" /> : <ShieldCheckIcon className="size-3.5" />}
                批量验活（{verifiableSelected.length}）
              </Button>
              <Button size="sm" variant="ghost" className="h-7" onClick={() => setSelected([])}>
                清空选择
              </Button>
            </div>
          ) : null}
        </CardHeader>

        <CardContent className="p-3">
          {loading ? (
            <div className="flex items-center justify-center py-16">
              <Spinner className="size-5" />
            </div>
          ) : items.length === 0 ? (
            <div className="text-muted-foreground py-16 text-center text-sm">没有匹配的凭据。</div>
          ) : view === "cards" ? (
            <div className="grid gap-2.5 [grid-template-columns:repeat(auto-fill,minmax(300px,1fr))]">
              {items.map((entry) => (
                <CredentialCard
                  key={entry.uid}
                  entry={entry}
                  selected={selected.includes(entry.uid)}
                  busy={busy}
                  onSelect={(checked) =>
                    setSelected((prev) =>
                      checked ? [...prev, entry.uid] : prev.filter((uid) => uid !== entry.uid),
                    )
                  }
                  onOpen={() => void openDetail(entry)}
                  onVerify={() => void verifyOne(entry)}
                  onProbe={() => void probeOne(entry)}
                  onArchive={() => void archive(entry)}
                />
              ))}
            </div>
          ) : (
            <div className="overflow-x-auto">
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
                    <TableHead className="w-52">厂商 / 形态</TableHead>
                    <TableHead>服务 / 目标</TableHead>
                    <TableHead className="hidden w-32 xl:table-cell">掩码</TableHead>
                    <TableHead className="hidden w-32 2xl:table-cell">额度</TableHead>
                    <TableHead className="hidden w-28 2xl:table-cell">最近</TableHead>
                    <TableHead className="w-32 text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((entry) => (
                    <TableRow key={entry.uid}>
                      <TableCell>
                        <Checkbox
                          checked={selected.includes(entry.uid)}
                          onCheckedChange={(checked) =>
                            setSelected((prev) =>
                              checked === true ? [...prev, entry.uid] : prev.filter((uid) => uid !== entry.uid),
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <StatusBadge entry={entry} />
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="truncate text-sm">{providerLabel(entry)}</span>
                          <span className="text-muted-foreground truncate text-[11px]">
                            {entry.variant_label || entry.variant || pclassLabel(entry.pclass)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="truncate text-sm">{entry.service || KIND_LABEL[entry.kind] || entry.kind}</span>
                          <span className="text-muted-foreground truncate text-[11px]">{short(entry.source_url, 52)}</span>
                        </div>
                      </TableCell>
                      <TableCell className="hidden xl:table-cell">
                        <code className="text-xs">{entry.masked || "—"}</code>
                      </TableCell>
                      <TableCell className="hidden text-xs 2xl:table-cell">{quotaText(entry)}</TableCell>
                      <TableCell className="text-muted-foreground hidden text-xs 2xl:table-cell">
                        {fmtShort(entry.checked_at)}
                      </TableCell>
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
                              disabled={busy === `verify:${entry.uid}`}
                              onClick={() => void verifyOne(entry)}
                            >
                              {busy === `verify:${entry.uid}` ? <Spinner className="size-3" /> : "验活"}
                            </Button>
                          ) : null}
                          {entry.kind === "proxy_node" ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-7 px-2"
                              disabled={busy === `probe:${entry.uid}`}
                              onClick={() => void probeOne(entry)}
                            >
                              {busy === `probe:${entry.uid}` ? <Spinner className="size-3" /> : "探测"}
                            </Button>
                          ) : null}
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}

          <div className="flex items-center justify-between border-t pt-3">
            <span className="text-muted-foreground text-xs">
              共 {total.toLocaleString()} 条 · 第 {page + 1}/{pages} 页
            </span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={page <= 0 || loading} onClick={() => setPage((p) => p - 1)}>
                <ChevronLeftIcon className="size-4" />
                上一页
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={page + 1 >= pages || loading}
                onClick={() => setPage((p) => p + 1)}
              >
                下一页
                <ChevronRightIcon className="size-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <Sheet open={detailOpen} onOpenChange={setDetailOpen}>
        <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
          <SheetHeader>
            <SheetTitle className="flex flex-wrap items-center gap-2">
              {detail ? <ProviderMark provider={detail.provider} name={providerLabel(detail)} /> : null}
              {detail ? providerLabel(detail) : "凭据详情"}
              {detail ? <StatusBadge entry={detail} /> : null}
            </SheetTitle>
            <SheetDescription>
              {detail ? `${detail.uid} · ${detail.service || KIND_LABEL[detail.kind] || detail.kind}` : ""}
            </SheetDescription>
          </SheetHeader>

          {detail ? (
            <div className="flex flex-col gap-5 px-4 pb-4 text-sm">
              <div className="flex flex-wrap gap-2">
                {["llm_key", "oauth", "api_key", "token"].includes(detail.kind) ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy === `verify:${detail.uid}`}
                    onClick={() => void verifyOne(detail)}
                  >
                    {busy === `verify:${detail.uid}` ? <Spinner className="size-4" /> : <ShieldCheckIcon className="size-4" />}
                    验活
                  </Button>
                ) : null}
                {detail.kind === "proxy_node" ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy === `probe:${detail.uid}`}
                    onClick={() => void probeOne(detail)}
                  >
                    <ActivityIcon className="size-4" />
                    探测
                  </Button>
                ) : null}
                <Button size="sm" variant="outline" onClick={() => void reveal()}>
                  {revealed ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
                  {revealed ? "隐藏明文" : "查看明文"}
                </Button>
                {keysURL ? (
                  <Button size="sm" variant="outline" asChild>
                    <a href={keysURL} target="_blank" rel="noreferrer">
                      <ExternalLinkIcon className="size-4" />
                      厂商控制台
                    </a>
                  </Button>
                ) : null}
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
                  <div className="text-muted-foreground text-xs">分类协议</div>
                  <div>{(PROTOCOL_LABEL[clsString(detail, "protocol")] ?? clsString(detail, "protocol")) || "—"}</div>
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
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div className="text-sm font-medium">探测配置</div>
                  <div className="text-muted-foreground text-[11px]">
                    命中 {clsString(detail, "matched_by") || "—"}
                    {typeof detail.classification?.confidence === "number"
                      ? ` · ${Math.round((detail.classification.confidence as number) * 100)}%`
                      : ""}
                  </div>
                </div>
                <div className="flex flex-col gap-2 text-xs">
                  {detailEndpoints.length > 0 ? (
                    detailEndpoints.map((raw, i) => {
                      const ep = (raw ?? {}) as Record<string, unknown>;
                      return (
                        <div key={i} className="flex items-start gap-2">
                          <Badge variant="outline" className="flex-none text-[10px]">
                            {PROTOCOL_LABEL[String(ep.protocol ?? "")] ?? String(ep.protocol ?? "—")}
                          </Badge>
                          <span className="min-w-0 break-all font-mono">{String(ep.base ?? "")}</span>
                        </div>
                      );
                    })
                  ) : (
                    <div className="break-all font-mono">{classificationBase(detail) || "—"}</div>
                  )}
                  <div>
                    <span className="text-muted-foreground">模型：</span>
                    {detailModels.length > 0 ? detailModels.map((m) => String(m)).join(", ") : clsString(detail, "model") || "—"}
                  </div>
                  <div>
                    <span className="text-muted-foreground">鉴权：</span>
                    {clsString(detail, "auth") || "自动"}
                  </div>
                  {website ? (
                    <div className="break-all">
                      <span className="text-muted-foreground">厂商文档：</span>
                      <a className="underline underline-offset-2" href={website} target="_blank" rel="noreferrer">
                        {website}
                      </a>
                    </div>
                  ) : null}
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
                  <p className="text-muted-foreground mt-2 text-[11px]">手动状态只影响展示，不覆盖探测历史。</p>
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

              {detail.quota ? (
                <div className="rounded-md border p-3">
                  <div className="mb-2 flex items-center justify-between">
                    <div className="text-sm font-medium">额度快照</div>
                    <div className="text-muted-foreground text-[11px]">{quotaText(detail)}</div>
                  </div>
                  {detailWindows.length > 0 ? (
                    <div className="mb-2">
                      <QuotaBars windows={detailWindows} />
                    </div>
                  ) : null}
                  <details>
                    <summary className="text-muted-foreground cursor-pointer text-[11px]">原始 JSON</summary>
                    <pre className="bg-muted mt-2 max-h-48 overflow-auto rounded p-2 text-[11px] whitespace-pre-wrap">
                      {JSON.stringify(detail.quota, null, 2)}
                    </pre>
                  </details>
                </div>
              ) : null}

              <div>
                <div className="mb-2 flex items-center gap-2 text-sm font-medium">
                  <ActivityIcon className="size-4" />
                  探测历史
                </div>
                <div className="overflow-x-auto rounded-md border">
                  <Table className="min-w-[620px]">
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-32">时间</TableHead>
                        <TableHead className="w-16">来源</TableHead>
                        <TableHead className="w-20">状态</TableHead>
                        <TableHead className="w-20">协议</TableHead>
                        <TableHead className="w-32">模型</TableHead>
                        <TableHead className="w-14">HTTP</TableHead>
                        <TableHead className="w-16">耗时</TableHead>
                        <TableHead>摘要</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {probes.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={8} className="text-muted-foreground py-4 text-center text-xs">
                            暂无探测历史
                          </TableCell>
                        </TableRow>
                      ) : (
                        probes.map((p) => (
                          <TableRow key={p.id}>
                            <TableCell className="text-muted-foreground text-xs">{fmtShort(p.ts)}</TableCell>
                            <TableCell className="text-xs">{SOURCE_LABEL[p.source] ?? p.source}</TableCell>
                            <TableCell className="text-xs">{STATUS_LABEL[p.status] ?? p.status}</TableCell>
                            <TableCell className="text-xs">
                              {PROTOCOL_LABEL[p.protocol] ?? p.protocol ?? "—"}
                            </TableCell>
                            <TableCell className="max-w-[8rem] truncate text-xs" title={p.model}>
                              {p.model || "—"}
                            </TableCell>
                            <TableCell className="text-xs">{p.http || "—"}</TableCell>
                            <TableCell className="text-xs">{p.ms ? `${p.ms}ms` : "—"}</TableCell>
                            <TableCell className="max-w-[16rem] truncate text-xs" title={p.summary}>
                              {p.summary || "—"}
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </div>

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
