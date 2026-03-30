import { useMemo, useEffect, useCallback, useState, useTransition } from "react";
import {
  ReactFlow,
  ReactFlowProvider,
  useNodesState,
  useEdgesState,
  useReactFlow,
  Background,
  Controls,
  MiniMap,
  type Node,
  type Edge,
  Handle,
  Position,
} from "@xyflow/react";
import { forceSimulation, forceLink, forceManyBody, forceCenter, forceCollide, forceX, forceY, type SimulationNodeDatum } from "d3-force";
import "@xyflow/react/dist/style.css";
import { useTranslation } from "react-i18next";
import type { KGEntity, KGRelation } from "@/types/knowledge-graph";

// Color mapping for entity types — dark-first palette for depth contrast
// bg uses semi-transparent dark tones so nodes float on dark canvas
const TYPE_COLORS: Record<string, { bg: string; border: string; text: string; glow: string }> = {
  person:       { bg: "rgba(232,93,36,0.12)",  border: "#E85D24", text: "#f4a574", glow: "rgba(232,93,36,0.35)" },
  project:      { bg: "rgba(34,197,94,0.12)",  border: "#22c55e", text: "#86efac", glow: "rgba(34,197,94,0.35)" },
  task:         { bg: "rgba(245,158,11,0.12)", border: "#f59e0b", text: "#fcd34d", glow: "rgba(245,158,11,0.35)" },
  event:        { bg: "rgba(236,72,153,0.12)", border: "#ec4899", text: "#f9a8d4", glow: "rgba(236,72,153,0.35)" },
  concept:      { bg: "rgba(248,208,128,0.10)", border: "#F8D080", text: "#fde68a", glow: "rgba(248,208,128,0.30)" },
  location:     { bg: "rgba(20,184,166,0.12)",  border: "#14b8a6", text: "#5eead4", glow: "rgba(20,184,166,0.35)" },
  organization: { bg: "rgba(239,68,68,0.12)",  border: "#ef4444", text: "#fca5a5", glow: "rgba(239,68,68,0.35)" },
};

const DEFAULT_COLOR = { bg: "rgba(156,163,175,0.10)", border: "#9ca3af", text: "#d1d5db", glow: "rgba(156,163,175,0.25)" };

// Mass multipliers by entity type — heavier nodes anchor center, lighter ones orbit outward
const TYPE_MASS: Record<string, number> = {
  organization: 8,
  project: 6,
  person: 4,
  concept: 3,
  location: 3,
  event: 2,
  task: 1.5,
};
const DEFAULT_MASS = 2;

/** Compute degree (connection count) for each entity to size nodes by centrality */
function computeDegreeMap(entities: KGEntity[], relations: KGRelation[]): Map<string, number> {
  const degrees = new Map<string, number>();
  const entityIds = new Set(entities.map((e) => e.id));
  for (const r of relations) {
    if (entityIds.has(r.source_entity_id)) degrees.set(r.source_entity_id, (degrees.get(r.source_entity_id) ?? 0) + 1);
    if (entityIds.has(r.target_entity_id)) degrees.set(r.target_entity_id, (degrees.get(r.target_entity_id) ?? 0) + 1);
  }
  return degrees;
}

function EntityNode({ data }: { data: { label: string; type: string; description?: string; degree: number; selected?: boolean } }) {
  const colors = TYPE_COLORS[data.type] || DEFAULT_COLOR;
  // Gentler scale: 1.0 → 1.5 max (avoids overlap seen in previous version)
  const sizeScale = Math.min(1 + data.degree * 0.08, 1.5);
  const isHighDegree = data.degree >= 4;

  return (
    <div
      className="px-4 py-1.5 cursor-grab"
      style={{
        background: colors.bg,
        border: `1.5px solid ${colors.border}`,
        borderRadius: 20,
        transform: `scale(${sizeScale})`,
        backdropFilter: "blur(8px)",
        boxShadow: isHighDegree
          ? `0 0 16px ${colors.glow}, 0 0 4px ${colors.glow}`
          : `0 0 6px rgba(0,0,0,0.3)`,
      }}
    >
      {/* Hidden handles — required by ReactFlow for edge routing, zero visual/interaction cost */}
      <Handle type="target" position={Position.Top} style={{ opacity: 0, width: 0, height: 0, pointerEvents: "none" }} />
      <Handle type="source" position={Position.Bottom} style={{ opacity: 0, width: 0, height: 0, pointerEvents: "none" }} />
      <div className="text-[11px] font-medium whitespace-nowrap" style={{ color: colors.text }}>
        {data.label}
      </div>
      <div className="text-[9px] text-center" style={{ color: colors.text, opacity: 0.5 }}>
        {data.type}
      </div>
    </div>
  );
}

const nodeTypes = { entity: EntityNode };

interface SimNode extends SimulationNodeDatum {
  id: string;
  mass: number;
}

