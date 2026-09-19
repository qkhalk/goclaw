import type { Layer, Scene } from "../hooks/use-timeline";
import { layerWindow } from "../hooks/use-timeline";
import { getIconImage } from "../lib/feather-icons";
import { renderSceneWithTransition, type SceneTransition } from "./scene-transition";

// ── Shared storyboard frame renderer ──
//
// Single source of truth for painting one storyboard frame, used by BOTH the
// canvas player (preview) and the client-side MediaRecorder export, so what
// you preview is what exports. Handles: color scenes (animated gradient +
// optional blueprint grid, mirroring the worker's lavfi gradients/drawgrid),
// image scenes, Ken Burns, OpenCut-style per-scene transform
// (scale/offset/rotate/opacity) + filters (brightness/contrast/saturate/blur),
// captions (with karaoke word-reveal synced to the narration audio), and
// enter transitions.

export function wrapText(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string[] {
  const words = text.split(/\s+/);
  const lines: string[] = [];
  let current = "";
  for (const word of words) {
    const test = current ? `${current} ${word}` : word;
    if (ctx.measureText(test).width > maxWidth && current) {
      lines.push(current);
      current = word;
    } else {
      current = test;
    }
  }
  if (current) lines.push(current);
  return lines.length > 0 ? lines : [""];
}

/** CSS filter string for the scene's color grading, "" when untouched. */
function sceneFilter(f: Scene["filter"]): string {
  if (!f) return "";
  const parts: string[] = [];
  if (f.brightness !== undefined && f.brightness !== 1) parts.push(`brightness(${f.brightness})`);
  if (f.contrast !== undefined && f.contrast !== 1) parts.push(`contrast(${f.contrast})`);
  if (f.saturate !== undefined && f.saturate !== 1) parts.push(`saturate(${f.saturate})`);
  if (f.blur !== undefined && f.blur > 0) parts.push(`blur(${f.blur}px)`);
  return parts.join(" ");
}

/** Halve each channel — the TS twin of the worker's darkerHex, so the
 * preview's default second stop matches the server gradient exactly. */
function darkerHex(hex: string): string {
  const h = hex.replace("#", "");
  if (h.length !== 6) return hex;
  let out = "";
  for (let i = 0; i < 3; i++) {
    const v = Math.floor(parseInt(h.slice(i * 2, i * 2 + 2), 16) / 2);
    out += v.toString(16).padStart(2, "0");
  }
  return `#${out}`;
}

// ── Style packs (multi-form engine) ──
// Canonical pack table — mirrored by stylePacks in vworker/motion_layers.go.
// A pack expands to the scene's color/color2/grid (color scenes only) and
// glow (all scenes), plus the default text/accent colors of layers that
// don't set fill. Keep the two tables in lockstep or preview drifts.
export interface StylePackSpec {
  color: string;
  color2: string;
  grid: boolean;
  glow: string; // "" = no glow (paper_light)
  text: string; // default text color
  accent: string; // default layer accent color
}

export const STYLE_PACKS: Record<string, StylePackSpec> = {
  tech_dark: { color: "#0D1117", color2: "#1E293B", grid: true, glow: "#38BDF8", text: "#F8FAFC", accent: "#38BDF8" },
  neon_lab: { color: "#07070B", color2: "#1E1B4B", grid: false, glow: "#22D3EE", text: "#F4F4F5", accent: "#22D3EE" },
  // paper_light has no glow — radial orbs wash out on light backdrops.
  paper_light: { color: "#F8FAFC", color2: "#E2E8F0", grid: true, glow: "", text: "#0F172A", accent: "#2563EB" },
  bold_red: { color: "#140404", color2: "#7F1D1D", grid: false, glow: "#EF4444", text: "#FFFFFF", accent: "#F87171" },
};

/** Per-scene style values after pack expansion — mirror of the worker's
 * resolveStylePack + sceneTextColor + sceneAccent. Explicit scene fields
 * always win over the pack; the grid flag merges as "pack default unless
 * the scene turned it on" (bools carry no explicit-off, same as the wire).
 * color2 "" means "darker shade of color" (the caller derives it). */
export interface ResolvedPack {
  color: string;
  color2: string;
  grid: boolean;
  glow: string;
  text: string; // "" = plain white fallback
  accent: string; // "" = none
}

export function resolveStylePack(scene: Scene): ResolvedPack {
  const pack = scene.style_pack ? STYLE_PACKS[scene.style_pack] : undefined;
  const resolved: ResolvedPack = {
    color: scene.color || "#000000",
    color2: scene.color2?.trim() || "",
    grid: scene.grid ?? false,
    glow: scene.glow || "",
    text: "",
    accent: "",
  };
  if (!pack) return resolved;
  if (scene.type === "color") {
    if (!scene.color) resolved.color = pack.color;
    if (!resolved.color2) resolved.color2 = pack.color2;
    if (!resolved.grid) resolved.grid = pack.grid;
  }
  if (!resolved.glow) resolved.glow = pack.glow;
  resolved.text = pack.text;
  resolved.accent = pack.accent;
  return resolved;
}

/** Layer primary color: explicit fill, then the pack accent, then fallback —
 * mirror of packFillOr in vworker/motion_layers.go. */
function packFillOr(resolved: ResolvedPack, layer: Layer, fallback: string): string {
  if (layer.fill) return layer.fill;
  if (resolved.accent) return resolved.accent;
  return fallback;
}

// ── Effective layer defaults — TS twins of contract.Layer's Effective*
// methods (geometry, style, and the kind-specific clamps) ──

/** EffectiveCols — toggle_grid grid width (default 3, cap 4). */
export function effectiveCols(l: Layer): number {
  return l.cols !== undefined && l.cols >= 1 ? Math.min(l.cols, 4) : 3;
}

/** EffectiveRows — toggle_grid grid height (default 3, cap 4). */
export function effectiveRows(l: Layer): number {
  return l.rows !== undefined && l.rows >= 1 ? Math.min(l.rows, 4) : 3;
}

/** EffectiveCadence — toggle flip period (default 0.6s, clamped 0.2..2). */
export function effectiveCadence(l: Layer): number {
  if (l.cadence === undefined || l.cadence <= 0) return 0.6;
  return Math.min(Math.max(l.cadence, 0.2), 2);
}

/** EffectiveN — stack slab count (default 3, cap 6). */
export function effectiveN(l: Layer): number {
  return l.n !== undefined && l.n >= 1 ? Math.min(l.n, 6) : 3;
}

/** EffectiveWidthA — compare_bars bar A width (default 0.62, cap 1). */
export function effectiveWidthA(l: Layer): number {
  return l.width_a !== undefined && l.width_a > 0 ? Math.min(l.width_a, 1) : 0.62;
}

/** EffectiveWidthB — compare_bars bar B width (default 0.38, cap 1). */
export function effectiveWidthB(l: Layer): number {
  return l.width_b !== undefined && l.width_b > 0 ? Math.min(l.width_b, 1) : 0.38;
}

/** EffectiveBox — geometry defaults (x/y 0.1, w 0.8) plus the kind-specific
 * h defaults that keep the motion primitives sensible bare. */
function effectiveBox(l: Layer): { x: number; y: number; w: number; h: number } {
  let x = l.x ?? 0;
  let y = l.y ?? 0;
  let w = l.w ?? 0;
  let h = l.h ?? 0;
  if (x === 0) x = 0.1;
  if (y === 0) y = l.kind === "cta" ? 0.8 : 0.1;
  if (w === 0) w = 0.8;
  if (h === 0) {
    switch (l.kind) {
      case "shape":
      case "card":
        h = 0.3;
        break;
      case "toggle_grid":
        h = w * (effectiveRows(l) / effectiveCols(l));
        break;
      case "compare_bars":
        h = 0.24;
        break;
      case "stack":
        h = 0.44;
        break;
      case "stamp":
        h = w * 0.42;
        break;
      case "cta":
        h = 0.12;
        break;
      case "icon":
        h = w;
        break;
    }
  }
  return { x, y, w, h };
}

/** Shared font string for layer text (plain text, counter, bars/stack
 * labels) — mirrors the worker's per-layer face selection (layerFontFor). */
function layerFontString(layer: Layer, fontSize: number): string {
  const weight = layer.font === "display" ? "700 " : layer.font === "mono" ? "500 " : "600 ";
  const family =
    layer.font === "display" ? CAPTION_DISPLAY_FONT : layer.font === "mono" ? CAPTION_MONO_FONT : LAYER_TEXT_FONT;
  return `${weight}${fontSize}px ${family}`;
}

/** Bold face for stamp/cta text — the worker rasterizes both with the bundled
 * bold face (boldFace in prepareMotionAssets). */
function boldFontString(fontSize: number): string {
  return `700 ${fontSize}px ${CAPTION_DISPLAY_FONT}`;
}

/** Parse #RRGGBB into rgb ints — null when invalid (mirrors hexRGB's error). */
function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
  const h = hex?.replace("#", "") ?? "";
  if (!/^[0-9a-fA-F]{6}$/.test(h)) return null;
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
  };
}

