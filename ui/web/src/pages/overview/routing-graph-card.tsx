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

const MAX_NODES = 6;
const ROW_H = 52;
const ROW_GAP = 12;
const COL_LEFT = 118;
const COL_CENTER = 92;
const COL_RIGHT = 118;
const REFRESH_INTERVAL = 30_000;

function compactCount(n: number): string {
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`;
  return String(n);
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

/** 9router-style routing graph: providers → GoClaw → models with request
 * counts on the edges (24h llm_call aggregation). */
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

  const rows = Math.max(providers.length, models.length, 1);
  const height = rows * ROW_H + (rows - 1) * ROW_GAP;
  const colY = (count: number, i: number) => {
    const colH = count * ROW_H + (count - 1) * ROW_GAP;
    return (height - colH) / 2 + ROW_H / 2 + i * (ROW_H + ROW_GAP);
  };
  const gap = Math.max((width - COL_LEFT - COL_CENTER - COL_RIGHT) / 2, 10);
  const xProviderRight = COL_LEFT;
  const xCenterLeft = COL_LEFT + gap;
  const xCenterRight = xCenterLeft + COL_CENTER;
  const xModelLeft = xCenterRight + gap;

  const curve = (x0: number, y0: number, x1: number, y1: number) => {
    const mid = (x0 + x1) / 2;
    return `M ${x0} ${y0} C ${mid} ${y0}, ${mid} ${y1}, ${x1} ${y1}`;
  };
  const bezierMid = (x0: number, y0: number, x1: number, y1: number) => {
    const mid = (x0 + x1) / 2;
    return { x: (x0 + 3 * mid + 3 * mid + x1) / 8, y: (y0 + 3 * y0 + 3 * y1 + y1) / 8 };
  };

  const centerCalls = providers.reduce((s, p) => s + p.calls, 0);
  const modelCallsByName = new Map(models.map((m) => [m.name, m.calls]));
  const providerCallsByName = new Map(providers.map((p) => [p.name, p.calls]));
  const providersFolded = new Set(edges.map((e) => e.provider)).size - providers.length;
  const modelsFolded = new Set(edges.map((e) => e.model)).size - models.length;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">{t("routing.title")}</CardTitle>
        <Badge variant="outline" className="text-muted-foreground">{t("routing.window")}</Badge>
      </CardHeader>
      <CardContent>
        {edges.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">{t("routing.noData")}</p>
        ) : (
          <div ref={containerRef} className="relative w-full" style={{ height }}>
            {width > 0 && (
              <>
                <svg className="absolute inset-0" width={width} height={height} aria-hidden>
                  {providers.map((p, i) => {
                    const y0 = colY(providers.length, i);
                    const y1 = height / 2;
                    const mid = bezierMid(xProviderRight, y0, xCenterLeft, y1);
                    return (
                      <g key={`pe-${p.name}`}>
                        <path d={curve(xProviderRight, y0, xCenterLeft, y1)} fill="none" stroke="currentColor" className="text-primary/35" strokeWidth={1.5} />
                        <text x={mid.x} y={mid.y - 4} textAnchor="middle" className="fill-muted-foreground text-[9px] tabular-nums">
                          {compactCount(providerCallsByName.get(p.name) ?? 0)}
                        </text>
                      </g>
                    );
                  })}
                  {models.map((m, i) => {
                    const y0 = height / 2;
                    const y1 = colY(models.length, i);
                    const mid = bezierMid(xCenterRight, y0, xModelLeft, y1);
                    return (
                      <g key={`me-${m.name}`}>
                        <path d={curve(xCenterRight, y0, xModelLeft, y1)} fill="none" stroke="currentColor" className="text-primary/35" strokeWidth={1.5} />
                        <text x={mid.x} y={mid.y - 4} textAnchor="middle" className="fill-muted-foreground text-[9px] tabular-nums">
                          {compactCount(modelCallsByName.get(m.name) ?? 0)}
                        </text>
                      </g>
                    );
                  })}
                </svg>

                {providers.map((p, i) => (
                  <div
                    key={p.name}
                    className="absolute flex flex-col justify-center rounded-md border bg-muted/30 px-2"
                    style={{ left: 0, top: colY(providers.length, i) - ROW_H / 2, width: COL_LEFT, height: ROW_H }}
                  >
                    <p className="truncate text-xs font-medium" title={p.name}>{p.name}</p>
                    <p className="truncate text-[10px] text-muted-foreground tabular-nums">
                      {t("routing.req", { count: p.calls })} · {formatTokens(p.tokens)}
                    </p>
                  </div>
                ))}

                <div
                  className="absolute flex flex-col items-center justify-center rounded-md border border-primary/40 bg-primary/5 px-2"
                  style={{ left: xCenterLeft, top: height / 2 - ROW_H / 2, width: COL_CENTER, height: ROW_H }}
                >
                  <Bot className="h-4 w-4 text-primary" />
                  <p className="text-xs font-semibold">GoClaw</p>
                  <p className="text-[10px] text-muted-foreground tabular-nums">
                    {compactCount(centerCalls)} · {t("routing.req", { count: centerCalls })}
                  </p>
                </div>

                {models.map((m, i) => (
                  <div
                    key={m.name}
                    className="absolute flex flex-col justify-center rounded-md border bg-muted/30 px-2"
                    style={{ left: xModelLeft, top: colY(models.length, i) - ROW_H / 2, width: COL_RIGHT, height: ROW_H }}
                  >
                    <p className="truncate font-mono text-[11px] font-medium" title={m.name}>{m.name}</p>
                    <p className="truncate text-[10px] text-muted-foreground tabular-nums">
                      {t("routing.req", { count: m.calls })} · {formatTokens(m.tokens)}
                    </p>
                  </div>
                ))}
              </>
            )}
          </div>
        )}
        {(providersFolded > 0 || modelsFolded > 0) && (
          <p className="mt-2 text-[11px] text-muted-foreground">
            {[
              providersFolded > 0 ? t("routing.moreProviders", { count: providersFolded }) : "",
              modelsFolded > 0 ? t("routing.moreModels", { count: modelsFolded }) : "",
            ].filter(Boolean).join(" · ")}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
