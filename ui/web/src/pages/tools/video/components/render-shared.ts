import type { Scene } from "../hooks/use-timeline";
import { layerWindow } from "../hooks/use-timeline";
import { renderSceneWithTransition, type SceneTransition } from "./scene-transition";

// ── Shared storyboard frame renderer ──
//
// Single source of truth for painting one storyboard frame, used by BOTH the
// canvas player (preview) and the client-side MediaRecorder export, so what
// you preview is what exports. Handles: color/image scenes, Ken Burns,
// OpenCut-style per-scene transform (scale/offset/rotate/opacity) + filters
// (brightness/contrast/saturate/blur), captions, and enter transitions.

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

export function renderSceneBase(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
) {
  const { width, height } = canvas;
  ctx.clearRect(0, 0, width, height);

  if (scene.type === "color") {
    ctx.fillStyle = scene.color || "#000000";
    ctx.fillRect(0, 0, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
    drawCaption(ctx, width, height, scene);
    return;
  }

  const img = scene.source ? imageCache.get(scene.source) : undefined;
  if (!img) {
    ctx.fillStyle = "#1a1a2e";
    ctx.fillRect(0, 0, width, height);
    drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
    drawCaption(ctx, width, height, scene);
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

  drawSceneLayers(ctx, width, height, scene, localTime, imageCache);
  drawCaption(ctx, width, height, scene);
}

/** Paint the scene's timed overlay layers (under the caption), in array
 * order — mirrors the worker's ffmpeg layer filters (drawtext/drawbox/
 * overlay) so preview matches the server render. */
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
    if (layer.kind === "text") {
      const fontSize = layer.font_size || 48;
      const align = layer.align || "center";
      ctx.save();
      ctx.globalAlpha = opacity;
      ctx.font = `bold ${fontSize}px sans-serif`;
      ctx.fillStyle = layer.fill || "#FFFFFF";
      ctx.textBaseline = "top";
      ctx.textAlign = align === "left" ? "left" : align === "right" ? "right" : "center";
      ctx.shadowColor = "rgba(0,0,0,0.7)";
      ctx.shadowBlur = fontSize * 0.25;
      const tx = align === "left" ? x : align === "right" ? x + w : x + w / 2;
      const lines = wrapText(ctx, layer.text ?? "", w);
      const lineHeight = fontSize * 1.3;
      lines.forEach((line, li) => ctx.fillText(line, tx, y + li * lineHeight));
      ctx.restore();
    } else if (layer.kind === "shape") {
      const h = (layer.h ?? 0.3) * height;
      ctx.save();
      ctx.globalAlpha = opacity;
      ctx.fillStyle = layer.fill || "#FFFFFF";
      ctx.fillRect(x, y, w, h);
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
      ctx.drawImage(img, x + (w - drawW) / 2, y, drawW, drawH);
      ctx.restore();
    }
  }
}

function drawCaption(ctx: CanvasRenderingContext2D, width: number, height: number, scene: Scene) {
  if (!scene.caption?.text) return;
  const fontSize = scene.caption.font_size || Math.round(Math.min(width, height) * 0.035);
  ctx.font = `bold ${fontSize}px sans-serif`;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.shadowColor = "rgba(0,0,0,0.7)";
  ctx.shadowBlur = fontSize * 0.25;
  ctx.shadowOffsetX = 0;
  ctx.shadowOffsetY = fontSize * 0.05;

  const lines = wrapText(ctx, scene.caption.text, width * 0.85);
  const lineHeight = fontSize * 1.3;
  const totalTextH = lines.length * lineHeight;

  let baseY: number;
  switch (scene.caption.position) {
    case "top":
      baseY = totalTextH / 2 + fontSize;
      break;
    case "center":
      baseY = height / 2;
      break;
    default:
      baseY = height - totalTextH / 2 - fontSize;
      break;
  }

  ctx.fillStyle = "#fff";
  lines.forEach((line, li) => {
    ctx.fillText(line, width / 2, baseY + (li - (lines.length - 1) / 2) * lineHeight);
  });

  ctx.shadowColor = "transparent";
  ctx.shadowBlur = 0;
  ctx.shadowOffsetY = 0;
}

/** Paint the storyboard frame at scenes[index]/localTime, honoring the
 * scene's enter transition. scratchA/scratchB are caller-owned reusable
 * offscreen buffers (resized here when the output size changes). */
export function drawStoryboardFrame(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scenes: Scene[],
  index: number,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  scratchA: HTMLCanvasElement,
  scratchB: HTMLCanvasElement,
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
  );
}

export type { SceneTransition };
