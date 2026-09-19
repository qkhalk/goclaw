import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { toast } from "@/stores/use-toast-store";
import { cn } from "@/lib/utils";
import { useIsTablet } from "@/hooks/use-media-query";
import { useUiStore } from "@/stores/use-ui-store";
import {
  blankSlide,
  defaultDeck,
  type Deck,
  type DeckTheme,
  type Slide,
  type SlideLayout,
  type SlideTransition,
} from "./types";
import { exportDeckPptx } from "./lib/pptx-export";
import { parseDeck } from "./lib/parse-deck-blocks";
import { InteractiveStage, type InteractiveStageHandle } from "./components/interactive-stage";
import { SlideEditor } from "./components/slide-editor";
import { Presentation } from "./components/presentation";
import { DesignerColumn } from "./components/designer-column";
import { Ribbon, type RibbonViewToggles } from "./components/ribbon";
import { SlideThumbnails } from "./components/slide-thumbnails";
import { StatusBar } from "./components/status-bar";

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

/** localStorage draft key — the deck survives tab/app reloads. */
const DRAFT_KEY = "goclaw:pptx-draft:v1";

/**
 * PPTX Studio, presented as a PowerPoint-like shell: ribbon header on top,
 * slide thumbnail rail on the left, the interactive canvas in the middle on
 * a neutral surface, the designer chat as a collapsible right column, and a
 * status bar (slide position + notes toggle + zoom) at the bottom.
 */
