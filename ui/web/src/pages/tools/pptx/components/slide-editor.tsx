import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Sparkles, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  type AnimEffect,
  type DecorPrim,
  type FrameVariant,
  type Slide,
  type SlideLayout,
  DECK_LAYOUTS,
} from "../types";
import { IconPicker } from "./icon-picker";

interface SlideEditorProps {
  slide: Slide;
  index: number;
  total: number;
  onChange: (patch: Partial<Slide>) => void;
  onLayoutChange: (layout: SlideLayout) => void;
}

const ANIM_EFFECTS: AnimEffect[] = ["fade-in", "slide-up", "slide-left", "scale-in"];

/** Default placement boxes per frame variant (1280×720 stage). */
function defaultFrameBox(variant: FrameVariant): { x: number; y: number; w: number; h: number } {
  switch (variant) {
    case "band":
      return { x: 0, y: 160, w: 40, h: 400 };
    case "dots":
      return { x: 900, y: 72, w: 300, h: 216 };
    case "ring":
      return { x: 920, y: 64, w: 280, h: 280 };
    default:
      return { x: 56, y: 56, w: 1168, h: 608 };
  }
}

/**
 * Form editor for the selected slide. Layout switching goes through the page
 * (it re-derives the field skeleton); field patches are shallow merges so
 * typing never drops sibling fields. Beyond the classic content fields this
 * edits the decoration layer: slide icons, per-bullet/stat icons, frame
 * primitives and their preview-only entrance animations.
 */
