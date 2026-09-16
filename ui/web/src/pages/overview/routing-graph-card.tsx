import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Clapperboard, MessagesSquare, Presentation, Send } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatTokens } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import type { ChannelStatusEntry, RoutingEdge } from "./types";

interface NodeAgg {
  name: string;
  calls: number;
  tokens: number;
}

/** One entry surface around the gateway hub. */
interface Surface {
  key: "telegram" | "web" | "pptx" | "video";
  label: string;
  status: "active" | "idle" | "offline";
  calls24h: number;
}

const MAX_TABLE_MODELS = 8;
const ROW_H = 46;
const NODE_W = 168;
const CENTER_W = 116;
const GRAPH_MIN_WIDTH = 620;
const REFRESH_INTERVAL = 30_000;
const DAY_MS = 24 * 60 * 60 * 1000;

const DOT_COLORS = [
  "bg-blue-500",
  "bg-violet-500",
  "bg-emerald-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-cyan-500",
];

const AVATAR_TINTS = [
  "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  "bg-violet-500/15 text-violet-600 dark:text-violet-400",
  "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  "bg-rose-500/15 text-rose-600 dark:text-rose-400",
  "bg-cyan-500/15 text-cyan-600 dark:text-cyan-400",
];

function compactCount(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function hashIndex(name: string): number {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) | 0;
  return Math.abs(h);
}

function dotColor(name: string): string {
  return DOT_COLORS[hashIndex(name) % DOT_COLORS.length] ?? DOT_COLORS[0]!;
}

function avatarTint(name: string): string {
  return AVATAR_TINTS[hashIndex(name) % AVATAR_TINTS.length] ?? AVATAR_TINTS[0]!;
}

function aggregate(
  edges: RoutingEdge[],
  key: "provider" | "model",
  max: number,
): NodeAgg[] {
  const map = new Map<string, NodeAgg>();
  for (const e of edges) {
    const name = e[key];
    const agg = map.get(name) ?? { name, calls: 0, tokens: 0 };
    agg.calls += e.calls;
    agg.tokens += e.input_tokens + e.output_tokens;
    map.set(name, agg);
  }
  return [...map.values()].sort((a, b) => b.calls - a.calls).slice(0, max);
}

const SURFACE_ICONS = {
  telegram: Send,
  web: MessagesSquare,
  pptx: Presentation,
  video: Clapperboard,
} as const;

const EDGE_STROKE = {
  active: "text-emerald-500/80 routing-edge-active",
  idle: "text-muted-foreground/30",
  offline: "text-muted-foreground/20",
} as const;

const NODE_BORDER = {
  active: "border-emerald-500/60",
  idle: "border-border",
  offline: "border-border opacity-60",
} as const;

/** 9router-style surface topology: the entry surfaces an operator actually
 * uses (Telegram, web chat, PPTX, video editor) orbit the GoClaw hub — the
 * hub carries the claw logo, and an active surface gets an animated dashed
 * edge flowing into it plus a ping dot. The per-model usage table underneath
 * keeps the real model routing data. Falls back to a compact edge list on
 * narrow screens. */