/** Animated two-stop gradient backdrop for color scenes — mirrors the
 * worker's `gradients=c0:c1:speed=0.008` lavfi source: a linear gradient
 * whose axis slowly rotates over the scene. Colors come pre-resolved from
 * resolveStylePack (scene fields win, pack fills the gaps). */
function drawColorBackdrop(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  resolved: ResolvedPack,
  localTime: number,
) {
  const c0 = resolved.color;
  const c1 = resolved.color2 || darkerHex(c0);
  const angle = (localTime * 0.008 * Math.PI) % Math.PI;
  const r = Math.hypot(width, height) / 2;
  const dx = Math.cos(angle) * r;
  const dy = Math.sin(angle) * r;
  const grad = ctx.createLinearGradient(width / 2 - dx, height / 2 - dy, width / 2 + dx, height / 2 + dy);
  grad.addColorStop(0, c0);
  grad.addColorStop(1, c1);
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, width, height);

  if (resolved.grid) {
    const cellW = Math.max(1, width / 24);
    const cellH = Math.max(1, height / 24);
    ctx.save();
    ctx.strokeStyle = "rgba(148,163,184,0.10)";
    ctx.lineWidth = Math.max(1, width / 1280);
    ctx.beginPath();
    for (let x = cellW; x < width; x += cellW) {
      ctx.moveTo(x, 0);
      ctx.lineTo(x, height);
    }
    for (let y = cellH; y < height; y += cellH) {
      ctx.moveTo(0, y);
      ctx.lineTo(width, y);
    }
    ctx.stroke();
    ctx.restore();
  }
}

export function renderSceneBase(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  narrProgress?: number,
  fontScale: number = 1,
) {
  const { width, height } = canvas;
  ctx.clearRect(0, 0, width, height);

  // Style pack expansion — the worker resolves the pack once per scene
  // before any layer filter runs (resolveStylePack in vworker), so every
  // painter below sees the same merged values.
  const resolved = resolveStylePack(scene);

  if (scene.type === "color") {
    drawColorBackdrop(ctx, width, height, resolved, localTime);
    if (resolved.glow) drawGlowOrbs(ctx, width, height, resolved.glow, localTime);
    if (scene.vignette) drawVignette(ctx, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache, fontScale, resolved);
    drawCaption(ctx, width, height, scene, localTime, narrProgress, fontScale);
    return;
  }

  const img = scene.source ? imageCache.get(scene.source) : undefined;
  if (!img) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    if (resolved.glow) drawGlowOrbs(ctx, width, height, resolved.glow, localTime);
    if (scene.vignette) drawVignette(ctx, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache, fontScale, resolved);
    drawCaption(ctx, width, height, scene, localTime, narrProgress, fontScale);
    return;
  }

  // Ken Burns zoom/pan
  let scale = 1;
  let offsetX = 0;
  let offsetY = 0;
  if (scene.ken_burns) {
    const progress = scene.duration_sec > 0 ? localTime / scene.duration_sec : 0;
    const kb = scene.ken_burns;
    scale = kb.zoom_from + (kb.zoom_to - kb.zoom_from) * progress;
    const panAmount = (scale - 1) * Math.min(width, height) * 0.5;
    switch (kb.pan) {
      case "left":
        offsetX = panAmount * progress;
        break;
      case "right":
        offsetX = -panAmount * progress;
        break;
      case "up":
        offsetY = panAmount * progress;
        break;
      case "down":
        offsetY = -panAmount * progress;
        break;
    }
  }

  const imgAspect = img.naturalWidth / img.naturalHeight;
  const canvasAspect = width / height;
  let drawW: number;
  let drawH: number;
  if (scene.fit === "contain") {
    if (imgAspect > canvasAspect) {
      drawW = width * scale;
      drawH = (width / imgAspect) * scale;
    } else {
      drawH = height * scale;
      drawW = (height * imgAspect) * scale;
    }
  } else {
    if (imgAspect > canvasAspect) {
      drawH = height * scale;
      drawW = (height * imgAspect) * scale;
    } else {
      drawW = width * scale;
      drawH = (width / imgAspect) * scale;
    }
  }

  const x = (width - drawW) / 2 + offsetX;
  const y = (height - drawH) / 2 + offsetY;

  // OpenCut-style per-scene transform: pivot at the canvas center so identity
  // values reproduce the plain draw. Letterbox areas stay untouched.
  const t = scene.transform;
  const filter = sceneFilter(scene.filter);
  ctx.save();
  if (filter) ctx.filter = filter;
  if (t && (t.scale !== undefined || t.x !== undefined || t.y !== undefined || t.rotate !== undefined || t.opacity !== undefined)) {
    ctx.translate(width / 2 + ((t.x ?? 0) / 100) * width, height / 2 + ((t.y ?? 0) / 100) * height);
    if (t.rotate) ctx.rotate((t.rotate * Math.PI) / 180);
    if (t.scale && t.scale !== 1) ctx.scale(t.scale, t.scale);
    if (t.opacity !== undefined) ctx.globalAlpha = Math.max(0, Math.min(1, t.opacity));
    ctx.drawImage(img, x - width / 2, y - height / 2, drawW, drawH);
  } else {
    ctx.drawImage(img, x, y, drawW, drawH);
  }
  ctx.restore();

  if (scene.vignette) drawVignette(ctx, width, height);
  if (resolved.glow) drawGlowOrbs(ctx, width, height, resolved.glow, localTime);
  drawSceneLayers(ctx, width, height, scene, localTime, imageCache, fontScale, resolved);
  drawCaption(ctx, width, height, scene, localTime, narrProgress, fontScale);
}

