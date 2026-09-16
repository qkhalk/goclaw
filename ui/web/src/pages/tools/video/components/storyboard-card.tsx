import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Circle, Image as ImageIcon, Video } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ParsedStoryboard } from "../lib/parse-storyboard-blocks";

interface StoryboardCardProps {
  parsed: ParsedStoryboard;
  /** A storyboard was applied and this card's JSON matches it. */
  applied: boolean;
  onApply: () => void;
}

function gcd(a: number, b: number): number {
  return b === 0 ? a : gcd(b, a % b);
}

function aspectLabel(w: number, h: number): string {
  if (!w || !h) return "";
  for (const [rw, rh, label] of [
    [9, 16, "9:16"],
    [16, 9, "16:9"],
    [1, 1, "1:1"],
  ] as const) {
    if (Math.abs(w / h - rw / rh) < 0.01) return label;
  }
  const d = gcd(w, h);
  return `${Math.round(w / d)}:${Math.round(h / d)}`;
}

/**
 * Preview card rendered under an assistant reply that ended with a
 * ```storyboard block: aspect + scene count + duration at a glance, a mini
 * strip of the scenes, Apply, and an expandable raw JSON view. Invalid
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

  const { sb } = parsed;
  const w = sb.canvas?.width ?? 1080;
  const h = sb.canvas?.height ?? 1920;
  const totalSec = sb.scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0);

  return (
    <div className="mt-2 rounded-lg border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="outline" className="shrink-0 font-medium tabular-nums">
          {aspectLabel(w, h)}
        </Badge>
        <span className="text-sm text-muted-foreground tabular-nums">
          {t("video.designer.sceneCount", { n: sb.scenes.length })}
        </span>
        <span className="text-sm text-muted-foreground tabular-nums">
          {t("video.designer.duration", { sec: totalSec.toFixed(0) })}
        </span>
        {applied && (
          <Badge variant="outline" className="ml-auto shrink-0 border-green-600/40 text-green-600">
            <Check className="mr-1 h-3 w-3" />
            {t("video.designer.applied")}
          </Badge>
        )}
      </div>

      {/* Mini scene strip: one cell per scene, colored swatch for color scenes */}
      <div className="mt-2.5 flex gap-1.5 overflow-x-auto pb-0.5">
        {sb.scenes.slice(0, 12).map((s, i) => (
          <div
            key={i}
            className="flex h-9 min-w-9 shrink-0 items-center justify-center gap-0.5 rounded-md border bg-background px-1.5"
            style={s.type === "color" && s.color ? { backgroundColor: s.color } : undefined}
            title={`${i + 1}. ${s.type} · ${s.duration_sec}s${s.caption?.text ? ` · ${s.caption.text}` : ""}`}
          >
            {s.type === "image" && <ImageIcon className="h-3.5 w-3.5 text-muted-foreground" />}
            {s.type === "video" && <Video className="h-3.5 w-3.5 text-muted-foreground" />}
            {s.type === "color" && (
              <Circle
                className={cn("h-2.5 w-2.5", isDark(s.color) ? "text-white/80" : "text-black/60")}
                fill="currentColor"
              />
            )}
            <span
              className={cn(
                "text-[10px] tabular-nums",
                s.type === "color" && isDark(s.color) ? "text-white/90" : "text-muted-foreground",
              )}
            >
              {Number(s.duration_sec).toFixed(0)}
            </span>
          </div>
        ))}
        {sb.scenes.length > 12 && (
          <div className="flex h-9 items-center px-1 text-xs text-muted-foreground tabular-nums">
            +{sb.scenes.length - 12}
          </div>
        )}
      </div>

      <div className="mt-2.5 flex items-center gap-2">
        <Button
          size="sm"
          onClick={onApply}
          className="min-h-11 sm:min-h-8"
        >
          {t("video.designer.apply")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowJson((v) => !v)}
          className="min-h-11 sm:min-h-8 text-muted-foreground"
        >
          {showJson ? t("video.designer.hideJson") : t("video.designer.viewJson")}
        </Button>
      </div>

      {showJson && (
        <pre className="mt-2 max-h-48 overflow-auto rounded-md border bg-background p-2 font-mono text-xs leading-relaxed">
          {JSON.stringify(sb, null, 2)}
        </pre>
      )}
    </div>
  );
}

function isDark(hex?: string): boolean {
  if (!hex || !/^#[0-9a-fA-F]{6}$/.test(hex)) return false;
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  // Rec. 709 luma — dark swatches need light foreground
  return 0.2126 * r + 0.7152 * g + 0.0722 * b < 128;
}
