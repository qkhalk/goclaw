import type { Scene } from "../hooks/use-timeline";
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

/** Animated two-stop gradient backdrop for color scenes — mirrors the
 * worker's `gradients=c0:c1:speed=0.008` lavfi source: a linear gradient
 * whose axis slowly rotates over the scene. */
function drawColorBackdrop(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  scene: Scene,
  localTime: number,
) {
  const c0 = scene.color || "#000000";
  const c1 = scene.color2?.trim() || darkerHex(c0);
  const angle = (localTime * 0.008 * Math.PI) % Math.PI;
  const r = Math.hypot(width, height) / 2;
  const dx = Math.cos(angle) * r;
  const dy = Math.sin(angle) * r;
  const grad = ctx.createLinearGradient(width / 2 - dx, height / 2 - dy, width / 2 + dx, height / 2 + dy);
  grad.addColorStop(0, c0);
  grad.addColorStop(1, c1);
  ctx.fillStyle = grad;
  ctx.fillRect(0, 0, width, height);

  if (scene.grid) {
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
) {
  const { width, height } = canvas;
  ctx.clearRect(0, 0, width, height);

  if (scene.type === "color") {
    drawColorBackdrop(ctx, width, height, scene, localTime);
    if (scene.glow) drawGlowOrbs(ctx, width, height, scene.glow, localTime);
    if (scene.vignette) drawVignette(ctx, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
    drawCaption(ctx, width, height, scene, localTime, narrProgress);
    return;
  }

  const img = scene.source ? imageCache.get(scene.source) : undefined;
  if (!img) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    if (scene.glow) drawGlowOrbs(ctx, width, height, scene.glow, localTime);
    if (scene.vignette) drawVignette(ctx, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
    drawCaption(ctx, width, height, scene, localTime, narrProgress);
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
  if (scene.glow) drawGlowOrbs(ctx, width, height, scene.glow, localTime);
  drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
  drawCaption(ctx, width, height, scene, localTime, narrProgress);
}

/** Paint the scene's timed overlay layers (under the caption), in array
 * order — mirrors the worker's ffmpeg layer filters (drawtext/drawbox/
 * overlay + entrance anims) so preview matches the server render. */
export function drawSceneLayers(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
) {
  if (!scene.layers?.length) return;
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
      const fontSize = layer.font_size || 48;
      const align = layer.align || "center";
      ctx.save();
      // 0.3s fade at the layer's start — mirrors the worker's drawtext alpha
      // expression; font mirrors the worker's per-layer face selection.
      ctx.globalAlpha = opacity * fade;
      ctx.font = `${layer.font === "display" ? "700 " : layer.font === "mono" ? "500 " : "600 "}${fontSize}px ${layer.font === "display" ? CAPTION_DISPLAY_FONT : layer.font === "mono" ? CAPTION_MONO_FONT : LAYER_TEXT_FONT}`;
      ctx.fillStyle = layer.fill || "#FFFFFF";
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
    }
  }
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
) {
  const cap = scene.caption;
  if (!cap?.text) return;
  const style = cap.style ?? "";
  const fontSize = cap.font_size || Math.round(Math.min(width, height) * 0.035);
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
  );
}

export type { SceneTransition };
