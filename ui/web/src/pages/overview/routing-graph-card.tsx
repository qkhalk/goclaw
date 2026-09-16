import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Background,
  BackgroundVariant,
  Controls,
  Handle,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Clapperboard, MessagesSquare, Presentation, Send } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { useHttp } from "@/hooks/use-ws";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import type { ChannelStatusEntry } from "./types";

type SurfaceKey = "telegram" | "web" | "pptx" | "video";
type SurfaceStatus = "active" | "idle" | "offline";

interface SurfaceData extends Record<string, unknown> {
  key: SurfaceKey;
  label: string;
  status: SurfaceStatus;
}
type SurfaceFlowNode = Node<SurfaceData, "surface">;
type HubFlowNode = Node<Record<string, never>, "hub">;

const SURFACE_ICONS = {
  telegram: Send,
  web: MessagesSquare,
  pptx: Presentation,
  video: Clapperboard,
} as const;

const REFRESH_INTERVAL = 30_000;
const HOUR_MS = 60 * 60 * 1000;

/** Status -> edge/node accent, mirroring 9router ProviderTopology colors. */
const EDGE_COLOR: Record<SurfaceStatus, { stroke: string; opacity: number }> = {
  active: { stroke: "#22c55e", opacity: 0.9 },
  idle: { stroke: "#94a3b8", opacity: 0.35 },
  offline: { stroke: "#94a3b8", opacity: 0.2 },
};

const NODE_BORDER: Record<SurfaceStatus, string> = {
  active: "border-[#22c55e]",
  idle: "border-border",
  offline: "border-border opacity-60",
};

const SURFACE_TINT = {
  telegram: "bg-blue-500/15 text-blue-600 dark:text-blue-400",
  web: "bg-violet-500/15 text-violet-600 dark:text-violet-400",
  pptx: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  video: "bg-rose-500/15 text-rose-600 dark:text-rose-400",
} as const;

const POSITIONS = [Position.Bottom, Position.Left, Position.Top, Position.Right];

/** 9router ProviderTopology layout: surfaces on an ellipse around the hub.
 * rx scales with node count so nodes never crowd, and the canvas is
 * pannable/zoomable via ReactFlow. */
function buildLayout(surfaces: SurfaceData[]): {
  nodes: (SurfaceFlowNode | HubFlowNode)[];
  edges: Edge[];
} {
  const count = Math.max(surfaces.length, 1);
  const rx = Math.max(320, (150 * count) / (2 * Math.PI));
  const ry = Math.max(200, rx * 0.55);
  const posIndex = (i: number) => ((i % 4) + 4) % 4;

  const nodes: (SurfaceFlowNode | HubFlowNode)[] = [
    {
      id: "goclaw",
      type: "hub",
      position: { x: -60, y: -29 },
      data: {},
      draggable: false,
    },
  ];
  const edges: Edge[] = [];

  surfaces.forEach((s, i) => {
    const angle = -Math.PI / 2 + (2 * Math.PI * i) / count;
    const px = rx * Math.cos(angle);
    const py = ry * Math.sin(angle);
    const handlePos = POSITIONS[posIndex(i)];
    nodes.push({
      id: `surface-${s.key}`,
      type: "surface",
      position: { x: px - 80, y: py - 24 },
      data: s,
      draggable: false,
    });
    edges.push({
      id: `edge-${s.key}`,
      source: `surface-${s.key}`,
      sourceHandle: `s-${handlePos}`,
      target: "goclaw",
      targetHandle: `t-${handlePos}`,
      type: "default",
      animated: s.status === "active",
      style: {
        stroke: EDGE_COLOR[s.status].stroke,
        strokeOpacity: EDGE_COLOR[s.status].opacity,
        strokeWidth: 1.5,
      },
    });
  });

  return { nodes, edges };
}