export function PptxToolPage() {
  const { t } = useTranslation("toolbox");

  const [deck, setDeck] = useState<Deck>(defaultDeck);
  const [selected, setSelected] = useState(0);
  const [exporting, setExporting] = useState(false);
  const [jsonDraft, setJsonDraft] = useState("");
  const [jsonError, setJsonError] = useState("");
  const [presenting, setPresenting] = useState(false);

  // Panel visibility (View ribbon + status bar toggles). The slide editor
  // form defaults open on real desktops, collapsed where space is tight.
  const [view, setView] = useState<RibbonViewToggles>(() => ({
    thumbs: true,
    notes: false,
    editor: typeof window !== "undefined" && window.matchMedia("(min-width: 1024px)").matches,
    json: false,
  }));

  // Canvas zoom: multiplier over the fit scale (1 = fit). The stage reports
  // the effective fraction back for the status bar percentage.
  const [zoom, setZoom] = useState(1);
  const [scalePct, setScalePct] = useState(100);
  const [elementSelected, setElementSelected] = useState(false);
  const [history, setHistory] = useState({ undo: 0, redo: 0 });
  const stageRef = useRef<InteractiveStageHandle | null>(null);

  // Designer column (chat rail on desktop, bottom sheet on tablets/phones)
  const isCompact = useIsTablet();
  const designerOpen = useUiStore((s) => s.pptxDesignerOpen);
  const setDesignerOpen = useUiStore((s) => s.setPptxDesignerOpen);

  // Draft persistence: restore once on mount, save debounced on every edit.
  const draftRestoredRef = useRef(false);
  useEffect(() => {
    if (draftRestoredRef.current) return;
    draftRestoredRef.current = true;
    try {
      const raw = localStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      const parsed = parseDeck(raw);
      if (!parsed.ok) return;
      setDeck(parsed.deck);
      toast.success(t("pptx.draft_restored"));
    } catch {
      // Corrupted draft — start fresh rather than blocking the page.
    }
  }, []);

  useEffect(() => {
    const id = setTimeout(() => {
      try {
        localStorage.setItem(DRAFT_KEY, JSON.stringify(deck));
      } catch {
        // Storage full/unavailable — drafts are best-effort.
      }
    }, 800);
    return () => clearTimeout(id);
  }, [deck]);

  function clearDraft() {
    localStorage.removeItem(DRAFT_KEY);
    setDeck(defaultDeck());
    setSelected(0);
    toast.success(t("pptx.draft_cleared"));
  }

  const current = deck.slides[selected];

  const deckName = useMemo(() => {
    const titleSlide = deck.slides.find((s) => s.layout === "title") ?? deck.slides[0];
    return (titleSlide?.title ?? "").trim();
  }, [deck.slides]);

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

  const changeTransition = (transition: SlideTransition | undefined) => {
    setDeck((d) => ({
      ...d,
      slides: d.slides.map((s, i) => (i === selected ? { ...s, transition } : s)),
    }));
  };

  // The one Apply path shared by the JSON mode and the designer column, so
  // the preview, rail and export payload stay consistent. Designer fences
  // stay v1 (layout-driven); JSON mode may carry v2 (elements/transition).
  const applyDeck = (next: Deck) => {
    setDeck({ ...next, version: next.version === 2 ? 2 : 1 });
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
    setView((v) => ({ ...v, json: false }));
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

  // ── View / zoom wiring (stable callbacks: the stage mirrors state through
  // them on every render, and the setters bail out on unchanged values) ──
  const toggleView = useCallback((key: keyof RibbonViewToggles) => {
    setView((v) => ({ ...v, [key]: !v[key] }));
  }, []);

  const handleScale = useCallback((fraction: number) => {
    setScalePct(Math.round(fraction * 100));
  }, []);

  const handleSelection = useCallback((has: boolean) => {
    setElementSelected(has);
  }, []);

  const handleHistory = useCallback((h: { undo: number; redo: number }) => {
    setHistory((prev) => (prev.undo === h.undo && prev.redo === h.redo ? prev : h));
  }, []);

  const zoomIn = useCallback(() => {
    setZoom((z) => Math.min(2, Math.round((z + 0.1) * 10) / 10));
  }, []);

  const zoomOut = useCallback(() => {
    setZoom((z) => Math.max(0.5, Math.round((z - 0.1) * 10) / 10));
  }, []);

  const zoomFit = useCallback(() => setZoom(1), []);

  // Slide Show: the presentation overlay starts at the page's selected index,
  // so "from current" just opens it and "from beginning" resets the index.
  const presentFromCurrent = useCallback(() => setPresenting(true), []);
  const presentFromBeginning = useCallback(() => {
    setSelected(0);
    setPresenting(true);
  }, []);

  return (
    // PowerPoint-style fixed shell: the page fills the app scrollport exactly
    // (h-full of <main>) — ribbon and status bar are fixed rows, the middle
    // row shares the rest, and every region scrolls internally.
    <div className="flex h-full min-h-0 flex-col">
      <Ribbon
        deckName={deckName}
        exporting={exporting}
        onExport={handleExport}
        designerOpen={designerOpen}
        onToggleDesigner={() => setDesignerOpen(!designerOpen)}
        onClearDraft={clearDraft}
        selectedIndex={selected}
        slideCount={deck.slides.length}
        onAddSlide={addSlide}
        onDuplicateSlide={duplicateSlide}
        onRemoveSlide={removeSlide}
        onMoveSlide={moveSlide}
        stageRef={stageRef}
        elementSelected={elementSelected}
        history={history}
        transition={current?.transition}
        onTransitionChange={changeTransition}
        theme={deck.theme}
        onPatchTheme={patchTheme}
        onApplyPreset={applyPreset}
        view={view}
        onViewToggle={toggleView}
        zoom={zoom}
        onZoomIn={zoomIn}
        onZoomOut={zoomOut}
        onZoomFit={zoomFit}
        onPresentFromBeginning={presentFromBeginning}
        onPresentFromCurrent={presentFromCurrent}
      />

      {/* JSON mode: raw deck import/export for power users */}
      {view.json && (
        <div className="flex shrink-0 flex-col gap-2 border-b px-3 py-2">
          <Textarea
            rows={6}
            value={jsonDraft || JSON.stringify(deck, null, 2)}
            onChange={(e) => setJsonDraft(e.target.value)}
            className="font-mono text-base md:text-xs"
            aria-label={t("pptx.json_mode")}
          />
          {jsonError && <p className="text-xs text-destructive">{jsonError}</p>}
          <div className="flex justify-end">
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

      <div className="flex min-h-0 flex-1">
        <SlideThumbnails
          deck={deck}
          selected={selected}
          onSelect={setSelected}
          onAdd={addSlide}
          open={view.thumbs}
        />

        {/* Center: canvas + optional notes/editor panels */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col px-2 pb-1 pt-2 sm:px-3">
          {current && (
            // key={selected}: soft crossfade + fresh stage state per slide
            <div key={selected} className="pptx-enter-soft min-h-0 flex-1">
              <InteractiveStage
                ref={stageRef}
                slide={current}
                theme={deck.theme}
                zoom={zoom}
                onScaleChange={handleScale}
                onSelectionChange={handleSelection}
                onHistoryChange={handleHistory}
                onChange={(next) =>
                  setDeck((d) => ({
                    ...d,
                    slides: d.slides.map((s, i) => (i === selected ? next : s)),
                  }))
                }
              />
            </div>
          )}

          {view.notes && current && (
            <div className="mt-1.5 shrink-0 rounded-md border bg-card/30 px-3 py-2">
              <Label className="text-xs text-muted-foreground">{t("pptx.field.notes")}</Label>
              <Textarea
                rows={2}
                value={current.notes ?? ""}
                onChange={(e) => patchSlide({ notes: e.target.value })}
                className="mt-1 text-base md:text-sm"
                placeholder={t("pptx.present_no_notes")}
              />
            </div>
          )}

          {view.editor && current && (
            <div
              key={`editor-${selected}`}
              className="mt-1.5 max-h-[45%] shrink-0 overflow-y-auto overscroll-contain rounded-md border bg-card/30"
            >
              <div className="pptx-enter-soft p-3 sm:p-4">
                <SlideEditor
                  slide={current}
                  index={selected}
                  total={deck.slides.length}
                  onChange={patchSlide}
                  onLayoutChange={changeLayout}
                />
              </div>
            </div>
          )}
        </div>

        {/* Designer column: full-height chat rail on desktop; on compact
            screens it renders itself as a portal bottom sheet, so the wrapper
            stays empty. */}
        <div className={cn("h-full min-h-0 shrink-0", !isCompact && "min-w-0")}>
          <DesignerColumn onApplyDeck={applyDeck} currentDeck={deck} />
        </div>
      </div>

      <StatusBar
        index={selected}
        total={deck.slides.length}
        notesOpen={view.notes}
        onToggleNotes={() => toggleView("notes")}
        thumbsOpen={view.thumbs}
        onToggleThumbs={() => toggleView("thumbs")}
        zoom={zoom}
        onZoomChange={setZoom}
        scalePct={scalePct}
      />

      {/* Fullscreen presentation overlay */}
      {presenting && (
        <Presentation
          deck={deck}
          index={selected}
          onIndexChange={setSelected}
          onExit={() => setPresenting(false)}
        />
      )}
    </div>
  );
}