/** Paint the scene's timed overlay layers (under the caption), in array
 * order — mirrors the worker's ffmpeg layer filters (drawtext/drawbox/
 * overlay + entrance anims) so preview matches the server render.
 * fontScale maps render-space font pixels (the storyboard's font_size is
 * absolute on the server's output-size render, e.g. 720px wide) onto this
 * canvas — the preview canvas is container-sized, so text must shrink with
 * it or it wraps and stacks twice as large as the burn-in.
 * resolved is the pre-expanded style pack (resolveStylePack); computed here
 * when omitted. */
export function drawSceneLayers(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  fontScale: number = 1,
  resolved?: ResolvedPack,
) {
  if (!scene.layers?.length) return;
  const pack = resolved ?? resolveStylePack(scene);
  for (const layer of scene.layers) {
    const { start, end } = layerWindow(layer, scene.duration_sec);
    if (localTime < start || localTime >= end) continue;
    const x = (layer.x ?? 0.1) * width;
    const y = (layer.y ?? 0.1) * height;
    const w = (layer.w ?? 0.8) * width;
    const opacity = Math.max(0, Math.min(1, layer.opacity ?? 1));
    // Entrance animation state — the exact numbers the worker encodes into
    // its ffmpeg expressions (animSec 0.45, 6% slide, 0.35 pop overshoot).
    const p = Math.max(0, Math.min(1, (localTime - start) / 0.45));
    const ease = 1 - (1 - p) * (1 - p); // ease-out-quad
    const slideX = layer.anim === "left" ? 0.06 * width * (1 - ease) : layer.anim === "right" ? -0.06 * width * (1 - ease) : 0;
    const slideY = layer.anim === "up" ? 0.06 * height * (1 - ease) : layer.anim === "down" ? -0.06 * height * (1 - ease) : 0;
    const pop = layer.anim === "pop" ? 1 + 0.35 * (1 - ease) : 1;
    const fadeD = layer.anim === "pop" ? 0.2 : 0.3;
    const fade = layer.anim === "fade" || layer.anim === "pop" ? Math.min(1, Math.max(0, (localTime - start) / fadeD)) : 1;
    if (layer.kind === "text") {
      // Highlighted text rides the worker's PNG-overlay path — word-level
      // colored segments with the PNG machinery's entrance anims.
      if (layer.highlights && layer.highlights.length > 0) {
        drawHighlightTextLayer(ctx, width, height, layer, fontScale, slideX, slideY, pop, fade);
        continue;
      }
      const fontSize = (layer.font_size || 48) * fontScale;
      const align = layer.align || "center";
      ctx.save();
      // 0.3s fade at the layer's start — mirrors the worker's drawtext alpha
      // expression; font mirrors the worker's per-layer face selection.
      ctx.globalAlpha = opacity * fade;
      ctx.font = layerFontString(layer, fontSize);
      // Fill chain: explicit fill → pack text color → white (the worker's
      // drawtext fontcolor resolution for text layers).
      ctx.fillStyle = layer.fill || pack.text || "#FFFFFF";
      ctx.textBaseline = "top";
      ctx.textAlign = align === "left" ? "left" : align === "right" ? "right" : "center";
      // Soft halo — half-strength border + drop shadow, like the worker.
      ctx.shadowColor = "rgba(0,0,0,0.5)";
      ctx.shadowBlur = fontSize * 0.22;
      ctx.shadowOffsetY = 2;
      const tx = (align === "left" ? x : align === "right" ? x + w : x + w / 2) + slideX;
      const lines = wrapText(ctx, layer.text ?? "", w);
      const lineHeight = fontSize * 1.3;
      lines.forEach((line, li) => ctx.fillText(line, tx, y + li * lineHeight + slideY));
      ctx.restore();
    } else if (layer.kind === "shape") {
      const h = (layer.h ?? 0.3) * height;
      ctx.save();
      ctx.globalAlpha = opacity;
      ctx.fillStyle = layer.fill || "#FFFFFF";
      ctx.fillRect(x + slideX, y + slideY, w, h);
      ctx.restore();
    } else if (layer.kind === "card") {
      const h = (layer.h ?? 0.3) * height;
      const rad = Math.round((layer.radius ?? 0.018) * width);
      ctx.save();
      // Pop scales around the card center (worker scales the input, which
      // overlay then centers in the box — same visual).
      const cx = x + w / 2 + slideX;
      const cy = y + h / 2 + slideY;
      ctx.translate(cx, cy);
      ctx.scale(pop, pop);
      ctx.translate(-cx, -cy);
      ctx.globalAlpha = fade;
      ctx.fillStyle = layer.fill || "#FFFFFF";
      ctx.globalAlpha = fade * opacity;
      ctx.beginPath();
      ctx.roundRect(x + slideX, y + slideY, w, h, rad);
      ctx.fill();
      if (layer.border) {
        ctx.globalAlpha = fade * Math.min(1, opacity + 0.4);
        ctx.lineWidth = 2;
        ctx.strokeStyle = layer.fill || "#FFFFFF";
        ctx.stroke();
      }
      ctx.restore();
    } else if (layer.kind === "icon") {
      // Icon (optionally on a tinted chip tile) — glyphs come from the same
      // embedded Feather set via data-URI <img>, cached per (name, color).
      const size = Math.min(w, (layer.h ?? w) * height) || w;
      const iconImg = getIconImage(layer);
      ctx.save();
      const cx = x + w / 2 + slideX;
      const cy = y + size / 2 + slideY;
      ctx.translate(cx, cy);
      ctx.scale(pop, pop);
      ctx.translate(-cx, -cy);
      ctx.globalAlpha = fade * opacity;
      if (layer.chip) {
        ctx.fillStyle = layer.fill || "#FFFFFF";
        ctx.globalAlpha = fade * opacity * 0.16;
        ctx.beginPath();
        ctx.roundRect(x + slideX, y + slideY, size, size, size * 0.24);
        ctx.fill();
        ctx.globalAlpha = fade * opacity;
      }
      if (iconImg) {
        const inner = size * 0.58;
        ctx.drawImage(iconImg, cx - inner / 2, cy - inner / 2, inner, inner);
      }
      ctx.restore();
    } else if (layer.kind === "image") {
      const img = layer.source ? imageCache.get(layer.source) : undefined;
      if (!img) continue;
      ctx.save();
      ctx.globalAlpha = opacity;
      // Width-fitted, centered horizontally at the box's top — same as the
      // worker's scale=w:-1 + centered overlay.
      const drawW = w;
      const drawH = (drawW * img.naturalHeight) / Math.max(1, img.naturalWidth);
      ctx.drawImage(img, x + (w - drawW) / 2 + slideX, y + slideY, drawW, drawH);
      ctx.restore();
    } else if (layer.kind === "counter") {
      drawCounterLayer(ctx, width, height, layer, pack, localTime, start, end - start, fontScale);
    } else if (layer.kind === "toggle_grid") {
      drawToggleGridLayer(ctx, width, height, layer, pack, localTime, start);
    } else if (layer.kind === "compare_bars") {
      drawCompareBarsLayer(ctx, width, height, layer, pack, localTime, start, end - start, fontScale);
    } else if (layer.kind === "stack") {
      drawStackLayer(ctx, width, height, layer, pack, localTime, start, end - start, fontScale);
    } else if (layer.kind === "stamp") {
      // Stamp/cta ride the worker's PNG-overlay machinery: entrance anims +
      // alpha-reduce, content width-fitted and centered in the box.
      drawStampLayer(ctx, width, height, layer, slideX, slideY, pop, fade);
    } else if (layer.kind === "cta") {
      drawCTALayer(ctx, width, height, layer, fontScale, slideX, slideY, pop, fade);
    }
  }
}

