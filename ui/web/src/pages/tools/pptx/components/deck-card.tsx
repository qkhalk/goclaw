import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { SlideView } from "./slide-view";
import type { ParsedDeck } from "../lib/parse-deck-blocks";

interface DeckCardProps {
  parsed: ParsedDeck;
  /** A deck was applied and this card's JSON matches it. */
  applied: boolean;
  onApply: () => void;
}

/**
 * Preview card rendered under an assistant reply that ended with a ```deck
 * block: theme swatches + slide count at a glance, a mini strip of the slide
 * layouts, Apply, and an expandable raw JSON view. Invalid blocks stay
 * visible as an error card so the conversation keeps flowing.
 */
export function DeckCard({ parsed, applied, onApply }: DeckCardProps) {
  const { t } = useTranslation("toolbox");
  const [showJson, setShowJson] = useState(false);

  if (!parsed.ok) {
    return (
      <div className="mt-2 rounded-lg border border-destructive/40 bg-destructive/5 p-3">
        <div className="flex items-center gap-2">
          <Badge variant="destructive" className="shrink-0">
            {t("pptx.designer.invalid")}
          </Badge>
          <span className="truncate text-xs text-muted-foreground" title={parsed.error}>
            {parsed.error}
          </span>
        </div>
      </div>
    );
  }

  const { deck } = parsed;
  const swatches = [deck.theme.background, deck.theme.foreground, deck.theme.accent, deck.theme.muted];

  return (
    <div className="pptx-enter mt-2 rounded-lg border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center gap-2">
        {/* Theme swatches */}
        <div className="flex h-6 items-center gap-1 rounded-md border bg-background px-1.5">
          {swatches.map((c, i) => (
            <span
              key={i}
              className="h-3.5 w-3.5 rounded-full border border-black/10"
              style={{ backgroundColor: c }}
              title={c}
            />
          ))}
        </div>
        <span className="text-sm text-muted-foreground tabular-nums">
          {t("pptx.designer.slideCount", { n: deck.slides.length })}
        </span>
        <span className="truncate text-xs text-muted-foreground">
          {deck.theme.font_heading} + {deck.theme.font_body}
        </span>
        {applied && (
          <Badge variant="outline" className="ml-auto shrink-0 border-green-600/40 text-green-600">
            <Check className="mr-1 h-3 w-3" />
            {t("pptx.designer.applied")}
          </Badge>
        )}
      </div>

      {/* Mini deck strip: real slide thumbnails, numbered, capped at 12 */}
      <div className="mt-2.5 flex gap-1.5 overflow-x-auto pb-0.5">
        {deck.slides.slice(0, 12).map((s, i) => (
          <div
            key={i}
            className="relative w-20 shrink-0 overflow-hidden rounded-md border"
            title={`${i + 1}. ${s.layout}${s.title ? ` · ${s.title}` : ""}`}
          >
            <SlideView slide={s} theme={deck.theme} />
            <span className="absolute left-0.5 top-0.5 rounded bg-background/80 px-1 text-[9px] leading-[14px] tabular-nums text-muted-foreground">
              {i + 1}
            </span>
          </div>
        ))}
        {deck.slides.length > 12 && (
          <div className="flex h-9 items-center px-1 text-xs text-muted-foreground tabular-nums">
            +{deck.slides.length - 12}
          </div>
        )}
      </div>

      <div className={cn("mt-2.5 flex items-center gap-2")}>
        <Button size="sm" onClick={onApply} className="min-h-11 sm:min-h-8">
          {t("pptx.designer.apply")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setShowJson((v) => !v)}
          className="min-h-11 text-muted-foreground sm:min-h-8"
        >
          {showJson ? t("pptx.designer.hideJson") : t("pptx.designer.viewJson")}
        </Button>
      </div>

      {showJson && (
        <pre className="mt-2 max-h-48 overflow-auto rounded-md border bg-background p-2 font-mono text-xs leading-relaxed">
          {JSON.stringify(deck, null, 2)}
        </pre>
      )}
    </div>
  );
}
