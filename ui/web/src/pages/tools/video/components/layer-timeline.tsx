// LayerTimeline — per-scene multi-track view of the scene's timed overlay
// layers (the Remotion/OpenCut-style editing surface). Bars are positioned by
// the layer's start/duration within the scene; the bar body drags to move,
// the edge handles drag to trim. All math is seconds-per-pixel against the
// track's client width, clamped to the scene bounds.
import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { layerWindow, type Layer } from "../hooks/use-timeline";

const KIND_BAR: Record<Layer["kind"], string> = {
  text: "bg-sky-500/70 border-sky-300/60",
  shape: "bg-amber-500/70 border-amber-300/60",
  image: "bg-emerald-500/70 border-emerald-300/60",
};

type DragMode = "move" | "trim-start" | "trim-end";

interface LayerTimelineProps {
  layers: Layer[];
  sceneSec: number;
  selected: number;
  onSelect: (index: number) => void;
  onChange: (index: number, timing: { start?: number; duration?: number }) => void;
}

export function LayerTimeline({ layers, sceneSec, selected, onSelect, onChange }: LayerTimelineProps) {
  const { t } = useTranslation("toolbox");
  const trackRef = useRef<HTMLDivElement | null>(null);
  const [drag, setDrag] = useState<{ index: number; mode: DragMode; grabOffset: number } | null>(null);

  const secPerPx = useCallback(() => {
    const w = trackRef.current?.clientWidth ?? 1;
    return sceneSec / Math.max(1, w);
  }, [sceneSec]);

  const onPointerDown = (e: React.PointerEvent, index: number, mode: DragMode) => {
    e.stopPropagation();
    (e.target as HTMLElement).setPointerCapture(e.pointerId);
    const { start } = layerWindow(layers[index]!, sceneSec);
    setDrag({ index, mode, grabOffset: mode === "move" ? (e.clientX - (trackRef.current?.getBoundingClientRect().left ?? 0)) * secPerPx() - start : 0 });
    onSelect(index);
  };

  const onPointerMove = (e: React.PointerEvent) => {
    if (!drag) return;
    const spp = secPerPx();
    const xSec = (e.clientX - (trackRef.current?.getBoundingClientRect().left ?? 0)) * spp;
    const layer = layers[drag.index];
    if (!layer) return;
    const { start, end } = layerWindow(layer, sceneSec);
    const dur = end - start;
    if (drag.mode === "move") {
      const nextStart = Math.max(0, Math.min(sceneSec - dur, xSec - drag.grabOffset));
      onChange(drag.index, { start: round2(nextStart), duration: round2(dur) });
    } else if (drag.mode === "trim-start") {
      const nextStart = Math.max(0, Math.min(end - 0.2, xSec));
      onChange(drag.index, { start: round2(nextStart), duration: round2(end - nextStart) });
    } else {
      const nextEnd = Math.max(start + 0.2, Math.min(sceneSec, xSec));
      onChange(drag.index, { start: round2(start), duration: round2(nextEnd - start) });
    }
  };

  const endDrag = () => setDrag(null);

  return (
    <div className="flex flex-col gap-1">
      <div
        ref={trackRef}
        className="relative h-auto min-h-[28px] w-full overflow-hidden rounded-md border bg-muted/40"
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
      >
        {/* ruler: second ticks */}
        <div className="pointer-events-none absolute inset-0">
          {Array.from({ length: Math.max(1, Math.floor(sceneSec)) }, (_, i) => (
            <div key={i} className="absolute top-0 bottom-0 w-px bg-border/60" style={{ left: `${((i + 1) / sceneSec) * 100}%` }} />
          ))}
        </div>
        {layers.map((layer, i) => {
          const { start, end } = layerWindow(layer, sceneSec);
          const left = (start / sceneSec) * 100;
          const width = Math.max(2, ((end - start) / sceneSec) * 100);
          const label = layer.kind === "text" ? layer.text || t("video.layer.kind_text") : t(`video.layer.kind_${layer.kind}`);
          return (
            <div
              key={i}
              role="button"
              tabIndex={0}
              aria-label={label}
              onPointerDown={(e) => onPointerDown(e, i, "move")}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onSelect(i);
              }}
              className={cn(
                "absolute top-1 bottom-1 cursor-grab touch-none select-none rounded border px-1 text-[10px] leading-[20px] text-white active:cursor-grabbing",
                KIND_BAR[layer.kind],
                i === selected && "ring-2 ring-primary",
              )}
              style={{ left: `${left}%`, width: `${width}%` }}
            >
              <span className="pointer-events-none block truncate">{label}</span>
              {/* trim handles */}
              <span
                aria-hidden
                onPointerDown={(e) => onPointerDown(e, i, "trim-start")}
                className="absolute top-0 bottom-0 left-0 w-1.5 cursor-ew-resize touch-none bg-white/40"
              />
              <span
                aria-hidden
                onPointerDown={(e) => onPointerDown(e, i, "trim-end")}
                className="absolute top-0 bottom-0 right-0 w-1.5 cursor-ew-resize touch-none bg-white/40"
              />
            </div>
          );
        })}
        {layers.length === 0 && (
          <p className="absolute inset-0 flex items-center justify-center text-xs text-muted-foreground">
            {t("video.layers_empty_hint")}
          </p>
        )}
      </div>
    </div>
  );
}

function round2(v: number): number {
  return Math.round(v * 100) / 100;
}