// ── Motion-kind painters (multi-form engine) ──
// Hand-mirrored twins of vworker/motion_layers.go: the counter value
// formula, the toggle on/off schedule, the bar ease-out and the stack
// stagger MUST stay in lockstep with the ffmpeg filters or the preview
// drifts from the burn-in.

/** counter — number count-up: value = from + (to-from)·min(1,max(0,
 * (t-start)/win)), rendered as prefix + animated value + suffix with a 0.3s
 * fade — the canvas twin of counterFilter's drawtext %{eif} expansion.
 * Entrance anims are intentionally ignored: the worker renders counters
 * with a plain enable window. */
function drawCounterLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  pack: ResolvedPack,
  localTime: number,
  start: number,
  win: number,
  fontScale: number,
) {
  const { x, y, w } = effectiveBox(layer);
  const x0 = x * width;
  const y0 = y * height;
  const bw = w * width;
  const fontSize = (layer.font_size || 48) * fontScale;
  const align = layer.align || "center";
  const from = layer.from ?? 0;
  const to = layer.to ?? 0;
  const decimals = Math.max(0, Math.min(2, Math.round(layer.decimals ?? 0)));
  const value = from + (to - from) * Math.min(1, Math.max(0, (localTime - start) / win));
  const text = `${layer.text ?? ""}${value.toFixed(decimals)}${layer.suffix ?? ""}`;
  ctx.save();
  ctx.globalAlpha = (layer.opacity ?? 1) * Math.min(1, Math.max(0, (localTime - start) / 0.3));
  ctx.font = layerFontString(layer, fontSize);
  ctx.fillStyle = packFillOr(pack, layer, "#38BDF8");
  ctx.textBaseline = "top";
  ctx.textAlign = align === "left" ? "left" : align === "right" ? "right" : "center";
  // Soft halo — same treatment as plain text layers (half-strength border +
  // drop shadow on the burn-in).
  ctx.shadowColor = "rgba(0,0,0,0.5)";
  ctx.shadowBlur = fontSize * 0.22;
  ctx.shadowOffsetY = 2;
  ctx.fillText(text, align === "left" ? x0 : align === "right" ? x0 + bw : x0 + bw / 2, y0);
  ctx.restore();
}

/** toggle_grid — cols×rows switches flipping on/off with the worker's exact
 * arithmetic schedule: on = ((c·31 + r·17 + step·7) mod 5) < 3 where
 * step = floor((t-start)/cadence) (toggleOnExpr / toggleGridFilters).
 * Off cells draw at min(opacity, 0.5), on cells at opacity. The switch
 * cells get rounded corners (the burn-in's drawboxes are square) — a
 * cosmetic-only divergence, the schedule and colors are exact. */
function drawToggleGridLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  pack: ResolvedPack,
  localTime: number,
  start: number,
) {
  const { x, y, w, h } = effectiveBox(layer);
  const x0 = Math.round(x * width);
  const y0 = Math.round(y * height);
  const bw = Math.round(w * width);
  const bh = Math.round(h * height);
  const cols = effectiveCols(layer);
  const rows = effectiveRows(layer);
  const cadence = effectiveCadence(layer);
  const opacity = layer.opacity ?? 1;
  const onColor = packFillOr(pack, layer, "#22C55E");
  const offColor = layer.fill_b || "#334155";
  const gap = Math.max(2, Math.round(bw * 0.012));
  const cellW = Math.floor((bw - gap * (cols - 1)) / cols);
  const cellH = Math.floor((bh - gap * (rows - 1)) / rows);
  if (cellW < 2 || cellH < 2) return;
  const step = Math.floor((localTime - start) / cadence);
  const radius = Math.max(2, Math.round(Math.min(cellW, cellH) * 0.14));
  ctx.save();
  for (let r = 0; r < rows; r++) {
    for (let c = 0; c < cols; c++) {
      const on = (c * 31 + r * 17 + step * 7) % 5 < 3;
      ctx.globalAlpha = on ? opacity : Math.min(opacity, 0.5);
      ctx.fillStyle = on ? onColor : offColor;
      ctx.beginPath();
      ctx.roundRect(x0 + c * (cellW + gap), y0 + r * (cellH + gap), cellW, cellH, radius);
      ctx.fill();
    }
  }
  ctx.restore();
}

/** compare_bars — two labeled bars growing to their target widths with the
 * worker's ease-out 1-(1-p)² over grow = min(0.8, win·0.6). The burn-in
 * quantizes the growth into 8 drawbox steps; the browser animates the same
 * ease continuously — identical at rest (blessed by the worker's comment).
 * Each row: dim full-width track (94A3B8 @ 25%), eased bar, label above. */
function drawCompareBarsLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  pack: ResolvedPack,
  localTime: number,
  start: number,
  win: number,
  fontScale: number,
) {
  const { x, y, w, h } = effectiveBox(layer);
  const x0 = Math.round(x * width);
  const y0 = Math.round(y * height);
  const bw = Math.round(w * width);
  const bh = Math.round(h * height);
  const opacity = Math.min(1, layer.opacity ?? 1);
  const fontSize = (layer.font_size || 48) * fontScale;
  const grow = Math.min(0.8, win * 0.6);
  const p = grow > 0 ? Math.min(1, Math.max(0, (localTime - start) / grow)) : 1;
  const ease = 1 - (1 - p) * (1 - p); // ease-out-quad
  const fillA = packFillOr(pack, layer, "#38BDF8");
  const fillB = layer.fill_b || "#F97316";
  const rowH = Math.floor(bh / 2);
  const labelH = Math.floor(fontSize * 1.25);
  let barH = rowH - labelH - Math.floor(rowH / 12);
  if (barH < 4) barH = 4;
  barH = Math.min(barH, Math.floor(rowH / 2));
  const labelFade = Math.min(1, Math.max(0, (localTime - start) / 0.3));
  ctx.save();
  ctx.font = layerFontString(layer, fontSize);
  ctx.textBaseline = "top";
  ctx.textAlign = "left";
  for (let row = 0; row < 2; row++) {
    const label = row === 0 ? layer.label_a : layer.label_b;
    const widthFrac = row === 0 ? effectiveWidthA(layer) : effectiveWidthB(layer);
    const color = row === 0 ? fillA : fillB;
    const rowY = y0 + row * rowH;
    const barY = rowY + labelH;
    // Dim full-width track.
    ctx.globalAlpha = opacity * 0.25;
    ctx.fillStyle = "#94A3B8";
    ctx.fillRect(x0, barY, bw, barH);
    // Bar easing out to its target width.
    ctx.globalAlpha = opacity;
    ctx.fillStyle = color;
    ctx.fillRect(x0, barY, Math.round(bw * widthFrac * ease), barH);
    // Label above the bar (0.3s fade like the burn-in's drawtext alpha).
    if (label) {
      ctx.globalAlpha = opacity * labelFade;
      ctx.fillStyle = pack.text || "#FFFFFF";
      ctx.shadowColor = "rgba(0,0,0,0.5)";
      ctx.shadowBlur = fontSize * 0.22;
      ctx.shadowOffsetY = 2;
      ctx.fillText(label, x0, rowY);
      ctx.shadowColor = "transparent";
      ctx.shadowBlur = 0;
      ctx.shadowOffsetY = 0;
    }
  }
  ctx.restore();
}

/** Slab k color: fill→fill_b gradient mix by k/(n-1), fill alone, or the
 * dark default — mirror of the worker's per-slab hex derivation. */
function stackSlabColor(
  c0: { r: number; g: number; b: number } | null,
  c1: { r: number; g: number; b: number } | null,
  k: number,
  n: number,
): string {
  const hex = (r: number, g: number, b: number) =>
    `#${Math.round(r).toString(16).padStart(2, "0")}${Math.round(g).toString(16).padStart(2, "0")}${Math.round(b).toString(16).padStart(2, "0")}`;
  if (c0 && c1) {
    const t = n > 1 ? k / (n - 1) : 0;
    return hex(c0.r + (c1.r - c0.r) * t, c0.g + (c1.g - c0.g) * t, c0.b + (c1.b - c0.b) * t);
  }
  if (c0) return hex(c0.r, c0.g, c0.b);
  return "#1E293B";
}

/** stack — n slabs sliding in top-down with a per-slab stagger
 * (slideSec = min(0.27, win·0.35), stagSec = min(0.18, (win-slide)/(n-1)·0.8))
 * and optional labels that appear once a slab lands. The burn-in quantizes
 * each slide into 3 ease-out steps; the browser animates the same ease
 * continuously — identical at rest. */
function drawStackLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  pack: ResolvedPack,
  localTime: number,
  start: number,
  win: number,
  fontScale: number,
) {
  const { x, y, w, h } = effectiveBox(layer);
  const x0 = Math.round(x * width);
  const y0 = Math.round(y * height);
  const bw = Math.round(w * width);
  const bh = Math.round(h * height);
  const n = effectiveN(layer);
  const opacity = Math.min(1, layer.opacity ?? 1);
  const fontSize = (layer.font_size || 48) * fontScale;
  const slideSec = Math.min(0.27, win * 0.35);
  const stagSec = n > 1 ? Math.min(0.18, ((win - slideSec) / (n - 1)) * 0.8) : 0;
  const gap = Math.floor((bh * 4) / 100);
  const slabH = Math.max(4, Math.floor((bh - gap * (n - 1)) / n));
  const slideMax = Math.round(0.05 * height);
  const c0 = layer.fill ? hexToRgb(layer.fill) : null;
  const c1 = layer.fill_b ? hexToRgb(layer.fill_b) : null;
  ctx.save();
  ctx.font = layerFontString(layer, fontSize);
  ctx.textBaseline = "top";
  ctx.textAlign = "left";
  ctx.shadowColor = "rgba(0,0,0,0.5)";
  ctx.shadowBlur = fontSize * 0.22;
  ctx.shadowOffsetY = 2;
  for (let k = 0; k < n; k++) {
    const enterAt = start + stagSec * k;
    if (localTime < enterAt) continue;
    const slabY = y0 + k * (slabH + gap);
    const p = slideSec > 0 ? Math.min(1, Math.max(0, (localTime - enterAt) / slideSec)) : 1;
    const ease = 1 - (1 - p) * (1 - p); // ease-out-quad
    const off = Math.round(slideMax * (1 - ease));
    ctx.globalAlpha = opacity;
    ctx.fillStyle = stackSlabColor(c0, c1, k, n);
    ctx.fillRect(x0, slabY + off, bw, slabH);
    const label = layer.labels?.[k];
    if (label && label.trim() && localTime >= enterAt + slideSec) {
      ctx.globalAlpha = opacity;
      ctx.fillStyle = pack.text || "#FFFFFF";
      const labelY = slabY + Math.max(2, Math.floor((slabH - Math.floor(fontSize * 1.1)) / 2));
      ctx.fillText(label, x0 + Math.floor((slabH * 35) / 100), labelY);
    }
  }
  ctx.restore();
}

/** stamp — rotated bordered stamp: border ring + bold centered text with
 * shrink-to-fit (never overflows 62% of the box width), rotated by the
 * layer's angle (default -8°) and fit-scaled inside the box. Canvas twin of
 * renderStampPNG; the ring uses an evenodd path instead of the worker's
 * punch-out so the backdrop stays intact. */
function drawStampLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  slideX: number,
  slideY: number,
  pop: number,
  fade: number,
) {
  const text = (layer.text ?? "").trim();
  if (!text) return;
  const { x, y, w, h } = effectiveBox(layer);
  const bw = Math.round(w * width);
  const bh = Math.round(h * height);
  const cx = x * width + bw / 2 + slideX;
  const cy = y * height + bh / 2 + slideY;
  const fill = layer.fill || "#FFFFFF";
  // Shrink-to-fit — stampFaceSize: start at bh·0.38, step ×0.9 down to 10.
  let fontSize = Math.floor(bh * 0.38);
  const maxTextW = Math.floor((bw * 62) / 100);
  ctx.font = boldFontString(fontSize);
  let textW = ctx.measureText(text).width;
  while (textW > maxTextW && fontSize > 10) {
    fontSize = Math.floor(fontSize * 0.9);
    ctx.font = boldFontString(fontSize);
    textW = ctx.measureText(text).width;
  }
  const pad = Math.floor(bh * 0.22);
  const sw = textW + pad * 2;
  const sh = bh;
  // Border ring width at 1× — the worker draws max(3, sh·2·7/100) px at 2×
  // supersample (int math), i.e. max(1.5, floor(sh·14/100)/2) here.
  const border = Math.max(1.5, Math.floor((sh * 14) / 100) / 2);
  const angle = layer.angle !== undefined && layer.angle !== 0 ? layer.angle : -8;
  const rad = (angle * Math.PI) / 180;
  const sin = Math.abs(Math.sin(rad));
  const cos = Math.abs(Math.cos(rad));
  const needW = sw * cos + sh * sin;
  const needH = sw * sin + sh * cos;
  const fit = Math.min(1, bw / needW, bh / needH);
  const metrics = ctx.measureText("Hg");
  const ascent = Math.ceil(metrics.actualBoundingBoxAscent);
  const descent = Math.ceil(metrics.actualBoundingBoxDescent);

  ctx.save();
  ctx.translate(cx, cy);
  ctx.rotate(rad);
  ctx.scale(pop * fit, pop * fit);
  ctx.globalAlpha = fade * (layer.opacity ?? 1);
  // Border ring (evenodd: outer rect minus the inner punch-out).
  ctx.fillStyle = fill;
  ctx.beginPath();
  ctx.rect(-sw / 2, -sh / 2, sw, sh);
  ctx.rect(-sw / 2 + border, -sh / 2 + border, sw - border * 2, sh - border * 2);
  ctx.fill("evenodd");
  // Bold text centered in the punched-out middle.
  ctx.textAlign = "center";
  ctx.textBaseline = "alphabetic";
  const baseline = (sh + ascent - descent) / 2 - 1;
  ctx.fillText(text, 0, -sh / 2 + baseline);
  ctx.restore();
}

/** cta — horizontal gradient pill (radius h/2, fill→fill_b) with centered
 * bold white text — the call-to-action bar at the bottom of the frame.
 * Canvas twin of renderCTAPNG (shrink-to-fit at 78% of the pill width). */
function drawCTALayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  fontScale: number,
  slideX: number,
  slideY: number,
  pop: number,
  fade: number,
) {
  const text = (layer.text ?? "").trim();
  if (!text) return;
  const { x, y, w, h } = effectiveBox(layer);
  const bx = x * width + slideX;
  const by = y * height + slideY;
  const bw = w * width;
  const bh = h * height;
  const c0 = layer.fill || "#38BDF8";
  const c1 = layer.fill_b || "#8B5CF6";
  // Explicit font_size is render-space; the fallback derives from the box.
  let fontSize = layer.font_size ? layer.font_size * fontScale : Math.floor(bh * 0.44);
  const maxTextW = Math.floor((bw * 78) / 100);
  ctx.font = boldFontString(fontSize);
  let textW = ctx.measureText(text).width;
  while (textW > maxTextW && fontSize > 10) {
    fontSize = Math.floor(fontSize * 0.93);
    ctx.font = boldFontString(fontSize);
    textW = ctx.measureText(text).width;
  }
  const grad = ctx.createLinearGradient(bx, by, bx + bw, by);
  grad.addColorStop(0, c0);
  grad.addColorStop(1, c1);
  ctx.save();
  // Pop scales around the pill center (worker scales the PNG input which
  // the overlay centers in the box — same visual).
  ctx.translate(bx + bw / 2, by + bh / 2);
  ctx.scale(pop, pop);
  ctx.translate(-(bx + bw / 2), -(by + bh / 2));
  ctx.globalAlpha = fade * (layer.opacity ?? 1);
  ctx.fillStyle = grad;
  ctx.beginPath();
  ctx.roundRect(bx, by, bw, bh, bh / 2);
  ctx.fill();
  const metrics = ctx.measureText("Hg");
  const ascent = Math.ceil(metrics.actualBoundingBoxAscent);
  const descent = Math.ceil(metrics.actualBoundingBoxDescent);
  const baseline = (bh + ascent - descent) / 2 - 1;
  // White at alpha 235/255 — the worker's uniform for pill text.
  ctx.fillStyle = "rgba(255,255,255,0.92)";
  ctx.textAlign = "center";
  ctx.textBaseline = "alphabetic";
  ctx.fillText(text, bx + bw / 2, by + baseline);
  ctx.restore();
}

