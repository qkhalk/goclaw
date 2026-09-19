import type { Scene } from "../hooks/use-timeline";

// ── Scene enter transitions (Remotion-style compositions) ──
//
// A transition decorates the START of a scene: for the first
// TRANSITION_SEC seconds the incoming frame blends over the tail of the
// previous scene. Honored by the browser player, the client-side export,
// and the server render (xfade chain).

export type SceneTransition = "none" | "fade" | "crossfade" | "slide_left" | "slide_up";

export const TRANSITION_TYPES: SceneTransition[] = ["none", "fade", "crossfade", "slide_left", "slide_up"];

export const TRANSITION_SEC = 0.5;

type DrawScene = (
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scene: Scene,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  narrProgress?: number,
  fontScale?: number,
) => void;

/** Draw scenes[index] at localTime, applying its enter transition (blending
 * over the previous scene's final frame) when inside the transition window.
 * narrProgress (0..1) is the narration-audio progress of scenes[index]; the
 * outgoing scene draws with progress 1 (its narration is over).
 * fontScale maps the storyboard's render-space font pixels onto this canvas
 * (canvasWidth / render width) so preview text matches the server burn-in.
 * scratchA/scratchB are reused offscreen buffers sized to `canvas` to avoid
 * per-frame allocations; they are resized here if the canvas changed. */
export function renderSceneWithTransition(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  scenes: Scene[],
  index: number,
  localTime: number,
  imageCache: Map<string, HTMLImageElement>,
  drawScene: DrawScene,
  scratchA: HTMLCanvasElement,
  scratchB: HTMLCanvasElement,
  narrProgress?: number,
  fontScale: number = 1,
): void {
  const scene = scenes[index];
  if (!scene) return;

  // Incoming frame, drawn normally first.
  drawScene(ctx, canvas, scene, localTime, imageCache, narrProgress, fontScale);

  const type: SceneTransition = scene.transition ?? "none";
  if (index <= 0 || type === "none" || localTime >= TRANSITION_SEC) return;

  const prev = scenes[index - 1];
  if (!prev) return;

  const p = Math.min(1, localTime / TRANSITION_SEC);
  const { width, height } = canvas;

  // Scratch A: previous scene frozen at its last frame.
  if (scratchA.width !== width || scratchA.height !== height) {
    scratchA.width = width;
    scratchA.height = height;
  }
  const aCtx = scratchA.getContext("2d");
  if (!aCtx) return;
  aCtx.clearRect(0, 0, width, height);
  drawScene(aCtx, scratchA, prev, Math.max(0, prev.duration_sec - 1 / 60), imageCache, 1, fontScale);

  // Scratch B: copy of the already-drawn incoming frame.
  if (scratchB.width !== width || scratchB.height !== height) {
    scratchB.width = width;
    scratchB.height = height;
  }
  const bCtx = scratchB.getContext("2d");
  if (!bCtx) return;
  bCtx.clearRect(0, 0, width, height);
  bCtx.drawImage(canvas, 0, 0);

  // Composite (incoming = B, outgoing tail = A).
  ctx.clearRect(0, 0, width, height);
  switch (type) {
    case "fade": {
      // Fade in from black.
      ctx.fillStyle = "#000";
      ctx.fillRect(0, 0, width, height);
      ctx.globalAlpha = p;
      ctx.drawImage(scratchB, 0, 0);
      ctx.globalAlpha = 1;
      break;
    }
    case "slide_left": {
      ctx.drawImage(scratchA, 0, 0);
      ctx.drawImage(scratchB, Math.round(width * (1 - p)), 0);
      break;
    }
    case "slide_up": {
      ctx.drawImage(scratchA, 0, 0);
      ctx.drawImage(scratchB, 0, Math.round(height * (1 - p)));
      break;
    }
    case "crossfade":
    default: {
      ctx.drawImage(scratchA, 0, 0);
      ctx.globalAlpha = p;
      ctx.drawImage(scratchB, 0, 0);
      ctx.globalAlpha = 1;
      break;
    }
  }
}
