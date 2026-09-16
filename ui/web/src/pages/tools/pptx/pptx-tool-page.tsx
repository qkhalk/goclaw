import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowUp,
  Copy,
  Download,
  Loader2,
  Plus,
  Presentation,
  Trash2,
  Wand2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/shared/page-header";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import { useIsTablet } from "@/hooks/use-media-query";
import { useUiStore } from "@/stores/use-ui-store";
import {
  SAFE_FONTS,
  THEME_PRESETS,
  blankSlide,
  defaultDeck,
  type Deck,
  type DeckTheme,
  type Slide,
  type SlideLayout,
} from "./types";
import { exportDeckPptx } from "./lib/pptx-export";
import { parseDeck } from "./lib/parse-deck-blocks";
import { SlideView } from "./components/slide-view";
import { SlideEditor } from "./components/slide-editor";
import { DesignerColumn } from "./components/designer-column";

function slugifyTitle(deck: Deck): string {
  const titleSlide = deck.slides.find((s) => s.layout === "title") ?? deck.slides[0];
  const raw = (titleSlide?.title ?? "").trim() || "presentation";
  const slug = raw
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/đ/g, "d")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 60);
  return slug || "presentation";
}

export function PptxToolPage() {
  const { t } = useTranslation("toolbox");

  const [deck, setDeck] = useState<Deck>(defaultDeck);
  const [selected, setSelected] = useState(0);
  const [exporting, setExporting] = useState(false);
  const [showJson, setShowJson] = useState(false);
  const [jsonDraft, setJsonDraft] = useState("");
  const [jsonError, setJsonError] = useState("");

  // Designer column (chat rail on desktop, bottom sheet on tablets/phones)
  const isCompact = useIsTablet();
  const designerOpen = useUiStore((s) => s.pptxDesignerOpen);
  const setDesignerOpen = useUiStore((s) => s.setPptxDesignerOpen);

  const current = deck.slides[selected];
  const firstSlide = deck.slides[0];

  const patchSlide = (patch: Partial<Slide>) => {
    setDeck((d) => ({
      ...d,
      slides: d.slides.map((s, i) => (i === selected ? { ...s, ...patch } : s)),
    }));
  };

  const changeLayout = (layout: SlideLayout) => {
    setDeck((d) => ({
      ...d,
      slides: d.slides.map((s, i) => {
        if (i !== selected) return s;
        const fresh = blankSlide(layout);
        return {
          ...fresh,
          title: s.title ?? fresh.title,
          notes: s.notes,
        };
      }),
    }));
  };

  const addSlide = () => {
    setDeck((d) => {
      const slides = [...d.slides];
      slides.splice(selected + 1, 0, blankSlide("bullets"));
      return { ...d, slides };
    });
    setSelected((i) => i + 1);
  };

  const duplicateSlide = () => {
    if (!current) return;
    setDeck((d) => {
      const slides = [...d.slides];
      slides.splice(selected + 1, 0, JSON.parse(JSON.stringify(current)) as Slide);
      return { ...d, slides };
    });
    setSelected((i) => i + 1);
  };

  const removeSlide = () => {
    if (deck.slides.length <= 1) return;
    setDeck((d) => ({ ...d, slides: d.slides.filter((_, i) => i !== selected) }));
    setSelected((i) => Math.min(i, deck.slides.length - 2));
  };

  const moveSlide = (dir: -1 | 1) => {
    const to = selected + dir;
    if (to < 0 || to >= deck.slides.length) return;
    setDeck((d) => {
      const slides = [...d.slides];
      [slides[selected], slides[to]] = [slides[to]!, slides[selected]!];
      return { ...d, slides };
    });
    setSelected(to);
  };

  const patchTheme = (patch: Partial<DeckTheme>) => {
    setDeck((d) => ({ ...d, theme: { ...d.theme, ...patch } }));
  };

  const applyPreset = (theme: DeckTheme) => {
    setDeck((d) => ({ ...d, theme: { ...theme } }));
  };

  // The one Apply path shared by the JSON mode and the designer column, so
  // the preview, rail and export payload stay consistent.
  const applyDeck = (next: Deck) => {
    setDeck({ ...next, version: 1 });
    setSelected(0);
  };

  function loadJson() {
    const parsed = parseDeck(jsonDraft);
    if (!parsed.ok) {
      setJsonError(t("pptx.json_invalid", { error: parsed.error }));
      return;
    }
    applyDeck(parsed.deck);
    setJsonError("");
    setShowJson(false);
  }

  async function handleExport() {
    setExporting(true);
    try {
      await exportDeckPptx(deck, `${slugifyTitle(deck)}.pptx`);
      toast.success(t("pptx.export_done"));
    } catch (e) {
      toast.error(
        e instanceof Error && e.message
          ? t("pptx.export_failed", { error: e.message })
          : t("pptx.export_failed", { error: "" }),
      );
    } finally {
      setExporting(false);
    }
  }

  return (
    // The page fills the app scrollport exactly (h-full of <main>): the
    // studio column scrolls internally and the designer rail always fits
    // the viewport. sticky+h-dvh here used to overflow the scrollport by
    // the topbar height, cutting the composer below the fold.
    <div className="h-full min-h-0">
      <div className="mx-auto flex h-full w-full items-stretch">
        <div className="mx-auto flex w-full min-w-0 max-w-5xl flex-col gap-6 overflow-y-auto overscroll-contain px-4 py-6">
        <PageHeader
          title={t("pptx.title")}
          description={t("pptx.description")}
          actions={
            <>
              <Button
                onClick={handleExport}
                disabled={exporting || deck.slides.length === 0}
                className="min-h-11 sm:min-h-9"
              >
                {exporting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    {t("pptx.exporting")}
                  </>
                ) : (
                  <>
                    <Download className="mr-2 h-4 w-4" />
                    {t("pptx.export")}
                  </>
                )}
              </Button>
              <Button
                variant={designerOpen ? "default" : "outline"}
                size="sm"
                onClick={() => setDesignerOpen(!designerOpen)}
                className="min-h-11 sm:min-h-9"
                title={t("pptx.designer.toggle")}
              >
                <Wand2 className="mr-2 h-4 w-4" />
                {t("pptx.designer.title")}
              </Button>
            </>
          }
        />

        <Tabs defaultValue="slides">
          <TabsList>
            <TabsTrigger value="slides">{t("pptx.tabs.slides")}</TabsTrigger>
            <TabsTrigger value="theme">{t("pptx.tabs.theme")}</TabsTrigger>
          </TabsList>

          {/* ── Slides tab: one stage surface (preview + rail), editor below ── */}
          <TabsContent value="slides" className="flex flex-col gap-4">
            {/* Stage: preview on top, hairline divider, thumbnails below — a
                single surface so the deck reads as one object, not stacked boxes */}
            <div className="overflow-hidden rounded-lg border bg-background shadow-sm">
              {current && (
                <div className="p-2 sm:p-3">
                  {/* key={selected}: soft crossfade when the slide changes */}
                  <div key={selected} className="pptx-enter-soft overflow-hidden rounded-md">
                    <SlideView slide={current} theme={deck.theme} />
                  </div>
                </div>
              )}

              <div className="border-t px-3 pb-3 pt-2">
                <div className="flex items-center gap-2 pb-2">
                  <span className="text-sm font-medium tabular-nums">
                    {t("pptx.slides_count", { n: deck.slides.length })}
                  </span>
                  {current && (
                    <span className="text-xs text-muted-foreground tabular-nums">
                      {selected + 1} / {deck.slides.length}
                    </span>
                  )}
                  <div className="ml-auto flex items-center gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => moveSlide(-1)}
                      disabled={selected === 0}
                      className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
                      aria-label={t("pptx.move_up")}
                      title={t("pptx.move_up")}
                    >
                      <ArrowUp className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => moveSlide(1)}
                      disabled={selected >= deck.slides.length - 1}
                      className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
                      aria-label={t("pptx.move_down")}
                      title={t("pptx.move_down")}
                    >
                      <ArrowDown className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={duplicateSlide}
                      className="min-h-11 min-w-11 sm:min-h-8 sm:min-w-8"
                      aria-label={t("pptx.duplicate")}
                      title={t("pptx.duplicate")}
                    >
                      <Copy className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={removeSlide}
                      disabled={deck.slides.length <= 1}
                      className="min-h-11 min-w-11 text-destructive hover:text-destructive sm:min-h-8 sm:min-w-8"
                      aria-label={t("pptx.remove")}
                      title={t("pptx.remove")}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <div className="flex gap-2 overflow-x-auto pb-1">
                  {deck.slides.map((s, i) => (
                    <button
                      key={i}
                      type="button"
                      onClick={() => setSelected(i)}
                      className={cn(
                        "w-40 shrink-0 overflow-hidden rounded-md border-2 text-left transition-[border-color,transform] duration-200 hover:-translate-y-0.5 active:scale-[0.98]",
                        i === selected
                          ? "border-primary"
                          : "border-transparent hover:border-border",
                      )}
                      aria-label={`${t("pptx.layout." + s.layout)} ${i + 1}`}
                    >
                      <div className="relative pointer-events-none">
                        <SlideView slide={s} theme={deck.theme} />
                        <span className="absolute left-1 top-1 rounded bg-background/80 px-1 text-[10px] leading-4 tabular-nums text-muted-foreground">
                          {i + 1}
                        </span>
                      </div>
                    </button>
                  ))}
                  <button
                    type="button"
                    onClick={addSlide}
                    disabled={deck.slides.length >= 40}
                    className="flex w-40 shrink-0 flex-col items-center justify-center gap-1 rounded-md border border-dashed text-muted-foreground transition-colors hover:bg-accent min-h-11"
                  >
                    <Plus className="h-5 w-5" />
                    <span className="text-xs">{t("pptx.add_slide")}</span>
                  </button>
                </div>
              </div>
            </div>

            {/* Slide editor */}
            {current && (
              <div key={`editor-${selected}`} className="pptx-enter-soft rounded-lg border p-4">
                <SlideEditor
                  slide={current}
                  index={selected}
                  total={deck.slides.length}
                  onChange={patchSlide}
                  onLayoutChange={changeLayout}
                />
              </div>
            )}
          </TabsContent>

          {/* ── Theme tab: live preset previews + colors + fonts + JSON ── */}
          <TabsContent value="theme" className="flex flex-col gap-4">
            <div className="rounded-lg border p-4">
              <Label className="text-xs text-muted-foreground">{t("pptx.theme.preset")}</Label>
              {/* Preset chips render the deck's own first slide in the preset
                  theme — WYSIWYG instead of abstract color dots. */}
              <div className="mt-2 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
                {THEME_PRESETS.map((p) => {
                  const active =
                    deck.theme.background === p.theme.background &&
                    deck.theme.accent === p.theme.accent &&
                    deck.theme.font_heading === p.theme.font_heading;
                  return (
                    <button
                      key={p.key}
                      type="button"
                      onClick={() => applyPreset(p.theme)}
                      className={cn(
                        "group overflow-hidden rounded-lg border text-left transition-[border-color,transform] duration-200 hover:-translate-y-0.5 active:scale-[0.98]",
                        active ? "border-primary" : "border-border hover:border-primary/50",
                      )}
                      title={t(`pptx.theme.preset_${p.key}`)}
                    >
                      <div className="pointer-events-none">
                        {firstSlide && <SlideView slide={firstSlide} theme={p.theme} />}
                      </div>
                      <div className="border-t px-2 py-1.5 text-xs transition-colors group-hover:text-foreground text-muted-foreground">
                        {t(`pptx.theme.preset_${p.key}`)}
                      </div>
                    </button>
                  );
                })}
              </div>

              <div className="mt-6 grid gap-x-6 gap-y-4 sm:grid-cols-2">
                {(["background", "foreground", "accent", "muted"] as const).map((key) => (
                  <div key={key} className="flex items-center gap-2">
                    <Input
                      type="color"
                      value={deck.theme[key]}
                      onChange={(e) => patchTheme({ [key]: e.target.value })}
                      className="h-11 w-14 shrink-0 cursor-pointer p-1 sm:h-9"
                      aria-label={t(`pptx.theme.${key}`)}
                    />
                    <div className="flex flex-1 flex-col gap-1">
                      <Label className="text-xs text-muted-foreground">{t(`pptx.theme.${key}`)}</Label>
                      <Input
                        value={deck.theme[key]}
                        onChange={(e) => patchTheme({ [key]: e.target.value })}
                        className="min-h-11 font-mono text-xs sm:min-h-9"
                      />
                    </div>
                  </div>
                ))}
                {(["font_heading", "font_body"] as const).map((key) => (
                  <div key={key} className="flex flex-col gap-1.5">
                    <Label className="text-xs text-muted-foreground">{t(`pptx.theme.${key}`)}</Label>
                    <Select
                      value={deck.theme[key]}
                      onValueChange={(v) => patchTheme({ [key]: v })}
                    >
                      <SelectTrigger className="min-h-11 sm:min-h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {SAFE_FONTS.map((f) => (
                          <SelectItem key={f} value={f}>
                            {f}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ))}
              </div>
            </div>

            {/* JSON mode */}
            <div className="flex flex-col gap-2 rounded-lg border p-4">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  {t("pptx.slides_count", { n: deck.slides.length })}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowJson((v) => !v)}
                  className="min-h-11 sm:min-h-9"
                >
                  {showJson ? t("pptx.json_hide") : t("pptx.json_mode")}
                </Button>
              </div>
              {showJson && (
                <div className="pptx-enter-soft flex flex-col gap-2">
                  <Textarea
                    value={jsonDraft || JSON.stringify(deck, null, 2)}
                    onChange={(e) => setJsonDraft(e.target.value)}
                    rows={16}
                    className="font-mono text-xs"
                  />
                  {jsonError && <p className="text-xs text-destructive">{jsonError}</p>}
                  <div className="flex justify-end gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={loadJson}
                      disabled={!jsonDraft.trim()}
                      className="min-h-11 sm:min-h-9"
                    >
                      {t("pptx.json_load")}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          </TabsContent>
        </Tabs>

        <div className="flex items-center gap-2">
          <Badge variant="outline" className="shrink-0 text-muted-foreground">
            <Presentation className="mr-1 h-3 w-3" />
            {t("pptx.client_side")}
          </Badge>
        </div>
      </div>

      {/* Designer column: full-height chat rail on desktop; on compact screens
          it renders itself as a portal bottom sheet, so the wrapper stays empty. */}
      <div className={cn("h-full shrink-0", !isCompact && "min-w-0")}>
        <DesignerColumn onApplyDeck={applyDeck} currentDeck={deck} />
      </div>
    </div>
    </div>
  );
}