/** Highlighted text — word-level greedy wrap with per-word colors, the
 * exact layout math of renderHighlightTextPNG: measureText word widths,
 * 13/10 line height, baseline centered in each line box, and a hard offset
 * shadow (alpha 96/255, no blur). Word matching is case-sensitive; the
 * LAST matching highlight wins (map-assignment order on the worker). */
function drawHighlightTextLayer(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  layer: Layer,
  fontScale: number,
  slideX: number,
  slideY: number,
  pop: number,
  fade: number,
) {
  const fontSize = (layer.font_size || 48) * fontScale;
  const base = layer.fill || "#FFFFFF";
  const highlightOf = (word: string): string | null => {
    let color: string | null = null;
    for (const hl of layer.highlights ?? []) {
      if (hl.word === word && hexToRgb(hl.color)) color = hl.color;
    }
    return color;
  };
  const { x, y, w } = effectiveBox(layer);
  const x0 = x * width + slideX;
  const y0 = y * height + slideY;
  const bw = w * width;

  ctx.font = layerFontString(layer, fontSize);
  const spaceW = Math.ceil(ctx.measureText(" ").width);
  const lineH = Math.floor(fontSize * 1.3);
  const shadow = Math.max(2, Math.floor(fontSize / 22));

  // Greedy wrap — identical pen-advance math to the worker's layout.
  const words = (layer.text ?? "").trim().split(/\s+/).filter(Boolean);
  interface HWord {
    text: string;
    w: number;
    color: string;
  }
  const lines: HWord[][] = [];
  let cur: HWord[] = [];
  let curW = 0;
  for (const word of words) {
    const ww = Math.ceil(ctx.measureText(word).width);
    let pen = curW;
    if (cur.length > 0) pen += spaceW;
    if (pen + ww > bw && cur.length > 0) {
      lines.push(cur);
      cur = [];
      pen = 0;
    }
    cur.push({ text: word, w: ww, color: highlightOf(word) ?? base });
    curW = pen + ww;
  }
  if (cur.length > 0) lines.push(cur);
  if (lines.length === 0) return;

  const metrics = ctx.measureText("Hg");
  const ascent = Math.ceil(metrics.actualBoundingBoxAscent);
  const descent = Math.ceil(metrics.actualBoundingBoxDescent);
  const lineBoxH = ascent + descent;

  // Pop scales around the text block's center (the worker scales the PNG
  // input; the overlay then centers it in the box — same visual).
  const cx = x0 + bw / 2;
  const cy = y0 + (lines.length * lineH) / 2;

  ctx.save();
  ctx.translate(cx, cy);
  ctx.scale(pop, pop);
  ctx.translate(-cx, -cy);
  ctx.globalAlpha = fade * (layer.opacity ?? 1);
  ctx.font = layerFontString(layer, fontSize);
  ctx.textBaseline = "alphabetic";
  ctx.textAlign = "left";
  for (let li = 0; li < lines.length; li++) {
    const line = lines[li];
    if (!line) continue;
    let lineW = 0;
    for (let k = 0; k < line.length; k++) {
      const hw = line[k];
      if (!hw) continue;
      if (k > 0) lineW += spaceW;
      lineW += hw.w;
    }
    // Align within the box (matches layerXExpr / the worker's lineX).
    const align = layer.align || "center";
    let lineX = 0;
    if (align === "right") lineX = bw - lineW;
    else if (align === "center") lineX = (bw - lineW) / 2;
    const baseline = li * lineH + (lineH - lineBoxH) / 2 + ascent;
    let pen = lineX;
    for (const word of line) {
      // Soft offset shadow under the fill — the worker's alpha-96 black
      // shadow pass (canvas shadows composite equivalently).
      ctx.shadowColor = "rgba(0,0,0,0.376)";
      ctx.shadowBlur = 0;
      ctx.shadowOffsetX = shadow;
      ctx.shadowOffsetY = shadow;
      ctx.fillStyle = word.color;
      ctx.fillText(word.text, x0 + pen, y0 + baseline);
      pen += word.w + spaceW;
    }
  }
  ctx.restore();
}

/** Bundled caption fonts (public/fonts, same OFL files the worker embeds) —
 * the preview uses exactly what the server burns into the render. */
export const CAPTION_DISPLAY_FONT = '"Be Vietnam Pro", system-ui, sans-serif';
export const CAPTION_MONO_FONT = '"JetBrains Mono", ui-monospace, monospace';
export const LAYER_TEXT_FONT = '"Inter", system-ui, sans-serif';

/** Two drifting radial glow orbs — the exact sin-phase formulas the worker's
 * ffmpeg overlay expressions use (appendGlowSteps in vworker/ffmpeg.go).
 * Keep the two in lockstep or preview drifts from the render. */
function drawGlowOrbs(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  hex: string,
  t: number,
) {
  const rgb = hex.replace("#", "");
  const r = parseInt(rgb.slice(0, 2), 16) || 0;
  const g = parseInt(rgb.slice(2, 4), 16) || 0;
  const b = parseInt(rgb.slice(4, 6), 16) || 0;
  const orb = (cx: number, cy: number, d: number) => {
    const rad = d / 2;
    const grad = ctx.createRadialGradient(cx, cy, 0, cx, cy, rad);
    grad.addColorStop(0, `rgba(${r},${g},${b},0.17)`);
    grad.addColorStop(0.5, `rgba(${r},${g},${b},0.05)`);
    grad.addColorStop(1, "rgba(0,0,0,0)");
    ctx.fillStyle = grad;
    ctx.fillRect(cx - rad, cy - rad, rad * 2, rad * 2);
  };
  orb(
    width * (0.26 + 0.1 * Math.sin(t / 5.3)),
    height * (0.26 + 0.05 * Math.cos(t / 4.1)),
    width * 0.95,
  );
  orb(
    width * (0.74 + 0.08 * Math.sin(t / 6.1 + 2.2)),
    height * (0.72 + 0.05 * Math.sin(t / 5.0 + 1.0)),
    width * 0.72,
  );
}

