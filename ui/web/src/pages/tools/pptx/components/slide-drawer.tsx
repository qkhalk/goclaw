import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  DECK_LAYOUTS,
  SLIDE_TRANSITIONS,
  type Slide,
  type SlideLayout,
  type SlideTransition,
} from "../types";

/**
 * Slide-level settings as a drawer (the old always-on bottom editor form is
 * gone): layout template, transition and speaker notes for the current
 * slide. Text/content itself is edited directly on the canvas, so nothing
 * here duplicates it.
 */

interface SlideDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  slide: Slide;
  index: number;
  total: number;
  onLayoutChange: (layout: SlideLayout) => void;
  onTransitionChange: (transition: SlideTransition | undefined) => void;
  onPatch: (patch: Partial<Slide>) => void;
}

export function SlideDrawer({
  open,
  onOpenChange,
  slide,
  index,
  total,
  onLayoutChange,
  onTransitionChange,
  onPatch,
}: SlideDrawerProps) {
  const { t } = useTranslation("toolbox");

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>{t("pptx.slide_settings", { defaultValue: "Slide" })}</SheetTitle>
          <SheetDescription>
            {t("pptx.status.slide_of", { x: index + 1, y: total })}
          </SheetDescription>
        </SheetHeader>
        <div className="flex flex-col gap-5 overflow-y-auto overscroll-contain p-4">
          <div className="flex flex-col gap-1.5">
            <Label className="text-xs text-muted-foreground">{t("pptx.field.layout")}</Label>
            <Select value={slide.layout} onValueChange={(v) => onLayoutChange(v as SlideLayout)}>
              <SelectTrigger className="min-h-11 sm:min-h-9" aria-label={t("pptx.field.layout")}>
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
            <p className="text-xs text-muted-foreground">
              {t("pptx.slide_settings_layout_hint", {
                defaultValue: "Applying a template resets the slide content to that layout.",
              })}
            </p>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-xs text-muted-foreground">{t("pptx.transition")}</Label>
            <Select
              value={slide.transition ?? "none"}
              onValueChange={(v) => onTransitionChange(v === "none" ? undefined : (v as SlideTransition))}
            >
              <SelectTrigger className="min-h-11 sm:min-h-9" aria-label={t("pptx.transition")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SLIDE_TRANSITIONS.map((tr) => (
                  <SelectItem key={tr} value={tr}>
                    {t(`pptx.transition_${tr}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1.5">
            <Label className="text-xs text-muted-foreground">{t("pptx.field.notes")}</Label>
            <Textarea
              rows={5}
              value={slide.notes ?? ""}
              onChange={(e) => onPatch({ notes: e.target.value })}
              placeholder={t("pptx.present_no_notes")}
              className="text-base sm:text-sm"
            />
          </div>

          <Button
            variant="outline"
            className="min-h-11 self-start sm:min-h-9"
            onClick={() => onOpenChange(false)}
          >
            {t("pptx.slide_settings_done", { defaultValue: "Done" })}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
