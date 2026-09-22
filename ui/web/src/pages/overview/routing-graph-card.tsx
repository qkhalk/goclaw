import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Bot } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { formatTokens } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";

/** One provider→model pair from GET /v1/usage/routing. */
interface RoutingEdge {
  provider: string;
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  errors: number;
}

interface NodeAgg {
  name: string;
  calls: number;
  tokens: number;
}

const MAX_NODES = 5;
const ROW_H = 46;
const ROW_GAP = 10;
const NODE_W = 168;
const CENTER_W = 116;
const GRAPH_MIN_WIDTH = 620;
const REFRESH_INTERVAL = 30_000;

const DOT_COLORS = [
  "bg-blue-500",
  "bg-violet-500",
  "bg-emerald-500",
  "bg-amber-500",
  "bg-rose-500",
  "bg-cyan-500",
];

function compactCount(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
}

function dotColor(name: string): string {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) | 0;
  return DOT_COLORS[Math.abs(h) % DOT_COLORS.length] ?? DOT_COLORS[0]!;
}

function aggregate(edges: RoutingEdge[], key: "provider" | "model"): NodeAgg[] {
  const map = new Map<string, NodeAgg>();
  for (const e of edges) {
    const name = e[key];
    const agg = map.get(name) ?? { name, calls: 0, tokens: 0 };
    agg.calls += e.calls;
    agg.tokens += e.input_tokens + e.output_tokens;
    map.set(name, agg);
  }
  return [...map.values()].sort((a, b) => b.calls - a.calls).slice(0, MAX_NODES);
}

/** Node pill shared by both graph columns. */
function NodePill({
  name,
  agg,
  mono,
  width,
  top,
  left,
  req,
}: {
  name: string;
  agg: NodeAgg;
  mono?: boolean;
  width: number;
  top: number;
  left: number;
  req: (n: number) => string;
}) {
  return (
    <div
      className="absolute flex flex-col justify-center rounded-xl border bg-card px-3 shadow-sm"
      style={{ left, top, width, height: ROW_H }}
    >
      <div className="flex min-w-0 items-center gap-1.5">
        {!mono && <span className={`h-2 w-2 shrink-0 rounded-full ${dotColor(name)}`} />}
        <p
          className={`truncate text-xs font-medium ${mono ? "font-mono" : ""}`}
          title={name}
        >
          {name}
        </p>
      </div>
      <p className="truncate text-[10px] text-muted-foreground tabular-nums">
        {req(agg.calls)} · {formatTokens(agg.tokens)} tok
      </p>
    </div>
  );
}

/** 9router-style routing graph: providers → GoClaw → models with request
 * counts on the edges (24h llm_call aggregation). Falls back to a compact
 * edge list on narrow screens. */