/** Darkened frame edges — the canvas twin of the worker's vignette filter
 * (applied under the text layers so type stays crisp). */
function drawVignette(ctx: CanvasRenderingContext2D, width: number, height: number) {
  const grad = ctx.createRadialGradient(
    width / 2,
    height / 2,
    Math.hypot(width, height) * 0.28,
    width / 2,
    height / 2,
    Math.hypot(width, height) * 0.62,
  );
  grad.addColorStop(0, "rgba(0,0,0,0)");
  grad.addColorStop(1, "rgba(0,0,0,0.38)");
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, width, height);
}

/** One laid-out caption word with its pen position. */
interface CapWord {
  text: string;
  x: number; // pen x relative to the line start
  line: number;
  w: number;
}

/** Caption painter mirroring the worker's PNG compositor (caption.go):
 * display font (mono for the "mono" eyebrow style), rounded translucent chip
 * background, entrance fade, and the karaoke rule "word k lights up at k/N
 * of the narration while the rest stays dim" — the same k/N math the server
 * uses, so the file renders exactly what the preview showed. */
function drawCaption(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  scene: Scene,
  localTime: number,
  narrProgress?: number,
  fontScale: number = 1,
) {
  const cap = scene.caption;
  if (!cap?.text) return;
  const style = cap.style ?? "";
  // Explicit font_size is absolute on the server's render (like layers);
  // the fallback is already canvas-relative — don't scale it twice.
  const fontSize = cap.font_size ? cap.font_size * fontScale : Math.round(Math.min(width, height) * 0.035);
  ctx.font =
    style === "mono"
      ? `500 ${fontSize}px ${CAPTION_MONO_FONT}`
      : `700 ${fontSize}px ${CAPTION_DISPLAY_FONT}`;
  ctx.textBaseline = "alphabetic";
  ctx.textAlign = "left";

  // Word wrap with per-word pen positions (86% of the width, like caption.go).
  const maxW = width * 0.86;
  const words = cap.text.split(/\s+/).filter(Boolean);
  if (words.length === 0) return;
  const spaceW = ctx.measureText(" ").width;
  const lines: { words: CapWord[]; w: number }[] = [];
  let cur = { words: [] as CapWord[], w: 0 };
  for (const w of words) {
    const ww = ctx.measureText(w).width;
    const pen = cur.words.length > 0 ? cur.w + spaceW : cur.w;
    if (pen + ww > maxW && cur.words.length > 0) {
      lines.push(cur);
      cur = { words: [], w: 0 };
      cur.words.push({ text: w, x: 0, line: lines.length, w: ww });
      cur.w = ww;
    } else {
      cur.words.push({ text: w, x: pen, line: lines.length, w: ww });
      cur.w = pen + ww;
    }
  }
  if (cur.words.length > 0) lines.push(cur);

  const metrics = ctx.measureText("Hg");
  const lineH = fontSize * 1.32;
  const lineBoxH = metrics.actualBoundingBoxAscent + metrics.actualBoundingBoxDescent;
  const shadow = Math.max(2, Math.round(fontSize / 22));
  const padX = style === "chip" ? (fontSize * 2) / 3 : fontSize / 3 + 8;
  const padY = style === "chip" ? fontSize / 2 : fontSize / 4 + 6;
  const textW = Math.max(...lines.map((l) => l.w));
  const stripW = textW + padX * 2 + shadow;
  const stripH = lines.length * lineH + padY * 2 + shadow;

  const sx = (width - stripW) / 2;
  let sy: number;
  switch (cap.position) {
    case "top":
      sy = 60;
      break;
    case "center":
      sy = (height - stripH) / 2;
      break;
    default:
      sy = height - stripH - 60;
      break;
  }

  // Whole-caption entrance fade (server: fade=t=in st=0 d=0.25).
  const entrance = Math.min(1, localTime / 0.25);

  ctx.save();
  ctx.globalAlpha = entrance;

  if (style === "chip") {
    ctx.fillStyle = "rgba(8,12,22,0.58)";
    const r = Math.min(fontSize * 0.7, stripH / 2);
    ctx.beginPath();
    ctx.roundRect(sx, sy, stripW, stripH, r);
    ctx.fill();
  }

  // Karaoke: with narration progress, unspoken words stay dim; word k is lit
  // at k/N of the voice (identical to the server's reveal schedule).
  const karaoke =
    narrProgress !== undefined && words.length > 0 && !!scene.narration?.trim();
  const shown = karaoke
    ? Math.min(words.length, Math.ceil(Math.max(0, narrProgress) * words.length))
    : words.length;
  let wordIdx = 0;

  for (let li = 0; li < lines.length; li++) {
    const line = lines[li];
    if (!line) continue;
    const lineX = sx + padX + (textW - line.w) / 2;
    const baseline =
      sy + padY + li * lineH + (lineH - lineBoxH) / 2 + metrics.actualBoundingBoxAscent;
    for (const w of line.words) {
      const lit = wordIdx < shown;
      wordIdx++;
      ctx.save();
      ctx.globalAlpha = entrance * (lit ? 1 : karaoke ? 0.38 : 1);
      ctx.shadowColor = "rgba(0,0,0,0.45)";
      ctx.shadowOffsetX = shadow;
      ctx.shadowOffsetY = shadow;
      ctx.shadowBlur = 0;
      ctx.fillStyle = "#fff";
      ctx.fillText(w.text, lineX + w.x, baseline);
      ctx.restore();
    }
  }
  ctx.restore();
}

/** Paint the storyboard frame at scenes[index]/localTime, honoring the
 * scene's enter transition. narrProgress (0..1) is the narration-audio
 * progress of scenes[index] and drives the caption karaoke reveal.
 * scratchA/scratchB are caller-owned reusable offscreen buffers (resized
 * here when the output size changes). */
export function drawStoryboardFrame(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scenes: Scene[],
  index: number,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  scratchA: HTMLCanvasElement,
  scratchB: HTMLCanvasElement,
  narrProgress?: number,
  fontScale: number = 1,
): void {
  renderSceneWithTransition(
    ctx,
    canvas,
    scenes,
    index,
    localTime,
    imageCache,
    renderSceneBase,
    scratchA,
    scratchB,
    narrProgress,
    fontScale,
  );
}

export type { SceneTransition };
