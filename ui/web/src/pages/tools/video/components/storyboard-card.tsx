import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { getIconDef } from "../lib/icon-library";
import type { ParsedStoryboard } from "../lib/parse-storyboard-blocks";
import type { Scene } from "../hooks/use-timeline";

interface StoryboardCardProps {
  parsed: ParsedStoryboard;
  /** A storyboard was applied and this card's JSON matches it. */
  applied: boolean;
  onApply: (storyboard: Extract<ParsedStoryboard, { ok: true }>["storyboard"]) => void;
}

/**
 * Preview card rendered under an assistant reply that carries a
 * ```storyboard block: canvas ratio + scene count + duration at a glance, a
 * mini strip of the scenes, Apply, and an expandable raw JSON view. Invalid
 * blocks stay visible as an error card so the conversation keeps flowing.
 */
export function StoryboardCard({ parsed, applied, onApply }: StoryboardCardProps) {
  const { t } = useTranslation("toolbox");
  const [showJson, setShowJson] = useState(false);

  if (!parsed.ok) {
    return (
      <div className="mt-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3">
        <div className="flex items-center gap-2">
          <Badge variant="destructive" className="shrink-0">
            {t("video.designer.invalid")}
          </Badge>
          <span className="truncate text-xs text-muted-foreground" title={parsed.error}>
            {parsed.error}
          </span>
        </div>
      </div>
    );
  }

  const { storyboard } = parsed;
  const totalSec = storyboard.scenes.reduce(
    (acc, s) => acc + (Number(s.duration_sec) || 0),
    0,
  );
  const { width, height } = storyboard.canvas;
  const ratioLabel = ratioOf(width, height);

  return (
    <div className="mt-2 rounded-lg border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm text-muted-foreground tabular-nums">
          {t("video.designer.sceneCount", { n: storyboard.scenes.length })}
        </span>
        <span className="text-xs text-muted-foreground tabular-nums">
          {t("video.designer.durationSec", { sec: totalSec.toFixed(1) })}
        </span>
        <Badge variant="outline" className="shrink-0 text-muted-foreground tabular-nums">
          {ratioLabel} · {width}x{height}
        </Badge>
        {applied && (
          <Badge variant="outline" className="ml-auto shrink-0 border-green-600/40 text-green-600">
            <Check className="mr-1 h-3 w-3" />
            {t("video.designer.applied")}
          </Badge>
        )}
      </div>

      {/* Mini scene strip: gradient/color/icon/image thumbnails, numbered,
          capped at 12 */}
      <div className="mt-2.5 flex gap-1.5 overflow-x-auto pb-0.5">
        {storyboard.scenes.slice(0, 12).map((s, i) => (
          <div
            key={i}
            className="relative h-14 w-10 shrink-0 overflow-hidden rounded-md border"
            title={`${i + 1}. ${sceneTitle(s)}`}
          >
            <SceneMiniThumb scene={s} />
            <span className="absolute left-0.5 top-0.5 rounded bg-background/80 px-1 text-[9px] leading-[14px] tabular-nums text-muted-foreground">
              {i + 1}
            </span>
          </div>
        ))}
        {storyboard.scenes.length > 12 && (
          <div className="flex h-14 items-center px-1 text-xs text-muted-foreground tabular-nums">
            +{storyboard.scenes.length - 12}
          </div>
        )}
      </div>

      <div className={cn("mt-2.5 flex items-center gap-2")}>
        <Button size="sm" onClick={() => onApply(storyboard)} className="min-h-11 sm:min-h-8">
          {t("video.designer.apply")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowJson((v) => !v)}
          className="min-h-11 text-muted-foreground sm:min-h-8"
        >
          {showJson ? t("video.designer.hideJson") : t("video.designer.viewJson")}
        </Button>
      </div>

      {showJson && (
        <pre className="mt-2 max-h-48 overflow-auto rounded-md border bg-background p-2 font-mono text-xs leading-relaxed">
          {JSON.stringify(storyboard, null, 2)}
        </pre>
      )}
    </div>
  );
}

function SceneMiniThumb({ scene }: { scene: Scene }) {
  const background = scene.gradient
    ? `linear-gradient(135deg, ${scene.gradient.from}, ${scene.gradient.to})`
    : (scene.color ?? "#000000");

  if (scene.type === "image" || scene.type === "video") {
    if (scene.source) {
      return (
        <img
          src={scene.source}
          alt=""
          className="h-full w-full object-cover"
          crossOrigin="anonymous"
          onError={(e) => {
            (e.target as HTMLImageElement).style.display = "none";
          }}
        />
      );
    }
    return <div className="h-full w-full bg-muted" />;
  }

  const def = scene.type === "icon" ? getIconDef(scene.icon?.name) : undefined;
  return (
    <div className="flex h-full w-full items-center justify-center" style={{ background }}>
      {def && (
        <svg
          viewBox="0 0 24 24"
          className="h-3.5 w-3.5"
          fill="none"
          stroke={scene.icon?.color || "#ffffff"}
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
          aria-hidden
        >
          {def.d.map((d, i) => (
            <path key={i} d={d} />
          ))}
        </svg>
      )}
    </div>
  );
}

function ratioOf(width: number, height: number): string {
  const g = (a: number, b: number): number => (b === 0 ? a : g(b, a % b));
  const d = g(width, height) || 1;
  const w = Math.round(width / d);
  const h = Math.round(height / d);
  // Collapse verbose ratios (1080:1920 → 9:16 already collapses; guard odd ones).
  if (w <= 32 && h <= 32) return `${w}:${h}`;
  return `${(width / height).toFixed(2)}:1`;
}

function sceneTitle(s: Scene): string {
  const kind = s.type;
  const text = s.caption?.text ?? s.narration ?? "";
  return text ? `${kind} · ${text}` : kind;
}
