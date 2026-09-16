import { useCallback, useEffect, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  BaseEdge,
  Controls,
  Handle,
  Position,
  ReactFlow,
  getBezierPath,
  type Edge,
  type EdgeProps,
  type Node,
  type NodeProps,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Clapperboard, MessagesSquare, Presentation, Send } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useHttp } from "@/hooks/use-ws";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import type { ChannelStatusEntry } from "./types";

/** Channel names in usage_snapshots — Telegram uses its config name (e.g.
 *  "telegram-main"), web chat uses "ws".  We match case-insensitively. */
const TELEGRAM_CHANNEL_RE = /telegram/i;
const WEB_CHANNEL = "ws";

type SurfaceKey = "telegram" | "web" | "pptx" | "video";
type SurfaceStatus = "active" | "idle" | "offline";

interface SurfaceData extends Record<string, unknown> {
  key: SurfaceKey;
  label: string;
  status: SurfaceStatus;
  count: number;
}
type SurfaceFlowNode = Node<SurfaceData, "surface">;

interface HubData extends Record<string, unknown> {
  activeCount: number;
}
type HubFlowNode = Node<HubData, "hub">;
type TopologyNode = SurfaceFlowNode | HubFlowNode;

interface TopologyEdgeData extends Record<string, unknown> {
  active: boolean;
}
type TopologyFlowEdge = Edge<TopologyEdgeData, "topology">;

const SURFACE_ICONS = {
  telegram: Send,
  web: MessagesSquare,
  pptx: Presentation,
  video: Clapperboard,
} as const;

/** Per-surface accent: node border + icon tile + count badge when active. */
const SURFACE_COLOR: Record<SurfaceKey, string> = {
  telegram: "#3b82f6",
  web: "#8b5cf6",
  pptx: "#f59e0b",
  video: "#f43f5e",
};

const REFRESH_INTERVAL = 15_000;
const ACTIVE_WINDOW_MS = 5 * 60 * 1000;

const POSITIONS = [Position.Top, Position.Right, Position.Bottom, Position.Left];

/** Electric beam decoration (9router ProviderTopology): 4 handles per node so
 *  each edge can leave from the side facing the hub. */
function AllHandles({ type, prefix }: { type: "source" | "target"; prefix: string }) {
  return (
    <>
      {POSITIONS.map((p) => (
        <Handle
          key={p}
          id={`${prefix}-${p}`}
          type={type}
          position={p}
          isConnectable={false}
          className="!h-1 !w-1 !border-0 !bg-transparent"
        />
      ))}
    </>
  );
}

function SurfaceNode({ data }: NodeProps<SurfaceFlowNode>) {
  const Icon = SURFACE_ICONS[data.key];
  const active = data.status === "active";
  const offline = data.status === "offline";
  const color = SURFACE_COLOR[data.key];
  return (
    <div
      className={`relative flex items-center gap-2.5 rounded-lg border-2 bg-card px-4 py-2.5 transition-all duration-300 ${
        active ? "" : "border-border"
      } ${offline ? "opacity-55" : ""}`}
      style={{
        minWidth: 160,
        borderColor: active ? color : undefined,
        boxShadow: active ? `0 0 16px ${color}40` : "none",
      }}
    >
      <AllHandles type="source" prefix="s" />
      <span
        className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md"
        style={{ backgroundColor: `${color}15` }}
      >
        <Icon className="h-[18px] w-[18px]" style={{ color: active ? color : undefined }} />
      </span>
      <span className="truncate text-sm font-medium" style={{ color: active ? color : undefined }}>
        {data.label}
      </span>
      {active && (
        <>
          <span className="relative -mr-1 ml-1 flex h-2 w-2 shrink-0">
            <span
              className="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75 motion-reduce:hidden"
              style={{ backgroundColor: color }}
            />
            <span className="relative inline-flex h-2 w-2 rounded-full" style={{ backgroundColor: color }} />
          </span>
          {data.count > 0 && (
            <span
              className="ml-0.5 rounded-full px-1.5 py-0.5 text-[10px] font-bold leading-none text-white"
              style={{ backgroundColor: color }}
            >
              {data.count}
            </span>
          )}
        </>
      )}
    </div>
  );
}

/** GoClaw hub — pulse/glow core while any surface has traffic, mirroring the
 *  9router router node. */
