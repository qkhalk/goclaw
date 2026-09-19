import { useState } from "react";
import type { ComponentType, ReactNode, RefObject } from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowDown,
  ArrowRightToLine,
  ArrowUp,
  Blend,
  Braces,
  Circle,
  CirclePlay,
  CircleSlash,
  ClipboardList,
  Columns2,
  Copy,
  Download,
  Eraser,
  Frame,
  ImagePlus,
  Loader2,
  MonitorPlay,
  Notebook,
  PanelLeft,
  Plus,
  Presentation,
  Redo2,
  Scan,
  Square,
  Trash2,
  Type,
  Undo2,
  Wand2,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import {
  SAFE_FONTS,
  SLIDE_TRANSITIONS,
  THEME_PRESETS,
  type DeckTheme,
  type SlideTransition,
} from "../types";
import { MAX_ZOOM, MIN_ZOOM, type InteractiveStageHandle } from "./interactive-stage";

/**
 * PowerPoint-style ribbon header for the PPTX Studio: a title row (deck name
 * + the global Export / Present / Designer actions), a tab row (Home, Insert,
 * Design, Transitions, Slide Show, View) and the active tab's command strip —
 * groups of icon buttons separated by hairline dividers with small group
 * labels. Narrow screens collapse the strip to icon-only buttons inside a
 * horizontally scrollable row (the Home fallback).
 *
 * Every button wires to an existing handler owned by the page: slide ops and
 * theme patches are page callbacks; element ops go through the interactive
 * stage's imperative handle.
 */

export type RibbonTabId = "home" | "insert" | "design" | "transitions" | "slideshow" | "view";

export interface RibbonViewToggles {
  thumbs: boolean;
  notes: boolean;
  editor: boolean;
  json: boolean;
}

export interface RibbonProps {
  /** Row 1 */
  deckName: string;
  exporting: boolean;
  onExport: () => void;
  designerOpen: boolean;
  onToggleDesigner: () => void;
  onClearDraft: () => void;
  /** Home: slide-level ops */
  selectedIndex: number;
  slideCount: number;
  onAddSlide: () => void;
  onDuplicateSlide: () => void;
  onRemoveSlide: () => void;
  onMoveSlide: (dir: -1 | 1) => void;
  /** Home/Insert: element ops via the stage's imperative handle */
  stageRef: RefObject<InteractiveStageHandle | null>;
  elementSelected: boolean;
  history: { undo: number; redo: number };
  /** Transitions */
  transition: SlideTransition | undefined;
  onTransitionChange: (t: SlideTransition | undefined) => void;
  /** Design */
  theme: DeckTheme;
  onPatchTheme: (patch: Partial<DeckTheme>) => void;
  onApplyPreset: (theme: DeckTheme) => void;
  /** View */
  view: RibbonViewToggles;
  onViewToggle: (key: keyof RibbonViewToggles) => void;
  /** Zoom */
  zoom: number;
  onZoomIn: () => void;
  onZoomOut: () => void;
  onZoomFit: () => void;
  /** Slide Show */
  onPresentFromBeginning: () => void;
  onPresentFromCurrent: () => void;
}

type IconType = ComponentType<{ className?: string }>;