function SurfaceNode({ data }: NodeProps<SurfaceFlowNode>) {
  const Icon = SURFACE_ICONS[data.key];
  const active = data.status === "active";
  return (
    <div
      className={`relative flex items-center gap-2.5 rounded-lg border-2 bg-card px-4 py-2.5 shadow-sm ${NODE_BORDER[data.status]}`}
      style={{ minWidth: 160 }}
    >
      <span
        className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md ${SURFACE_TINT[data.key]}`}
      >
        <Icon className="h-[18px] w-[18px]" />
      </span>
      <span className="truncate text-sm font-medium">{data.label}</span>
      {active && (
        <span className="relative -mr-1 ml-1 flex h-2 w-2 shrink-0">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[#22c55e] opacity-75 motion-reduce:hidden" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-[#22c55e]" />
        </span>
      )}
      {POSITIONS.map((p) => (
        <Handle
          key={p}
          id={`s-${p}`}
          type="source"
          position={p}
          isConnectable={false}
          className="!h-1 !w-1 !border-0 !bg-transparent"
        />
      ))}
    </div>
  );
}

/** GoClaw brand lockup: claw logo + wordmark, the 9router hub equivalent. */
function HubNode() {
  return (
    <div className="flex items-center gap-2 rounded-xl border-2 border-primary/40 bg-primary/10 px-5 py-3 shadow-sm">
      <img src="/goclaw-icon.svg" alt="" className="h-7 w-7" />
      <span className="text-base font-bold text-primary">GoClaw</span>
      {POSITIONS.map((p) => (
        <Handle
          key={p}
          id={`t-${p}`}
          type="target"
          position={p}
          isConnectable={false}
          className="!h-1 !w-1 !border-0 !bg-transparent"
        />
      ))}
    </div>
  );
}

const nodeTypes = { surface: SurfaceNode, hub: HubNode };

/** 9router-style surface topology: Telegram / web chat / PPTX / video editor
 * orbit the GoClaw hub on a ReactFlow canvas (pan with the mouse, zoom with
 * controls/pinch). A surface only gets its animated green edge while it is
 * actually in use (right now or within the last hour); idle edges stay gray.
 * No side model table — this card is purely the topology. */
export function RoutingGraphCard({
  channelEntries = [],
  clientCount = 0,
}: {
  channelEntries?: [string, ChannelStatusEntry][];
  clientCount?: number;
}) {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const { agents } = useAgents();

  // Per-surface activity over the last hour: agent requests grouped by agent
  // UUID (mapped to agent keys), plus live video jobs as an editor signal.
  const { data: agentBreakdown } = useQuery({
    queryKey: ["usage", "breakdown", "agent", "1h", "routing-active"],
    refetchInterval: REFRESH_INTERVAL,
    queryFn: () => {
      const to = new Date();
      const from = new Date(to.getTime() - HOUR_MS);
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

  const agentCalls1h = useMemo(() => {
    const idToKey = new Map((agents ?? []).map((a) => [a.id, a.agent_key] as const));
    const byKey = new Map<string, number>();
    for (const row of agentBreakdown?.rows ?? []) {
      const key = idToKey.get(row.key) ?? row.key;
      byKey.set(key, (byKey.get(key) ?? 0) + row.request_count);
    }
    return byKey;
  }, [agents, agentBreakdown]);

  const surfaces = useMemo<SurfaceData[]>(() => {
    const telegram = channelEntries.find(([name]) => name.toLowerCase().includes("telegram"));
    const telegramRunning = telegram?.[1]?.running ?? false;
    const now = Date.now();
    const videoBusy = (videoJobs?.jobs ?? []).some((j) => {
      if (j.status === "queued" || j.status === "rendering") return true;
      const updated = new Date(j.updated_at).getTime();
      return Number.isFinite(updated) && now - updated < HOUR_MS;
    });
    const surface = (key: SurfaceKey, active: boolean, offline = false): SurfaceData => ({
      key,
      label: t(`routing.surfaces.${key}`),
      status: offline ? "offline" : active ? "active" : "idle",
    });
    return [
      surface("telegram", telegramRunning, telegram !== undefined && !telegramRunning),
      surface("web", clientCount > 0),
      surface("pptx", (agentCalls1h.get("pptx-designer") ?? 0) > 0),
      surface("video", videoBusy || (agentCalls1h.get("video-designer") ?? 0) > 0),
    ];
  }, [t, channelEntries, clientCount, agentCalls1h, videoJobs]);

  const { nodes, edges } = useMemo(() => buildLayout(surfaces), [surfaces]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-3">
        <CardTitle className="text-base">{t("routing.title")}</CardTitle>
        <Badge variant="outline" className="text-muted-foreground">{t("routing.window")}</Badge>
      </CardHeader>
      <CardContent>
        <div className="h-[320px] sm:h-[420px]">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.2 }}
            minZoom={0.35}
            maxZoom={1.5}
            nodesDraggable={false}
            nodesConnectable={false}
            elementsSelectable={false}
            zoomOnScroll={false}
            panOnScroll={false}
            preventScrolling={false}
          >
            <Background variant={BackgroundVariant.Dots} gap={24} size={1.5} className="opacity-40" />
            <Controls showInteractive={false} className="[&>button]:bg-card [&>button]:border-border [&>button:hover]:bg-accent" />
          </ReactFlow>
        </div>
      </CardContent>
    </Card>
  );
}