function HubNode({ data }: NodeProps<HubFlowNode>) {
  const powering = (data.activeCount ?? 0) > 0;
  return (
    <div
      className={`relative z-[1] flex items-center gap-2 rounded-xl border-2 px-5 py-3 ${
        powering
          ? "topology-router-core border-yellow-300 bg-gradient-to-br from-primary/30 via-yellow-400/20 to-cyan-400/25"
          : "border-primary/40 bg-primary/10 shadow-sm"
      }`}
    >
      <AllHandles type="target" prefix="t" />
      <img src="/goclaw-icon.svg" alt="" className={`h-7 w-7 ${powering ? "topology-router-icon" : ""}`} />
      <span className={`text-base font-bold ${powering ? "topology-router-label text-yellow-300" : "text-primary"}`}>
        GoClaw
      </span>
      {powering && (
        <span className="topology-router-badge ml-1 rounded-full bg-yellow-400 px-1.5 py-0.5 text-xs font-bold leading-none text-black">
          {data.activeCount}
        </span>
      )}
    </div>
  );
}

/** Active edge: electric kame beam (halo + plasma + white core + particles).
 *  Idle edge: plain faint line. Path runs surface -> hub, so dashes and
 *  particles flow INTO the GoClaw border. */
const BEAM_PARTICLES = 6;
const BEAM_SPARKS = 5;

function TopologyEdgeView({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  data,
}: EdgeProps<TopologyFlowEdge>) {
  const [edgePath] = getBezierPath({ sourceX, sourceY, sourcePosition, targetX, targetY, targetPosition });
  const active = !!data?.active;
  const stroke = style.stroke ?? "#94a3b8";
  const filterId = `topo-beam-${id}`;

  if (!active) {
    return <BaseEdge id={id} path={edgePath} style={{ ...style, stroke }} />;
  }

  return (
    <g className="topology-edge-electric">
      <defs>
        <filter id={filterId} x="-40%" y="-40%" width="180%" height="180%">
          <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" seed="2" result="noise">
            <animate attributeName="baseFrequency" values="0.8;1.4;0.8" dur="0.25s" repeatCount="indefinite" />
          </feTurbulence>
          <feDisplacementMap in="SourceGraphic" in2="noise" scale="3.5" xChannelSelector="R" yChannelSelector="G" />
        </filter>
      </defs>
      {/* Outer electric halo */}
      <path
        d={edgePath}
        fill="none"
        stroke="#22d3ee"
        strokeWidth={10}
        strokeOpacity={0.35}
        strokeLinecap="round"
        filter={`url(#${filterId})`}
        className="topology-edge-halo"
      />
      {/* Mid plasma */}
      <path
        d={edgePath}
        fill="none"
        stroke="#4ade80"
        strokeWidth={5}
        strokeOpacity={0.85}
        strokeLinecap="round"
        filter={`url(#${filterId})`}
        className="topology-edge-plasma"
      />
      {/* Hot white core — dashed, flows into the hub border */}
      <BaseEdge
        id={id}
        path={edgePath}
        style={{ stroke: "#f8fafc", strokeWidth: 2.2, opacity: 1 }}
        className="topology-edge-kame"
      />
      {/* Energy orbs riding the path */}
      {Array.from({ length: BEAM_PARTICLES }, (_, i) => (
        <circle
          key={`p-${i}`}
          r={i % 2 === 0 ? 4 : 2.5}
          fill={i % 3 === 0 ? "#fde047" : i % 3 === 1 ? "#67e8f9" : "#fff"}
          opacity={0.95}
          style={{ filter: "drop-shadow(0 0 4px #22d3ee)" }}
        >
          <animateMotion dur={`${0.4 + i * 0.08}s`} repeatCount="indefinite" path={edgePath} begin={`${i * 0.09}s`} />
        </circle>
      ))}
      {/* Short-lived sparks along the path */}
      {Array.from({ length: BEAM_SPARKS }, (_, i) => (
        <circle key={`s-${i}`} r={1.8} fill="#e0f2fe" opacity={0}>
          <animate
            attributeName="opacity"
            values="0;1;0;0;1;0"
            dur={`${0.35 + (i % 3) * 0.1}s`}
            begin={`${i * 0.07}s`}
            repeatCount="indefinite"
          />
          <animateMotion dur={`${0.28 + i * 0.05}s`} repeatCount="indefinite" path={edgePath} begin={`${i * 0.11}s`} />
        </circle>
      ))}
    </g>
  );
}

const nodeTypes = { surface: SurfaceNode, hub: HubNode };
const edgeTypes = { topology: TopologyEdgeView };