export function SlideEditor({ slide, index, total, onChange, onLayoutChange }: SlideEditorProps) {
  const { t } = useTranslation("toolbox");
  const [frameVariant, setFrameVariant] = useState<FrameVariant>("corner");

  const patchDecor = (i: number, patch: Partial<Extract<DecorPrim, { type: "frame" }>>) => {
    const next = (slide.decor ?? []).map((d, j) =>
      j === i && d.type === "frame" ? { ...d, ...patch } : d,
    );
    onChange({ decor: next });
  };

  const removeDecor = (i: number) => {
    onChange({ decor: (slide.decor ?? []).filter((_, j) => j !== i) });
  };

  const addFrame = (variant: FrameVariant) => {
    onChange({
      decor: [
        ...(slide.decor ?? []),
        { type: "frame", variant, ...defaultFrameBox(variant) },
      ],
    });
  };

  const setBulletIcon = (i: number, icon: string | null) => {
    // bullet_icons stays parallel to bullets; null = default square marker.
    const next: (string | null)[] = Array.from(
      { length: (slide.bullets ?? []).length },
      (_, j) => slide.bullet_icons?.[j] ?? null,
    );
    next[i] = icon;
    onChange({ bullet_icons: next.some((n) => n !== null) ? next : undefined });
  };

  const setStatIcon = (i: number, icon: string | null) => {
    const next = (slide.stats ?? []).map((st, j) =>
      j === i ? { ...st, icon: icon ?? undefined } : st,
    );
    onChange({ stats: next });
  };

  const hasAnim = (slide.decor ?? []).some((d) => !!d.anim);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex w-56 flex-col gap-1.5">
          <Label className="text-xs text-muted-foreground">{t("pptx.field.layout")}</Label>
          <Select value={slide.layout} onValueChange={(v) => onLayoutChange(v as SlideLayout)}>
            <SelectTrigger className="min-h-11 sm:min-h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DECK_LAYOUTS.map((l) => (
                <SelectItem key={l} value={l}>
                  {t(`pptx.layout.${l}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <span className="pb-2.5 text-xs text-muted-foreground tabular-nums">
          {index + 1} / {total}
        </span>
      </div>

      <div className="flex items-end gap-3">
        {hasTitle(slide) && (
          <Field label={t("pptx.field.title")} className="flex-1">
            <Input
              value={slide.title ?? ""}
              onChange={(e) => onChange({ title: e.target.value })}
              className="min-h-11 text-base sm:min-h-9 sm:text-sm"
            />
          </Field>
        )}
        {(slide.layout === "title" || slide.layout === "section") && (
          <Field label={t("pptx.field.icon")}>
            <IconPicker
              value={slide.icon}
              onChange={(name) => onChange({ icon: name ?? undefined })}
              label={t("pptx.field.icon")}
              searchPlaceholder={t("pptx.icon.search")}
              clearLabel={t("pptx.icon.none")}
            />
          </Field>
        )}
      </div>

      {(slide.layout === "title" || slide.layout === "section" || slide.layout === "end") && (
        <Field label={t("pptx.field.subtitle")}>
          <Input
            value={slide.subtitle ?? ""}
            onChange={(e) => onChange({ subtitle: e.target.value })}
            className="min-h-11 text-base sm:min-h-9 sm:text-sm"
          />
        </Field>
      )}

      {slide.layout === "bullets" && (
        <>
          <Field label={t("pptx.field.bullets")}>
            <Textarea
              rows={6}
              value={(slide.bullets ?? []).join("\n")}
              onChange={(e) =>
                onChange({ bullets: e.target.value.split("\n").filter((l: string) => l.trim() !== "") })
              }
              className="text-base sm:text-sm"
            />
          </Field>
          {(slide.bullets ?? []).length > 0 && (
            <Field label={t("pptx.field.bullet_icons")}>
              <div className="flex flex-wrap gap-2">
                {(slide.bullets ?? []).map((_, i) => (
                  <IconPicker
                    key={i}
                    value={slide.bullet_icons?.[i]}
                    onChange={(name) => setBulletIcon(i, name)}
                    label={`${t("pptx.field.bullet_icons")} ${i + 1}`}
                    searchPlaceholder={t("pptx.icon.search")}
                    clearLabel={t("pptx.icon.none")}
                  />
                ))}
              </div>
            </Field>
          )}
        </>
      )}

      {slide.layout === "two_column" && (
        <div className="grid gap-4 sm:grid-cols-2">
          {(["left", "right"] as const).map((side) => (
            <div key={side} className="flex flex-col gap-3 rounded-lg border p-3">
              <span className="text-xs font-medium text-muted-foreground">
                {side === "left" ? t("pptx.field.left") : t("pptx.field.right")}
              </span>
              <Input
                value={slide[side]?.heading ?? ""}
                onChange={(e) =>
                  onChange({ [side]: { ...(slide[side] ?? { heading: "", bullets: [] }), heading: e.target.value } })
                }
                placeholder={t("pptx.field.heading")}
                className="min-h-11 text-base sm:min-h-9 sm:text-sm"
              />
              <Textarea
                rows={5}
                value={(slide[side]?.bullets ?? []).join("\n")}
                onChange={(e) =>
                  onChange({
                    [side]: {
                      ...(slide[side] ?? { heading: "", bullets: [] }),
                      bullets: e.target.value.split("\n").filter((l: string) => l.trim() !== ""),
                    },
                  })
                }
                placeholder={t("pptx.field.bullets")}
                className="text-base sm:text-sm"
              />
            </div>
          ))}
        </div>
      )}

      {slide.layout === "quote" && (
        <>
          <Field label={t("pptx.field.quote")}>
            <Textarea
              rows={3}
              value={slide.quote ?? ""}
              onChange={(e) => onChange({ quote: e.target.value })}
              className="text-base sm:text-sm"
            />
          </Field>
          <Field label={t("pptx.field.author")}>
            <Input
              value={slide.author ?? ""}
              onChange={(e) => onChange({ author: e.target.value })}
              className="min-h-11 text-base sm:min-h-9 sm:text-sm"
            />
          </Field>
        </>
      )}

      {slide.layout === "stats" && (
        <div className="flex flex-col gap-2">
          {(slide.stats ?? []).map((st, i) => (
            <div key={i} className="flex items-center gap-2">
              <IconPicker
                value={st.icon}
                onChange={(name) => setStatIcon(i, name)}
                label={`${t("pptx.field.icon")} ${i + 1}`}
                searchPlaceholder={t("pptx.icon.search")}
                clearLabel={t("pptx.icon.none")}
              />
              <Input
                value={st.value}
                onChange={(e) => {
                  const next = [...(slide.stats ?? [])];
                  next[i] = { ...st, value: e.target.value };
                  onChange({ stats: next });
                }}
                placeholder={t("pptx.field.value")}
                className="min-h-11 w-28 text-base sm:min-h-9 sm:text-sm"
              />
              <Input
                value={st.label}
                onChange={(e) => {
                  const next = [...(slide.stats ?? [])];
                  next[i] = { ...st, label: e.target.value };
                  onChange({ stats: next });
                }}
                placeholder={t("pptx.field.label")}
                className="min-h-11 flex-1 text-base sm:min-h-9 sm:text-sm"
              />
              <Button
                variant="ghost"
                size="icon"
                onClick={() => onChange({ stats: (slide.stats ?? []).filter((_, j) => j !== i) })}
                disabled={(slide.stats ?? []).length <= 1}
                className="min-h-11 min-w-11 text-destructive hover:text-destructive sm:min-h-9 sm:min-w-9"
                aria-label={t("pptx.field.remove_stat")}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              onChange({ stats: [...(slide.stats ?? []), { value: "", label: "" }] })
            }
            disabled={(slide.stats ?? []).length >= 4}
            className="min-h-11 self-start sm:min-h-9"
          >
            <Plus className="mr-2 h-4 w-4" />
            {t("pptx.field.add_stat")}
          </Button>
        </div>
      )}

      {slide.layout === "image" && (
        <>
          <Field label={t("pptx.field.source")}>
            <Input
              value={slide.source ?? ""}
              onChange={(e) => onChange({ source: e.target.value })}
              placeholder="https://… / /v1/files/…"
              className="min-h-11 text-base sm:min-h-9 sm:text-sm"
            />
          </Field>
          <p className="-mt-2 text-xs text-muted-foreground">{t("pptx.image_hint")}</p>
          <Field label={t("pptx.field.caption")}>
            <Input
              value={slide.caption ?? ""}
              onChange={(e) => onChange({ caption: e.target.value })}
              className="min-h-11 text-base sm:min-h-9 sm:text-sm"
            />
          </Field>
        </>
      )}

      {hasNotes(slide) && (
        <Field label={t("pptx.field.notes")}>
          <Textarea
            rows={2}
            value={slide.notes ?? ""}
            onChange={(e) => onChange({ notes: e.target.value })}
            className="text-base sm:text-sm"
          />
        </Field>
      )}

      {/* Decoration layer: frame primitives + preview-only animations */}
      <div className="flex flex-col gap-2 rounded-lg border border-dashed p-3">
        <div className="flex items-center gap-2">
          <Sparkles className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          <Label className="text-xs text-muted-foreground">{t("pptx.field.decor")}</Label>
        </div>

        {(slide.decor ?? []).map((d, i) =>
          d.type === "frame" ? (
            <div key={i} className="flex flex-wrap items-center gap-2">
              <span className="min-w-20 text-xs text-muted-foreground">
                {t(`pptx.frame.${d.variant}`)}
              </span>
              <Select
                value={d.anim?.effect ?? "none"}
                onValueChange={(v) =>
                  patchDecor(i, {
                    anim:
                      v === "none"
                        ? undefined
                        : { effect: v as AnimEffect, delayMs: d.anim?.delayMs },
                  })
                }
              >
                <SelectTrigger className="min-h-11 w-32 sm:min-h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">{t("pptx.anim.none")}</SelectItem>
                  {ANIM_EFFECTS.map((fx) => (
                    <SelectItem key={fx} value={fx}>
                      {t(`pptx.anim.${fx.replace("-", "_")}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {d.anim && (
                <Input
                  type="number"
                  min={0}
                  step={100}
                  value={d.anim.delayMs ?? 0}
                  onChange={(e) =>
                    patchDecor(i, {
                      anim: {
                        effect: d.anim!.effect,
                        delayMs: Math.max(0, Math.min(60000, Number(e.target.value) || 0)),
                      },
                    })
                  }
                  title={t("pptx.anim.delay")}
                  aria-label={t("pptx.anim.delay")}
                  className="min-h-11 w-24 text-base sm:min-h-9 sm:text-sm"
                />
              )}
              <Button
                variant="ghost"
                size="icon"
                onClick={() => removeDecor(i)}
                className="ml-auto min-h-11 min-w-11 text-destructive hover:text-destructive sm:min-h-9 sm:min-w-9"
                aria-label={t("pptx.decor.remove")}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>
          ) : null,
        )}

        <div className="flex items-center gap-2">
          <Select value={frameVariant} onValueChange={(v) => setFrameVariant(v as FrameVariant)}>
            <SelectTrigger className="min-h-11 w-40 sm:min-h-9" aria-label={t("pptx.decor.add_frame")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(["corner", "outline", "band", "dots", "ring"] as const).map((v) => (
                <SelectItem key={v} value={v}>
                  {t(`pptx.frame.${v}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            variant="outline"
            size="sm"
            onClick={() => addFrame(frameVariant)}
            className="min-h-11 sm:min-h-9"
          >
            <Plus className="mr-2 h-4 w-4" />
            {t("pptx.decor.add_frame")}
          </Button>
        </div>

        {hasAnim && <p className="text-xs text-muted-foreground">{t("pptx.anim.preview_only")}</p>}
      </div>
    </div>
  );
}

function hasTitle(slide: Slide): boolean {
  return slide.layout !== "quote";
}

function hasNotes(slide: Slide): boolean {
  return ["bullets", "two_column", "stats", "image", "quote"].includes(slide.layout);
}

function Field({ label, children, className }: { label: string; children: React.ReactNode; className?: string }) {
  return (
    <div className={`flex flex-col gap-1.5 ${className ?? ""}`}>
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  );
}