/** Build ReactFlow nodes + edges with degree-based sizing metadata */
function buildGraph(entities: KGEntity[], relations: KGRelation[]) {
  const entityIds = new Set(entities.map((e) => e.id));
  const degreeMap = computeDegreeMap(entities, relations);

  const nodes: Node[] = entities.map((e) => ({
    id: e.id,
    type: "entity",
    position: { x: 0, y: 0 },
    data: {
      label: e.name,
      type: e.entity_type,
      description: e.description,
      degree: degreeMap.get(e.id) ?? 0,
    },
  }));

  const edges: Edge[] = relations
    .filter((r) => entityIds.has(r.source_entity_id) && entityIds.has(r.target_entity_id))
    .map((r) => ({
      id: r.id,
      source: r.source_entity_id,
      target: r.target_entity_id,
      // Store label in data for selection reveal — hidden by default to reduce clutter
      label: undefined,
      data: { relationLabel: r.relation_type.replace(/_/g, " ") },
      animated: false,
      type: "default",
      style: { stroke: "#334155", strokeWidth: 1, opacity: 0.4 },
    }));

  return { nodes, edges };
}

/** Run d3-force simulation with mass-based hierarchy for depth layering. */
function computeForceLayout(nodes: Node[], edges: Edge[], entities: KGEntity[]): Node[] {
  if (nodes.length === 0) return nodes;

  // Build entity type lookup for mass assignment
  const entityTypeMap = new Map(entities.map((e) => [e.id, e.entity_type]));

  const simNodes: SimNode[] = nodes.map((n) => {
    const entityType = entityTypeMap.get(n.id) ?? "";
    return {
      id: n.id,
      x: n.position.x,
      y: n.position.y,
      mass: TYPE_MASS[entityType] ?? DEFAULT_MASS,
    };
  });
  const simLinks = edges.map((e) => ({ source: e.source, target: e.target }));

  const w = 600;
  const h = 400;

  const simulation = forceSimulation(simNodes)
    .force("link", forceLink(simLinks).id((d: any) => d.id).distance(220).strength(0.4))
    // Mass-aware repulsion: heavier nodes push harder → anchor center, lighter orbit out
    .force("charge", forceManyBody().strength((d: any) => -300 * (d.mass ?? 1)))
    .force("center", forceCenter(w / 2, h / 2))
    .force("x", forceX(w / 2).strength(0.03))
    .force("y", forceY(h / 2).strength(0.03))
    // Larger collision radius prevents node overlap
    .force("collide", forceCollide().radius((d: any) => 55 + (d.mass ?? 1) * 5).strength(0.8))
    .stop();

  // Run simulation to completion synchronously (~300 ticks → 1 render instead of 300)
  const ticks = Math.ceil(Math.log(simulation.alphaMin()) / Math.log(1 - simulation.alphaDecay()));
  for (let i = 0; i < ticks; i++) simulation.tick();

  return nodes.map((n, i) => ({
    ...n,
    position: { x: simNodes[i]!.x ?? 0, y: simNodes[i]!.y ?? 0 },
  }));
}

interface KGGraphViewProps {
  entities: KGEntity[];
  relations: KGRelation[];
  onEntityClick?: (entity: KGEntity) => void;
}

export function KGGraphView(props: KGGraphViewProps) {
  return (
    <ReactFlowProvider>
      <KGGraphViewInner {...props} />
    </ReactFlowProvider>
  );
}