/** 9router ProviderTopology layout: surfaces evenly spaced on an ellipse
 *  around the hub (rx grows with node count so nodes never crowd). The edge
 *  leaves each surface from the side facing the hub. */
function buildLayout(surfaces: SurfaceData[]): {
  nodes: (SurfaceFlowNode | HubFlowNode)[];
  edges: TopologyFlowEdge[];
} {
  const nodeW = 180;
  const nodeH = 56;
  const hubW = 150;
  const hubH = 56;

  const count = Math.max(surfaces.length, 1);
  const rx = Math.max(320, ((nodeW + 24) * count) / (2 * Math.PI));
  const ry = Math.max(200, rx * 0.55);

  const nodes: (SurfaceFlowNode | HubFlowNode)[] = [
    {
      id: "goclaw",
      type: "hub",
      position: { x: -hubW / 2, y: -hubH / 2 },
      data: { activeCount: surfaces.filter((s) => s.status === "active").length },
      draggable: false,
    },
  ];
  const edges: TopologyFlowEdge[] = [];

  surfaces.forEach((s, i) => {
    const angle = -Math.PI / 2 + (2 * Math.PI * i) / count;
    const px = rx * Math.cos(angle);
    const py = ry * Math.sin(angle);

    let surfaceHandle: Position;
    let hubHandle: Position;
    if (py < -ry / Math.SQRT2) {
      surfaceHandle = Position.Bottom;
      hubHandle = Position.Top;
    } else if (py > ry / Math.SQRT2) {
      surfaceHandle = Position.Top;
      hubHandle = Position.Bottom;
    } else if (px > 0) {
      surfaceHandle = Position.Left;
      hubHandle = Position.Right;
    } else {
      surfaceHandle = Position.Right;
      hubHandle = Position.Left;
    }

    nodes.push({
      id: `surface-${s.key}`,
      type: "surface",
      position: { x: px - nodeW / 2, y: py - nodeH / 2 },
      data: s,
      draggable: false,
    });
    edges.push({
      id: `edge-${s.key}`,
      type: "topology",
      source: `surface-${s.key}`,
      sourceHandle: `s-${surfaceHandle}`,
      target: "goclaw",
      targetHandle: `t-${hubHandle}`,
      // The beam animates via SVG particles; ReactFlow's dash animation is CPU-heavy.
      animated: false,
      data: { active: s.status === "active" },
      style:
        s.status === "active"
          ? { stroke: "#22d3ee", strokeWidth: 3.5, opacity: 1 }
          : { stroke: "#94a3b8", strokeWidth: 1, opacity: 0.3 },
    });
  });

  return { nodes, edges };
}

/** 9router-style surface topology: Telegram / web chat / PPTX / video editor
 *  orbit the GoClaw hub on a pannable, zoomable canvas. A surface lights up
 *  (electric beam + glow) only while it has real requests in the last 5
 *  minutes — running services and idle WS connections stay gray. */
