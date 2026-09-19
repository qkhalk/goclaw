import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { GripVertical, Plus, VolumeX } from "lucide-react";
import { cn } from "@/lib/utils";
import { renderSceneBase } from "./render-shared";
import type { Scene } from "../hooks/use-timeline";

// ── Types ──

interface TimelineProps {
  scenes: Scene[];
  selectedIndex: number;
  onSelect: (index: number) => void;
  onAdd: () => void;
}

// ── Scene thumbnail ──

/** True-paint thumbnail for color scenes: the same renderSceneBase the
 * player uses (gradient + grid + glow + caption chip) on a small offscreen
 * canvas, redrawn when the scene changes or the caption fonts finish
 * loading. Image scenes keep the plain <img> below — the photo is the
 * preview. */
function ColorSceneThumb({ scene }: { scene: Scene }) {
  const ref = useRef<HTMLCanvasElement | null>(null);
  const sceneKey = JSON.stringify(scene);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const paint = () => {
      const ctx = canvas.getContext("2d");
      if (!ctx) return;
      renderSceneBase(ctx, canvas, scene, 0.7, new Map(), undefined, canvas.width / 720);
    };
    paint();
    // Repaint once the bundled caption fonts arrive (canvas falls back to a
    // system face until then).
    document.fonts?.ready.then(paint);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sceneKey]);

  return (
    <canvas
      ref={ref}
      width={216}
      height={384}
      className="h-full w-full rounded-[5px] object-cover"
      aria-hidden
    />
  );
}

function SceneThumb({ scene, index }: { scene: Scene; index: number }) {
  if (scene.type === "color") {
    return <ColorSceneThumb scene={scene} />;
  }

  if (scene.source) {
    return (
      <img
        src={scene.source}
        alt={`Scene ${index + 1}`}
        className="h-full w-full rounded-[5px] object-cover"
        crossOrigin="anonymous"
        onError={(e) => {
          (e.target as HTMLImageElement).style.display = "none";
        }}
      />
    );
  }

  return (
    <div className="flex h-full w-full items-center justify-center rounded-[5px] bg-white/[0.06] text-xs text-zinc-500">
      {index + 1}
    </div>
  );
}

// ── Ruler helpers ──

function formatRulerTime(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = Math.round(sec % 60);
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

const RULER_STEPS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600];

/** Smallest tick step whose pixel spacing stays readable at pxPerSec. */
function rulerStep(pxPerSec: number): number {
  for (const step of RULER_STEPS) {
    if (step * pxPerSec >= 64) return step;
  }
  return RULER_STEPS[RULER_STEPS.length - 1]!;
}

/** Track gutter label (Filmora-style "V1"), sticky at the strip's left. */
function TrackGutter() {
  return (
    <div className="sticky left-0 z-10 flex w-7 shrink-0 items-end justify-center border-r border-white/[0.06] bg-[#1b1d23] pb-1">
      <span className="font-mono text-[10px] font-medium text-zinc-500">V1</span>
    </div>
  );
}

// ── Component ──

/**
 * Filmora-style bottom timeline: dark surface, timecode tick ruler aligned
 * with time-proportional scene clips (gradient thumbnails, rounded clip
 * bodies), sticky V1 track gutter, and an add-scene slot at the end.
 */