export function Ribbon(props: RibbonProps) {
  const { t } = useTranslation("toolbox");
  const [tab, setTab] = useState<RibbonTabId>("home");

  const tabs: { id: RibbonTabId; label: string }[] = [
    { id: "home", label: t("pptx.ribbon.tab_home") },
    { id: "insert", label: t("pptx.ribbon.tab_insert") },
    { id: "design", label: t("pptx.ribbon.tab_design") },
    { id: "transitions", label: t("pptx.ribbon.tab_transitions") },
    { id: "slideshow", label: t("pptx.ribbon.tab_slideshow") },
    { id: "view", label: t("pptx.ribbon.tab_view") },
  ];

  return (
    <header className="shrink-0 border-b bg-card/40">
      {/* Row 1 — title + global actions */}
      <div className="flex h-12 items-center gap-2 px-2 sm:px-3">
        <Presentation className="h-4 w-4 shrink-0 text-primary" aria-hidden />
        <div className="flex min-w-0 flex-col leading-tight">
          <span className="truncate text-sm font-semibold">{t("pptx.title")}</span>
          <span className="max-w-44 truncate text-[11px] text-muted-foreground sm:max-w-72">
            {props.deckName || t("pptx.title")}
          </span>
        </div>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={props.onClearDraft}
            title={t("pptx.draft_clear_hint")}
            aria-label={t("pptx.draft_clear")}
            className="min-h-11 min-w-11 sm:h-8 sm:w-8"
          >
            <Eraser className="h-4 w-4" />
          </Button>
          <Button
            variant={props.designerOpen ? "default" : "outline"}
            size="sm"
            onClick={props.onToggleDesigner}
            className="min-h-11 sm:min-h-8"
            title={t("pptx.designer.toggle")}
          >
            <Wand2 className="h-4 w-4 sm:mr-1.5" />
            <span className="hidden sm:inline">{t("pptx.designer.title")}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={props.onPresentFromCurrent}
            className="min-h-11 sm:min-h-8"
            title={t("pptx.ribbon.from_current")}
          >
            <MonitorPlay className="h-4 w-4 sm:mr-1.5" />
            <span className="hidden sm:inline">{t("pptx.present")}</span>
          </Button>
          <Button
            size="sm"
            onClick={props.onExport}
            disabled={props.exporting || props.slideCount === 0}
            className="min-h-11 sm:min-h-8"
          >
            {props.exporting ? (
              <Loader2 className="h-4 w-4 animate-spin sm:mr-1.5" />
            ) : (
              <Download className="h-4 w-4 sm:mr-1.5" />
            )}
            <span className="hidden sm:inline">
              {props.exporting ? t("pptx.exporting") : t("pptx.export")}
            </span>
          </Button>
        </div>
      </div>

      {/* Row 2 — ribbon tabs */}
      <div className="overflow-x-auto overscroll-contain">
        <div role="tablist" aria-label={t("pptx.title")} className="flex w-max min-w-full items-stretch gap-0.5 px-1.5">
          {tabs.map((tb) => (
            <button
              key={tb.id}
              role="tab"
              type="button"
              aria-selected={tab === tb.id}
              onClick={() => setTab(tb.id)}
              className={cn(
                "min-h-11 whitespace-nowrap border-b-2 px-3 text-sm transition-colors sm:min-h-9",
                tab === tb.id
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {tb.label}
            </button>
          ))}
        </div>
      </div>

      {/* Row 3 — command strip of the active tab */}
      <div className="overflow-x-auto overscroll-contain">
        <div className="flex h-[72px] w-max min-w-full items-stretch px-1">
          {tab === "home" && <HomeStrip {...props} />}
          {tab === "insert" && <InsertStrip {...props} />}
          {tab === "design" && <DesignStrip {...props} />}
          {tab === "transitions" && <TransitionsStrip {...props} />}
          {tab === "slideshow" && <SlideShowStrip {...props} />}
          {tab === "view" && <ViewStrip {...props} />}
        </div>
      </div>
    </header>
  );
}

/* ── Building blocks ── */

/** Command group: buttons on top, small label under, hairline divider after. */
function Group({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <div className="flex flex-col items-center justify-between gap-1 px-1 py-1.5 sm:px-2">
        <div className="flex flex-1 items-center gap-0.5">{children}</div>
        <span className="hidden truncate text-[10px] leading-none text-muted-foreground/70 sm:block">
          {label}
        </span>
      </div>
      <Separator orientation="vertical" className="my-2" />
    </>
  );
}

/** PowerPoint-style ribbon button: icon over a tiny label. */
function RibbonButton({
  icon: Icon,
  label,
  onClick,
  disabled,
  active,
  danger,
  className,
}: {
  icon: IconType;
  label: string;
  onClick: () => void;
  disabled?: boolean;
  active?: boolean;
  danger?: boolean;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      aria-pressed={active}
      className={cn(
        "flex h-11 w-[52px] shrink-0 flex-col items-center justify-center gap-0.5 rounded-md px-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-40 sm:h-[46px] sm:w-14",
        active && "bg-accent text-accent-foreground",
        danger && "hover:bg-destructive/10 hover:text-destructive",
        className,
      )}
    >
      <Icon className="h-4 w-4 shrink-0" aria-hidden />
      <span className="w-full truncate text-center text-[9px] leading-tight sm:text-[10px]">{label}</span>
    </button>
  );
}

/** Image insert: same look, but a label wrapping a hidden file input. */
function RibbonImageButton({
  label,
  onFile,
}: {
  label: string;
  onFile: (file: File | undefined) => void;
}) {
  return (
    <label
      title={label}
      className="flex h-11 w-[52px] shrink-0 cursor-pointer flex-col items-center justify-center gap-0.5 rounded-md px-1 text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground sm:h-[46px] sm:w-14"
    >
      <ImagePlus className="h-4 w-4 shrink-0" aria-hidden />
      <span className="w-full truncate text-center text-[9px] leading-tight sm:text-[10px]">{label}</span>
      <input
        type="file"
        accept="image/*"
        className="sr-only"
        aria-label={label}
        onChange={(e) => {
          onFile(e.target.files?.[0]);
          e.target.value = "";
        }}
      />
    </label>
  );
}

/** Theme color swatch — click opens the native color picker. */
function RibbonColor({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <label
      title={label}
      className="flex h-11 w-[52px] shrink-0 cursor-pointer flex-col items-center justify-center gap-1 rounded-md px-1 transition-colors hover:bg-accent sm:h-[46px] sm:w-14"
    >
      <span className="h-4 w-7 rounded-sm border border-border" style={{ backgroundColor: value }} aria-hidden />
      <span className="w-full truncate text-center text-[9px] leading-tight text-muted-foreground sm:text-[10px]">
        {label}
      </span>
      <input
        type="color"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="sr-only"
        aria-label={label}
      />
    </label>
  );
}

/** Theme preset chip rendered from the preset's own colors. */
function RibbonPreset({
  label,
  theme,
  active,
  onClick,
}: {
  label: string;
  theme: DeckTheme;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      aria-label={label}
      aria-pressed={active}
      className={cn(
        "flex h-11 w-[52px] shrink-0 flex-col items-center justify-center gap-1 rounded-md px-1 transition-colors hover:bg-accent sm:h-[46px] sm:w-14",
        active && "bg-accent",
      )}
    >
      <span className="flex h-4 w-7 overflow-hidden rounded-sm border border-border" aria-hidden>
        <span className="h-full flex-1" style={{ backgroundColor: theme.background }} />
        <span className="h-full flex-1" style={{ backgroundColor: theme.accent }} />
        <span className="h-full flex-1" style={{ backgroundColor: theme.foreground }} />
      </span>
      <span className="w-full truncate text-center text-[9px] leading-tight text-muted-foreground sm:text-[10px]">
        {label}
      </span>
    </button>
  );
}

/* ── Tab strips ── */

function HomeStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  const stage = p.stageRef;
  const arrangeReady = p.elementSelected;
  return (
    <>
      <Group label={t("pptx.ribbon.group_quick")}>
        <RibbonButton
          icon={Undo2}
          label={t("pptx.stage.undo")}
          disabled={p.history.undo === 0}
          onClick={() => stage.current?.undo()}
        />
        <RibbonButton
          icon={Redo2}
          label={t("pptx.stage.redo")}
          disabled={p.history.redo === 0}
          onClick={() => stage.current?.redo()}
        />
      </Group>
      <Group label={t("pptx.ribbon.group_slides")}>
        <RibbonButton
          icon={Plus}
          label={t("pptx.add_slide")}
          disabled={p.slideCount >= 40}
          onClick={p.onAddSlide}
        />
        <RibbonButton icon={Copy} label={t("pptx.duplicate")} onClick={p.onDuplicateSlide} />
        <RibbonButton
          icon={ArrowUp}
          label={t("pptx.move_up")}
          disabled={p.selectedIndex === 0}
          onClick={() => p.onMoveSlide(-1)}
        />
        <RibbonButton
          icon={ArrowDown}
          label={t("pptx.move_down")}
          disabled={p.selectedIndex >= p.slideCount - 1}
          onClick={() => p.onMoveSlide(1)}
        />
        <RibbonButton
          icon={Trash2}
          label={t("pptx.remove")}
          disabled={p.slideCount <= 1}
          danger
          onClick={p.onRemoveSlide}
        />
      </Group>
      <Group label={t("pptx.ribbon.group_elements")}>
        <RibbonButton
          icon={Type}
          label={t("pptx.stage.add_text")}
          onClick={() => stage.current?.addElement({ kind: "text" })}
        />
        <RibbonButton
          icon={Square}
          label={t("pptx.stage.add_shape")}
          onClick={() => stage.current?.addElement({ kind: "rect" })}
        />
        <RibbonImageButton label={t("pptx.stage.add_image")} onFile={(f) => stage.current?.addImageFile(f)} />
      </Group>
      <Group label={t("pptx.ribbon.group_arrange")}>
        <RibbonButton
          icon={ArrowUp}
          label={t("pptx.stage.z_up")}
          disabled={!arrangeReady}
          onClick={() => stage.current?.moveZ(1)}
        />
        <RibbonButton
          icon={ArrowDown}
          label={t("pptx.stage.z_down")}
          disabled={!arrangeReady}
          onClick={() => stage.current?.moveZ(-1)}
        />
        <RibbonButton
          icon={Copy}
          label={t("pptx.duplicate")}
          disabled={!arrangeReady}
          onClick={() => stage.current?.duplicateElement()}
        />
        <RibbonButton
          icon={Trash2}
          label={t("pptx.stage.delete")}
          disabled={!arrangeReady}
          danger
          onClick={() => stage.current?.removeElement()}
        />
      </Group>
    </>
  );
}

function InsertStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  const stage = p.stageRef;
  return (
    <>
      <Group label={t("pptx.ribbon.group_slides")}>
        <RibbonButton
          icon={Plus}
          label={t("pptx.add_slide")}
          disabled={p.slideCount >= 40}
          onClick={p.onAddSlide}
        />
      </Group>
      <Group label={t("pptx.ribbon.group_elements")}>
        <RibbonButton
          icon={Type}
          label={t("pptx.stage.add_text")}
          onClick={() => stage.current?.addElement({ kind: "text" })}
        />
        <RibbonButton
          icon={Square}
          label={t("pptx.stage.add_shape")}
          onClick={() => stage.current?.addElement({ kind: "rect" })}
        />
        <RibbonButton
          icon={Circle}
          label={t("pptx.stage.kind_ellipse")}
          onClick={() => stage.current?.addElement({ kind: "ellipse" })}
        />
        <RibbonButton
          icon={Frame}
          label={t("pptx.stage.kind_frame")}
          onClick={() => stage.current?.addElement({ kind: "frame" })}
        />
        <RibbonImageButton label={t("pptx.stage.add_image")} onFile={(f) => stage.current?.addImageFile(f)} />
      </Group>
    </>
  );
}

function DesignStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  const activePreset = THEME_PRESETS.find(
    (preset) =>
      p.theme.background === preset.theme.background &&
      p.theme.accent === preset.theme.accent &&
      p.theme.font_heading === preset.theme.font_heading,
  );
  return (
    <>
      <Group label={t("pptx.ribbon.group_themes")}>
        {THEME_PRESETS.map((preset) => (
          <RibbonPreset
            key={preset.key}
            label={t(`pptx.theme.preset_${preset.key}`)}
            theme={preset.theme}
            active={activePreset?.key === preset.key}
            onClick={() => p.onApplyPreset(preset.theme)}
          />
        ))}
      </Group>
      <Group label={t("pptx.ribbon.group_colors")}>
        {(["background", "foreground", "accent", "muted"] as const).map((key) => (
          <RibbonColor
            key={key}
            label={t(`pptx.theme.${key}`)}
            value={p.theme[key]}
            onChange={(v) => p.onPatchTheme({ [key]: v })}
          />
        ))}
      </Group>
      <Group label={t("pptx.ribbon.group_fonts")}>
        <div className="flex items-center gap-1 px-0.5">
          <Select value={p.theme.font_heading} onValueChange={(v) => p.onPatchTheme({ font_heading: v })}>
            <SelectTrigger
              aria-label={t("pptx.theme.font_heading")}
              className="h-11 w-24 text-base md:h-9 md:w-28 md:text-sm"
            >
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
          <Select value={p.theme.font_body} onValueChange={(v) => p.onPatchTheme({ font_body: v })}>
            <SelectTrigger
              aria-label={t("pptx.theme.font_body")}
              className="h-11 w-24 text-base md:h-9 md:w-28 md:text-sm"
            >
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
      </Group>
    </>
  );
}