export function RoutingGraphCard({
  channelEntries = [],
}: {
  channelEntries?: [string, ChannelStatusEntry][];
}) {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const { agents } = useAgents();

  // ── Channel breakdown: real usage per surface in the active window ──
  const { data: channelBreakdown } = useQuery({
    queryKey: ["usage", "breakdown", "channel", "5m", "routing-topology"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - ACTIVE_WINDOW_MS);
      return http.get<{ rows: { key: string; request_count: number }[] }>(
        "/v1/usage/breakdown",
        { group_by: "channel", from: from.toISOString(), to: to.toISOString() },
      );
    },
  });

  const channelReqs = useMemo(() => {
    let telegram = 0;
    let web = 0;
    for (const row of channelBreakdown?.rows ?? []) {
      if (TELEGRAM_CHANNEL_RE.test(row.key)) telegram += row.request_count;
      else if (row.key === WEB_CHANNEL) web += row.request_count;
    }
    return { telegram, web };
  }, [channelBreakdown]);

  // ── Agent breakdown: PPTX + video agent usage ──
  const { data: agentBreakdown } = useQuery({
    queryKey: ["usage", "breakdown", "agent", "5m", "routing-active"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - ACTIVE_WINDOW_MS);
      return http.get<{ rows: { key: string; request_count: number }[] }>(
        "/v1/usage/breakdown",
        { group_by: "agent", from: from.toISOString(), to: to.toISOString() },
      );
    },
  });

  const agentCalls = useMemo(() => {
    const idToKey = new Map((agents ?? []).map((a) => [a.id, a.agent_key] as const));
    const byKey = new Map<string, number>();
    for (const row of agentBreakdown?.rows ?? []) {
      const key = idToKey.get(row.key) ?? row.key;
      byKey.set(key, (byKey.get(key) ?? 0) + row.request_count);
    }
    return byKey;
  }, [agents, agentBreakdown]);

  // ── Video jobs: live editor signal ──
  const { data: videoJobs } = useQuery({
    queryKey: ["video", "jobs", "routing-active"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () =>
      http.get<{ jobs: { status: string; updated_at: string }[] }>("/v1/video/jobs", { limit: "10" }),
  });

  const surfaces = useMemo<SurfaceData[]>(() => {
    const now = Date.now();
    const videoBusy = (videoJobs?.jobs ?? []).some((j) => {
      if (j.status === "queued" || j.status === "rendering") return true;
      const updated = new Date(j.updated_at).getTime();
      return Number.isFinite(updated) && now - updated < ACTIVE_WINDOW_MS;
    });

    // Telegram: offline if configured but not running.
    const telegramEntry = channelEntries.find(([n]) => TELEGRAM_CHANNEL_RE.test(n));
    const telegramRunning = telegramEntry?.[1]?.running ?? false;
    const telegramOffline = telegramEntry !== undefined && !telegramRunning;

    const surface = (key: SurfaceKey, count: number, offline = false): SurfaceData => ({
      key,
      label: t(`routing.surfaces.${key}`),
      status: offline ? "offline" : count > 0 ? "active" : "idle",
      count,
    });
    return [
      surface("telegram", channelReqs.telegram, telegramOffline),
      surface("web", channelReqs.web),
      surface("pptx", agentCalls.get("pptx-designer") ?? 0),
      surface("video", Math.max(videoBusy ? 1 : 0, agentCalls.get("video-designer") ?? 0)),
    ];
  }, [t, channelEntries, channelReqs, agentCalls, videoJobs]);

  const { nodes, edges } = useMemo(() => buildLayout(surfaces), [surfaces]);

  // Keep the whole topology in view on mount, container resize, and when the
  // surface count changes (9router ProviderTopology behavior).
  const rfRef = useRef<ReactFlowInstance<TopologyNode, TopologyFlowEdge> | null>(null);
  const containerRef = useRef<HTMLDivElement | null>(null);
  const fitOpts = useMemo(() => ({ padding: 0.2, duration: 200 }), []);
  const onInit = useCallback(
    (instance: ReactFlowInstance<TopologyNode, TopologyFlowEdge>) => {
      rfRef.current = instance;
      // fitView no-ops if the container hasn't been measured yet (e.g. the
      // card mounts while the layout is still settling), leaving the viewport
      // at identity and every node off-screen. Retry a few times — once the
      // container has real dimensions, fitView centers the ellipse.
      const timers = [50, 400, 1200].map((delay) =>
        setTimeout(() => instance.fitView(fitOpts), delay),
      );
      return () => timers.forEach(clearTimeout);
    },
    [fitOpts],
  );
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => rfRef.current?.fitView(fitOpts));
    ro.observe(el);
    return () => ro.disconnect();
  }, [fitOpts]);
  useEffect(() => {
    const id = setTimeout(() => rfRef.current?.fitView(fitOpts), 50);
    return () => clearTimeout(id);
  }, [nodes.length, fitOpts]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">{t("routing.title")}</CardTitle>
        <Badge variant="outline" className="text-muted-foreground">{t("routing.window")}</Badge>
      </CardHeader>
      <CardContent>
        <div ref={containerRef} className="h-[320px] w-full min-w-0 rounded-lg border bg-muted/30 sm:h-[480px]">
          <ReactFlow<TopologyNode, TopologyFlowEdge>
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            edgeTypes={edgeTypes}
            fitView
            fitViewOptions={fitOpts}
            minZoom={0.1}
            maxZoom={2}
            onInit={onInit}
            proOptions={{ hideAttribution: true }}
            panOnDrag
            zoomOnScroll
            zoomOnPinch
            zoomOnDoubleClick
            preventScrolling={false}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
          >
            <Controls showInteractive={false} className="[&>button]:bg-card [&>button]:border-border [&>button:hover]:bg-accent" />
          </ReactFlow>
        </div>
      </CardContent>
    </Card>
  );
}