export function RoutingGraphCard({
  channelEntries = [],
  clientCount = 0,
}: {
  channelEntries?: [string, ChannelStatusEntry][];
  clientCount?: number;
}) {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const { agents } = useAgents();

  const { data } = useQuery({
    queryKey: ["usage", "routing"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => http.get<{ window_hours: number; edges: RoutingEdge[] }>("/v1/usage/routing", { hours: "24", limit: "24" }),
  });
  const edges = data?.edges ?? [];

  // Per-surface activity, 24h: agent requests grouped by agent UUID, mapped
  // to agent keys, plus live video jobs as a strong editor signal.
  const { data: agentBreakdown } = useQuery({
    queryKey: ["usage", "breakdown", "agent", "24h", "routing-active"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - DAY_MS);
      return http.get<{ rows: { key: string; request_count: number }[] }>("/v1/usage/breakdown", {
        group_by: "agent",
        from: from.toISOString(),
        to: to.toISOString(),
      });
    },
  });
  const { data: videoJobs } = useQuery({
    queryKey: ["video", "jobs", "routing-active"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => http.get<{ jobs: { status: string; updated_at: string }[] }>("/v1/video/jobs", { limit: "10" }),
  });

  const agentCalls24h = useMemo(() => {
    const idToKey = new Map((agents ?? []).map((a) => [a.id, a.agent_key] as const));
    const byKey = new Map<string, number>();
    for (const row of agentBreakdown?.rows ?? []) {
      const key = idToKey.get(row.key) ?? row.key;
      byKey.set(key, (byKey.get(key) ?? 0) + row.request_count);
    }
    return byKey;
  }, [agents, agentBreakdown]);

  const surfaces = useMemo<Surface[]>(() => {
    const telegram = channelEntries.find(([name]) => name.toLowerCase().includes("telegram"));
    const telegramRunning = telegram?.[1]?.running ?? false;
    const now = Date.now();
    const videoBusy = (videoJobs?.jobs ?? []).some((j) => {
      if (j.status === "queued" || j.status === "rendering") return true;
      const updated = new Date(j.updated_at).getTime();
      return Number.isFinite(updated) && now - updated < DAY_MS;
    });
    const surface = (key: Surface["key"], active: boolean, calls24h: number, offline = false): Surface => ({
      key,
      label: t(`routing.surfaces.${key}`),
      status: offline ? "offline" : active ? "active" : "idle",
      calls24h,
    });
    return [
      surface("telegram", telegramRunning, agentCalls24h.get("telegram") ?? 0, telegram !== undefined && !telegramRunning),
      surface("web", clientCount > 0, 0),
      surface("pptx", (agentCalls24h.get("pptx-designer") ?? 0) > 0, agentCalls24h.get("pptx-designer") ?? 0),
      surface("video", videoBusy || (agentCalls24h.get("video-designer") ?? 0) > 0, agentCalls24h.get("video-designer") ?? 0),
    ];
  }, [t, channelEntries, clientCount, agentCalls24h, videoJobs]);

  const models = useMemo(() => aggregate(edges, "model", MAX_TABLE_MODELS), [edges]);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setWidth(el.clientWidth);
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const totalCalls = edges.reduce((s, e) => s + e.calls, 0);
  const req = (n: number) => t("routing.req", { count: compactCount(n) });
  const modelsFolded = new Set(edges.map((e) => e.model)).size - models.length;
  const moreLabel = modelsFolded > 0 ? t("routing.moreModels", { count: modelsFolded }) : "";

  // The measured wrapper renders in EVERY branch (narrow list, wide graph,
  // noData) so the ResizeObserver attached once at mount always has a node;
  // conditional ref divs would leave width=0 forever on cold load.
  return (
    <div ref={containerRef} className="w-full">
      {width > 0 && width < GRAPH_MIN_WIDTH ? (
        <NarrowList
          edges={edges}
          req={req}
          moreLabel={moreLabel}
          title={t("routing.title")}
          window={t("routing.window")}
          noData={t("routing.noData")}
        />
      ) : (
        <WideGraph
          width={width}
          edges={edges}
          models={models}
          surfaces={surfaces}
          totalCalls={totalCalls}
          req={req}
          moreLabel={moreLabel}
          title={t("routing.title")}
          window={t("routing.window")}
          noData={t("routing.noData")}
          byModel={t("routing.byModel")}
          colModel={t("recentRequests.columns.model")}
          colInOut={t("recentRequests.columns.inOut")}
          colRequests={t("routing.columns.requests")}
          colShare={t("routing.columns.share")}
          idleLabel={t("routing.surfaceIdle")}
          offlineLabel={t("routing.surfaceOffline")}
        />
      )}
    </div>
  );
}

function NarrowList({
  edges,
  req,
  moreLabel,
  title,
  window: windowLabel,
  noData,
}: {
  edges: RoutingEdge[];
  req: (n: number) => string;
  moreLabel: string;
  title: string;
  window: string;
  noData: string;
}) {
  return (
    <Card>
      <GraphHeader title={title} window={windowLabel} />
      <CardContent>
        {edges.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{noData}</p>
        ) : (
          <ul className="space-y-2">
            {edges.slice(0, 8).map((e) => (
              <li key={`${e.provider}-${e.model}`} className="flex items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2">
                <div className="min-w-0">
                  <p className="flex items-center gap-1.5 truncate text-xs font-medium">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor(e.provider)}`} />
                    <span className="truncate">{e.provider}</span>
                    <span className="text-muted-foreground">→</span>
                    <span className="truncate font-mono">{e.model}</span>
                  </p>
                  <p className="text-[10px] text-muted-foreground tabular-nums">
                    {formatTokens(e.input_tokens + e.output_tokens)} tok
                  </p>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {req(e.calls)}
                </span>
              </li>
            ))}
          </ul>
        )}
        {moreLabel && <p className="mt-2 text-[11px] text-muted-foreground">{moreLabel}</p>}
      </CardContent>
    </Card>
  );
}

function WideGraph(props: {
  width: number;
  edges: RoutingEdge[];
  models: NodeAgg[];
  surfaces: Surface[];
  totalCalls: number;
  req: (n: number) => string;
  moreLabel: string;
  title: string;
  window: string;
  noData: string;
  byModel: string;
  colModel: string;
  colInOut: string;
  colRequests: string;
  colShare: string;
  idleLabel: string;
  offlineLabel: string;
}) {
  const {
    width, edges, models, surfaces, totalCalls, req, moreLabel,
    title, window: windowLabel, noData, byModel, colModel, colInOut, colRequests, colShare,
    idleLabel, offlineLabel,
  } = props;

  // --- Ellipse layout: 4 surfaces at the compass points around the hub ---
  const n = Math.max(surfaces.length, 1);
  const cx = width / 2;
  const rx = Math.max(
    170,
    Math.min(((NODE_W + 20) * n) / (2 * Math.PI), width / 2 - NODE_W / 2 - 8),
  );
  const ry = Math.max(110, Math.min(rx * 0.55, 170));
  const height = 2 * ry + ROW_H;
  const surfacePos = surfaces.map((s, i) => {
    const angle = -Math.PI / 2 + (2 * Math.PI * i) / n;
    return {
      s,
      x: cx + rx * Math.cos(angle) - NODE_W / 2,
      y: height / 2 + ry * Math.sin(angle) - ROW_H / 2,
    };
  });

  const inOutByModel = new Map(models.map((m) => {
    let inTok = 0;
    let outTok = 0;
    for (const e of edges) {
      if (e.model === m.name) {
        inTok += e.input_tokens;
        outTok += e.output_tokens;
      }
    }
    return [m.name, { inTok, outTok }];
  }));

  return (
    <Card>
      <GraphHeader title={title} window={windowLabel} />
      <CardContent>
        {/* Hub topology always renders; only the model table needs data. */}
        <div className="relative w-full" style={{ height }}>
          {width > 0 && (
            <>
              <svg className="absolute inset-0" width={width} height={height} aria-hidden>
                {surfacePos.map(({ s, x, y }) => {
                  const x0 = x + NODE_W / 2;
                  const y0 = y + ROW_H / 2;
                  const x1 = cx;
                  const y1 = height / 2;
                  const mid = (x0 + x1) / 2;
                  return (
                    <path
                      key={`edge-${s.key}`}
                      d={`M ${x0} ${y0} C ${mid} ${y0}, ${mid} ${y1}, ${x1} ${y1}`}
                      fill="none"
                      stroke="currentColor"
                      className={EDGE_STROKE[s.status]}
                      strokeWidth={1.5}
                    />
                  );
                })}
              </svg>

              {surfacePos.map(({ s, x, y }) => {
                const Icon = SURFACE_ICONS[s.key];
                const sub =
                  s.status === "active" && s.calls24h > 0
                    ? req(s.calls24h)
                    : s.status === "offline"
                      ? offlineLabel
                      : idleLabel;
                return (
                  <div
                    key={s.key}
                    className={`absolute flex items-center gap-2 rounded-lg border-2 bg-card px-2.5 shadow-sm ${NODE_BORDER[s.status]}`}
                    style={{ left: x, top: y, width: NODE_W, height: ROW_H }}
                  >
                    <span
                      className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md ${avatarTint(s.key)}`}
                    >
                      <Icon className="h-4 w-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-xs font-medium" title={s.label}>
                        {s.label}
                      </p>
                      <p className="text-[10px] text-muted-foreground tabular-nums">{sub}</p>
                    </div>
                    {s.status === "active" && (
                      <span className="relative flex h-2 w-2 shrink-0">
                        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75 motion-reduce:hidden" />
                        <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
                      </span>
                    )}
                  </div>
                );
              })}

              {/* GoClaw hub: claw logo + wordmark (brand lockup) */}
              <div
                className="absolute flex flex-col items-center justify-center rounded-xl border-2 border-primary/30 bg-primary/10 px-2 shadow-sm"
                style={{ left: cx - CENTER_W / 2, top: height / 2 - ROW_H / 2, width: CENTER_W, height: ROW_H }}
              >
                <img src="/goclaw-icon.svg" alt="" className="h-5 w-5" />
                <p className="text-xs font-bold leading-tight text-primary">GoClaw</p>
                <p className="text-[10px] text-muted-foreground tabular-nums">
                  {req(totalCalls)}
                </p>
              </div>
            </>
          )}
        </div>
        {/* Usage by model (9router UsageTable style, compact) */}
        <div className="mt-4 border-t pt-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            {byModel}
          </p>
          {edges.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">{noData}</p>
          ) : (
            <>
              <div className="overflow-x-auto">
                <table className="w-full min-w-[480px] text-sm">
                  <thead>
                    <tr className="border-b text-left text-xs text-muted-foreground">
                      <th className="pb-2 font-medium">{colModel}</th>
                      <th className="px-3 pb-2 text-right font-medium">{colRequests}</th>
                      <th className="px-3 pb-2 text-right font-medium">{colInOut}</th>
                      <th className="w-28 pb-2 pl-3 font-medium">{colShare}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {models.map((m) => {
                      const share = totalCalls > 0 ? (m.calls / totalCalls) * 100 : 0;
                      const io = inOutByModel.get(m.name) ?? { inTok: 0, outTok: 0 };
                      return (
                        <tr key={m.name} className="border-b last:border-0">
                          <td className="py-2 pr-3">
                            <div className="flex min-w-0 items-center gap-2">
                              <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor(m.name)}`} />
                              <p className="truncate font-mono text-xs font-medium" title={m.name}>
                                {m.name}
                              </p>
                            </div>
                          </td>
                          <td className="px-3 py-2 text-right tabular-nums">{compactCount(m.calls)}</td>
                          <td className="whitespace-nowrap px-3 py-2 text-right tabular-nums">
                            <span className="text-rose-500 dark:text-rose-400">{formatTokens(io.inTok)}</span>
                            <span className="mx-1 text-muted-foreground/60">/</span>
                            <span className="text-emerald-600 dark:text-emerald-400">{formatTokens(io.outTok)}</span>
                          </td>
                          <td className="py-2 pl-3">
                            <div className="flex items-center gap-2">
                              <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                                <div
                                  className="h-full rounded-full bg-primary"
                                  style={{ width: `${Math.min(share, 100)}%` }}
                                />
                              </div>
                              <span className="text-xs tabular-nums text-muted-foreground">
                                {share.toFixed(1)}%
                              </span>
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {moreLabel && <p className="mt-2 text-[11px] text-muted-foreground">{moreLabel}</p>}
              <p className="mt-2 text-[11px] text-muted-foreground">
                {req(totalCalls)} · {windowLabel}
              </p>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

function GraphHeader({ title, window: windowLabel }: { title: string; window: string }) {
  return (
    <CardHeader className="flex flex-row items-center justify-between pb-3">
      <CardTitle className="text-base">{title}</CardTitle>
      <Badge variant="outline" className="text-muted-foreground">{windowLabel}</Badge>
    </CardHeader>
  );
}