function KGGraphViewInner({ entities, relations, onEntityClick }: KGGraphViewProps) {
  const { t } = useTranslation("memory");
  const { fitView } = useReactFlow();
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [layoutReady, setLayoutReady] = useState(false);

  // Build graph data immediately (cheap), defer force layout to next frame (expensive)
  const { rawNodes, rawEdges } = useMemo(() => {
    const { nodes, edges } = buildGraph(entities, relations);
    return { rawNodes: nodes, rawEdges: edges };
  }, [entities, relations]);

  const [layoutNodes, setLayoutNodes] = useState<Node[]>([]);
  const layoutEdges = rawEdges;

  // Defer heavy force simulation — setTimeout(0) yields to browser paint first
  useEffect(() => {
    setLayoutReady(false);
    const timer = setTimeout(() => {
      const positioned = computeForceLayout(rawNodes, rawEdges, entities);
      setLayoutNodes(positioned);
      setLayoutReady(true);
    }, 0);
    return () => clearTimeout(timer);
  }, [rawNodes, rawEdges, entities]);

  const [nodes, setNodes, onNodesChange] = useNodesState(layoutNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(layoutEdges);

  // Sync nodes when layout completes
  useEffect(() => {
    setNodes(layoutNodes);
    requestAnimationFrame(() => fitView({ padding: 0.15, duration: 300 }));
  }, [layoutNodes, setNodes, fitView]);

  // Sync base edges when data changes
  useEffect(() => {
    setEdges(layoutEdges);
  }, [layoutEdges, setEdges]);

  // Apply selection styling via setEdges callback — avoids full edge array rebuild in render
  useEffect(() => {
    if (!selectedNodeId) {
      // Reset all edges to default
      setEdges((eds) => eds.map((e) => ({
        ...e,
        label: undefined,
        animated: false,
        style: { stroke: "#334155", strokeWidth: 1, opacity: 0.4 },
        labelStyle: undefined,
        labelBgStyle: undefined,
        labelBgPadding: undefined,
        labelShowBg: undefined,
      })));
      return;
    }
    const entity = entities.find((ent) => ent.id === selectedNodeId);
    const colors = TYPE_COLORS[entity?.entity_type ?? ""] || DEFAULT_COLOR;
    setEdges((eds) => eds.map((e) => {
      const isConnected = e.source === selectedNodeId || e.target === selectedNodeId;
      if (!isConnected) return { ...e, label: undefined, animated: false, style: { stroke: "#1e293b", strokeWidth: 0.5, opacity: 0.15 } };
      return {
        ...e,
        label: (e.data as any)?.relationLabel,
        animated: true,
        style: { stroke: colors.border, strokeWidth: 2, opacity: 0.9 },
        labelStyle: { fontSize: 9, fill: colors.text, fontWeight: 500 },
        labelBgStyle: { fill: "rgba(15,23,42,0.85)", stroke: colors.border, rx: 4 },
        labelBgPadding: [4, 2] as [number, number],
        labelShowBg: true,
      };
    }));
  }, [selectedNodeId, entities, setEdges]);

  // useTransition lets React yield to browser paint before processing edge updates
  const [, startTransition] = useTransition();

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      startTransition(() => {
        setSelectedNodeId((prev) => (prev === node.id ? null : node.id));
      });
      if (!onEntityClick) return;
      const entity = entities.find((e) => e.id === node.id);
      if (entity) onEntityClick(entity);
    },
    [entities, onEntityClick, startTransition],
  );

  const handlePaneClick = useCallback(() => {
    startTransition(() => setSelectedNodeId(null));
  }, [startTransition]);

  if (entities.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t("kg.graphView.empty")}
      </div>
    );
  }

  if (!layoutReady) {
    return (
      <div className="flex h-full items-center justify-center" style={{ background: "#0a0a12" }}>
        <div className="text-sm" style={{ color: "#64748b" }}>{t("kg.graphView.loading")}</div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col rounded-md border overflow-hidden" style={{ position: "relative" }}>
      {/* Dark canvas with atmospheric depth gradient — center glow like GitNexus */}
      <div
        className="pointer-events-none absolute inset-0 z-0"
        style={{
          background: "radial-gradient(ellipse at 50% 45%, rgba(124,58,237,0.06) 0%, rgba(30,41,59,0.02) 50%, transparent 80%), linear-gradient(to bottom, #0a0a12, #0f1120, #0a0a12)",
        }}
      />

      <div className="min-h-0 flex-1 relative z-10">
        <ReactFlow
          nodes={nodes}
          edges={edges}
          onNodesChange={onNodesChange}
          onEdgesChange={onEdgesChange}
          onNodeClick={handleNodeClick}
          onPaneClick={handlePaneClick}
          nodeTypes={nodeTypes}
          colorMode="dark"
          fitView
          minZoom={0.1}
          maxZoom={3}
          nodesConnectable={false}
          nodesDraggable={true}
          elementsSelectable={true}
          proOptions={{ hideAttribution: true }}
        >
          <Background gap={30} size={0.5} color="#1a1a2e" />
          <Controls showInteractive={false} />
          <MiniMap
            nodeColor={(n) => {
              const type = (n.data as any)?.type as string;
              return (TYPE_COLORS[type] || DEFAULT_COLOR).border;
            }}
            maskColor="rgba(0,0,0,0.4)"
            style={{ width: 100, height: 75, background: "#0f1120" }}
          />
        </ReactFlow>
      </div>

      {/* Legend + selected node info — dark bar */}
      <div className="flex flex-wrap items-center gap-2 px-3 py-2 border-t border-slate-800 text-[10px] relative z-10" style={{ background: "#0d0d18" }}>
        {Object.entries(TYPE_COLORS).map(([type, colors]) => (
          <div key={type} className="flex items-center gap-1">
            <div className="w-2 h-2 rounded-full" style={{ background: colors.border, boxShadow: `0 0 4px ${colors.glow}` }} />
            <span style={{ color: "#64748b" }}>{type}</span>
          </div>
        ))}
        {selectedNodeId && (
          <div className="ml-auto animate-in fade-in" style={{ color: "#94a3b8" }}>
            {t("kg.graphView.selected", { name: entities.find((e) => e.id === selectedNodeId)?.name ?? "" })}
          </div>
        )}
      </div>
    </div>
  );
}