const TRANSITION_ICONS: Record<SlideTransition, IconType> = {
  none: CircleSlash,
  fade: Blend,
  push: ArrowRightToLine,
  wipe: Columns2,
  zoom: ZoomIn,
};

function TransitionsStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  const current = p.transition ?? "none";
  return (
    <Group label={t("pptx.ribbon.group_transition")}>
      {SLIDE_TRANSITIONS.map((tr) => (
        <RibbonButton
          key={tr}
          icon={TRANSITION_ICONS[tr]!}
          label={t(`pptx.transition_${tr}`)}
          active={current === tr}
          onClick={() => p.onTransitionChange(tr === "none" ? undefined : tr)}
        />
      ))}
    </Group>
  );
}

function SlideShowStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  return (
    <Group label={t("pptx.ribbon.group_slideshow")}>
      <RibbonButton icon={MonitorPlay} label={t("pptx.ribbon.from_beginning")} onClick={p.onPresentFromBeginning} />
      <RibbonButton icon={CirclePlay} label={t("pptx.ribbon.from_current")} onClick={p.onPresentFromCurrent} />
    </Group>
  );
}

function ViewStrip(p: RibbonProps) {
  const { t } = useTranslation("toolbox");
  return (
    <>
      <Group label={t("pptx.ribbon.group_zoom")}>
        <RibbonButton
          icon={ZoomOut}
          label={t("pptx.ribbon.zoom_out")}
          disabled={p.zoom <= MIN_ZOOM}
          onClick={p.onZoomOut}
        />
        <RibbonButton
          icon={ZoomIn}
          label={t("pptx.ribbon.zoom_in")}
          disabled={p.zoom >= MAX_ZOOM}
          onClick={p.onZoomIn}
        />
        <RibbonButton
          icon={Scan}
          label={t("pptx.ribbon.fit")}
          active={p.zoom === 1}
          onClick={p.onZoomFit}
        />
      </Group>
      <Group label={t("pptx.ribbon.group_panels")}>
        <RibbonButton
          icon={PanelLeft}
          label={t("pptx.ribbon.thumbnails")}
          active={p.view.thumbs}
          onClick={() => p.onViewToggle("thumbs")}
        />
        <RibbonButton
          icon={Notebook}
          label={t("pptx.ribbon.notes")}
          active={p.view.notes}
          onClick={() => p.onViewToggle("notes")}
        />
        <RibbonButton
          icon={ClipboardList}
          label={t("pptx.ribbon.editor")}
          active={p.view.editor}
          onClick={() => p.onViewToggle("editor")}
        />
        <RibbonButton
          icon={Braces}
          label={t("pptx.json_mode")}
          active={p.view.json}
          onClick={() => p.onViewToggle("json")}
        />
      </Group>
    </>
  );
}
