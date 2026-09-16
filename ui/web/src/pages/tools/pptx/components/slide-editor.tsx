import { useTranslation } from "react-i18next";
import { Plus, Trash2 } from "lucide-react";
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
import { DECK_LAYOUTS, type Slide, type SlideLayout } from "../types";

interface SlideEditorProps {
  slide: Slide;
  index: number;
  total: number;
  onChange: (patch: Partial<Slide>) => void;
  onLayoutChange: (layout: SlideLayout) => void;
}

/**
 * Form editor for the selected slide. Layout switching goes through the page
 * (it re-derives the field skeleton); field patches are shallow merges so
 * typing never drops sibling fields.
 */
export function SlideEditor({ slide, index, total, onChange, onLayoutChange }: SlideEditorProps) {
  const { t } = useTranslation("toolbox");

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

      {hasTitle(slide) && (
        <Field label={t("pptx.field.title")}>
          <Input
            value={slide.title ?? ""}
            onChange={(e) => onChange({ title: e.target.value })}
            className="min-h-11 text-base sm:min-h-9 sm:text-sm"
          />
        </Field>
      )}

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
        <Field label={t("pptx.field.bullets")}>
          <Textarea
            rows={6}
            value={(slide.bullets ?? []).join("\n")}
            onChange={(e) =>
              onChange({ bullets: e.target.value.split("\n").filter((l) => l.trim() !== "") })
            }
            className="text-base sm:text-sm"
          />
        </Field>
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
                      bullets: e.target.value.split("\n").filter((l) => l.trim() !== ""),
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
    </div>
  );
}

function hasTitle(slide: Slide): boolean {
  return slide.layout !== "quote";
}

function hasNotes(slide: Slide): boolean {
  return ["bullets", "two_column", "stats", "image", "quote"].includes(slide.layout);
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  );
}