export function RoutingGraphCard() {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const containerRef = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);

  const { data } = useQuery({
    queryKey: ["usage", "routing"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => http.get<{ window_hours: number; edges: RoutingEdge[] }>("/v1/usage/routing", { hours: "24", limit: "24" }),
  });
  const edges = data?.edges ?? [];

  const providers = useMemo(() => aggregate(edges, "provider"), [edges]);
  const models = useMemo(() => aggregate(edges, "model"), [edges]);

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
  const providersFolded = new Set(edges.map((e) => e.provider)).size - providers.length;
  const modelsFolded = new Set(edges.map((e) => e.model)).size - models.length;
  const moreLabel =
    providersFolded > 0 || modelsFolded > 0
      ? [
          providersFolded > 0 ? t("routing.moreProviders", { count: providersFolded }) : "",
          modelsFolded > 0 ? t("routing.moreModels", { count: modelsFolded }) : "",
        ]
            .filter(Boolean)
            .join(" · ")
      : "";

  // Narrow screens: compact edge list instead of the graph.
  if (width > 0 && width < GRAPH_MIN_WIDTH) {
    return (
      <Card>
        <GraphHeader title={t("routing.title")} window={t("routing.window")} />
        <CardContent>
          {edges.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">{t("routing.noData")}</p>
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

  const rows = Math.max(providers.length, models.length, 1);
  const height = rows * ROW_H + (rows - 1) * ROW_GAP;
  const colY = (count: number, i: number) => {
    const colH = count * ROW_H + (count - 1) * ROW_GAP;
    return (height - colH) / 2 + i * (ROW_H + ROW_GAP);
  };
  const gap = Math.max((width - NODE_W * 2 - CENTER_W) / 2, 24);
  const xProviderRight = NODE_W;
  const xCenterLeft = NODE_W + gap;
  const xCenterRight = xCenterLeft + CENTER_W;
  const xModelLeft = xCenterRight + gap;

  const curve = (x0: number, y0: number, x1: number, y1: number) => {
    const mid = (x0 + x1) / 2;
    return `M ${x0} ${y0} C ${mid} ${y0}, ${mid} ${y1}, ${x1} ${y1}`;
  };
  const bezierMidY = (y0: number, y1: number) => (y0 + y1) / 2;

  const providerCallsByName = new Map(providers.map((p) => [p.name, p.calls]));
  const modelCallsByName = new Map(models.map((m) => [m.name, m.calls]));
  const centerCalls = providers.reduce((s, p) => s + p.calls, 0);

  return (
    <Card>
      <GraphHeader title={t("routing.title")} window={t("routing.window")} />
      <CardContent>
        {edges.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t("routing.noData")}</p>
        ) : (
          <>
            <div ref={containerRef} className="relative w-full" style={{ height }}>
              {width > 0 && (
                <>
                  <svg className="absolute inset-0" width={width} height={height} aria-hidden>
                    {providers.map((p, i) => {
                      const y0 = colY(providers.length, i) + ROW_H / 2;
                      const y1 = height / 2;
                      const mx = (xProviderRight + xCenterLeft) / 2;
                      const my = bezierMidY(y0, y1);
                      return (
                        <g key={`pe-${p.name}`}>
                          <path
                            d={curve(xProviderRight, y0, xCenterLeft, y1)}
                            fill="none"
                            stroke="currentColor"
                            className="text-muted-foreground/35"
                            strokeWidth={1.5}
                          />
                          <text
                            x={mx}
                            y={my - 5}
                            textAnchor="middle"
                            className="fill-muted-foreground text-[10px] font-medium tabular-nums"
                            style={{ paintOrder: "stroke", stroke: "var(--card)", strokeWidth: 3 }}
                          >
                            {req(providerCallsByName.get(p.name) ?? 0)}
                          </text>
                        </g>
                      );
                    })}
                    {models.map((m, i) => {
                      const y0 = height / 2;
                      const y1 = colY(models.length, i) + ROW_H / 2;
                      const mx = (xCenterRight + xModelLeft) / 2;
                      const my = bezierMidY(y0, y1);
                      return (
                        <g key={`me-${m.name}`}>
                          <path
                            d={curve(xCenterRight, y0, xModelLeft, y1)}
                            fill="none"
                            stroke="currentColor"
                            className="text-muted-foreground/35"
                            strokeWidth={1.5}
                          />
                          <text
                            x={mx}
                            y={my - 5}
                            textAnchor="middle"
                            className="fill-muted-foreground text-[10px] font-medium tabular-nums"
                            style={{ paintOrder: "stroke", stroke: "var(--card)", strokeWidth: 3 }}
                          >
                            {req(modelCallsByName.get(m.name) ?? 0)}
                          </text>
                        </g>
                      );
                    })}
                  </svg>

                  {providers.map((p, i) => (
                    <NodePill
                      key={p.name}
                      name={p.name}
                      agg={p}
                      width={NODE_W}
                      left={0}
                      top={colY(providers.length, i)}
                      req={req}
                    />
                  ))}

                  <div
                    className="absolute flex flex-col items-center justify-center rounded-xl border-2 border-primary/30 bg-primary/10 px-2 shadow-sm"
                    style={{ left: xCenterLeft, top: height / 2 - ROW_H / 2, width: CENTER_W, height: ROW_H }}
                  >
                    <Bot className="h-4 w-4 text-primary" />
                    <p className="text-xs font-semibold leading-tight">GoClaw</p>
                    <p className="text-[10px] text-muted-foreground tabular-nums">
                      {req(centerCalls)}
                    </p>
                  </div>

                  {models.map((m, i) => (
                    <NodePill
                      key={m.name}
                      name={m.name}
                      agg={m}
                      mono
                      width={NODE_W}
                      left={xModelLeft}
                      top={colY(models.length, i)}
                      req={req}
                    />
                  ))}
                </>
              )}
            </div>
            {moreLabel && <p className="mt-2 text-[11px] text-muted-foreground">{moreLabel}</p>}
            <p className="mt-1 text-[11px] text-muted-foreground">
              {req(totalCalls)} · {t("routing.window")}
            </p>
          </>
        )}
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