export function Timeline({
  scenes,
  selectedIndex,
  onSelect,
  onAdd,
}: TimelineProps) {
  const { t } = useTranslation("toolbox");
  const scrollRef = useRef<HTMLDivElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  const [viewportW, setViewportW] = useState(0);

  // Auto-scroll to selected scene
  useEffect(() => {
    selectedRef.current?.scrollIntoView({
      behavior: "smooth",
      block: "nearest",
      inline: "center",
    });
  }, [selectedIndex]);

  // Track the viewport width so the strip can fit scenes without scrolling
  // when there is room (and stay readable when there is not).
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const update = () => setViewportW(el.clientWidth);
    update();
    const observer = new ResizeObserver(update);
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  const durations = scenes.map((s) => Number(s.duration_sec) || 0);
  const totalSec = durations.reduce((a, b) => a + b, 0);
  const minSceneSec = durations.length > 0 ? Math.min(...durations) : 0;

  // px per second: fill the viewport when possible, but never let the
  // shortest clip shrink below ~56px; clamped so pathological inputs still
  // render (6..90 px/s).
  const pxPerSec = useMemo(() => {
    const fit = viewportW > 56 && totalSec > 0 ? (viewportW - 56 - 16) / totalSec : 0;
    const readable = minSceneSec > 0 ? 56 / minSceneSec : 0;
    return Math.min(90, Math.max(fit, readable, 6));
  }, [viewportW, totalSec, minSceneSec]);

  const stripWidth = Math.round(totalSec * pxPerSec);
  const step = rulerStep(pxPerSec);
  const ticks = useMemo(() => {
    const out: number[] = [];
    for (let s = 0; s <= Math.floor(totalSec); s += step) out.push(s);
    return out;
  }, [totalSec, step]);

  return (
    <div className="bg-[#1b1d23]">
      <div
        ref={scrollRef}
        className="overflow-x-auto overscroll-contain"
      >
        <div className="flex min-w-full flex-col">
          {/* Timecode tick ruler (aligned with the clip row below — both rows
              are TrackGutter + a strip with identical horizontal geometry and
              no inter-row padding, so tick left = clip left for time t).
              Clips clamp to a 28px minimum width; scenes narrower than that
              break strict proportionality by design. */}
          <div aria-hidden className="flex">
            <TrackGutter />
            <div
              className="relative h-6 shrink-0 border-b border-white/[0.06]"
              style={{ width: stripWidth || undefined, minWidth: stripWidth || "100%" }}
            >
              {ticks.map((sec) => (
                <div
                  key={sec}
                  className="absolute bottom-0 flex flex-col items-start"
                  style={{ left: sec * pxPerSec }}
                >
                  <span className="pl-1 font-mono text-[9px] leading-none tabular-nums text-zinc-500">
                    {formatRulerTime(sec)}
                  </span>
                  <span className="mt-0.5 h-1.5 w-px bg-white/20" />
                </div>
              ))}
            </div>
          </div>

          {/* Clip row — py only: horizontal padding would offset the clips
              from the ruler above. */}
          <div role="listbox" aria-label={t("video.timeline.title")} className="flex py-1">
            <TrackGutter />
            {scenes.map((scene, i) => {
              const isSelected = i === selectedIndex;
              const sceneDur = durations[i] ?? 0;
              return (
                <div
                  key={i}
                  className="shrink-0"
                  style={{ width: Math.max(28, sceneDur * pxPerSec) }}
                >
                  <button
                    ref={isSelected ? selectedRef : undefined}
                    type="button"
                    role="option"
                    aria-selected={isSelected}
                    onClick={() => onSelect(i)}
                    className={cn(
                      "group relative flex h-[68px] w-full flex-col overflow-hidden rounded-md transition-all",
                      isSelected
                        ? "ring-2 ring-primary"
                        : "ring-1 ring-white/10 hover:ring-white/25",
                    )}
                  >
                    {/* Filmora-style accent edge: brand gradient on the
                        selected clip, subtle neutral on the rest. */}
                    <span
                      aria-hidden
                      className={cn(
                        "absolute inset-x-0 top-0 z-10 h-0.5",
                        isSelected
                          ? "bg-gradient-to-r from-rose-500 to-orange-400"
                          : "bg-white/15 group-hover:bg-white/25",
                      )}
                    />
                    {/* Thumbnail */}
                    <div className="relative h-12 w-full overflow-hidden rounded-[5px] bg-black/40">
                      <SceneThumb scene={scene} index={i} />
                      <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/55 via-transparent to-black/10" />
                      {/* Drag handle (visual only) */}
                      <div className="absolute left-0.5 top-0.5 opacity-0 transition-opacity group-hover:opacity-100">
                        <GripVertical className="h-3 w-3 text-white drop-shadow" />
                      </div>
                      {scene.mute && (
                        <span className="absolute right-0.5 top-0.5 flex h-4 w-4 items-center justify-center rounded bg-black/60">
                          <VolumeX className="h-2.5 w-2.5 text-zinc-200" />
                        </span>
                      )}
                    </div>

                    {/* Label + duration */}
                    <div className="flex items-center justify-between gap-1 bg-white/[0.04] px-1 py-0.5">
                      <span className="truncate text-[10px] font-medium text-zinc-300">
                        {t("video.scene_n", { n: i + 1 })}
                      </span>
                      <span className="shrink-0 font-mono text-[10px] tabular-nums text-zinc-500">
                        {sceneDur.toFixed(1)}s
                      </span>
                    </div>
                  </button>
                </div>
              );
            })}

            {/* Add scene slot (sits past the ruler's end, like empty track) */}
            <button
              type="button"
              onClick={onAdd}
              aria-label={t("video.add_scene")}
              className="ml-1 flex h-[68px] w-14 shrink-0 flex-col items-center justify-center gap-1 rounded-md border border-dashed border-white/15 transition-colors hover:border-primary/50 hover:bg-primary/5"
            >
              <Plus className="h-4 w-4 text-zinc-500" />
              <span className="px-1 text-center text-[9px] leading-tight text-zinc-500">
                {t("video.add_scene")}
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
