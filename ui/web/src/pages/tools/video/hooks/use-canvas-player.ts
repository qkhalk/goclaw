import { useCallback, useEffect, useRef, useState } from "react";
import type { Scene } from "../hooks/use-timeline";
import type { NarrationAudioController } from "./use-narration-audio";
import { drawStoryboardFrame } from "../components/render-shared";
import { getIconImage, setIconLoadListener } from "../lib/feather-icons";

// ── Types (Scene is the canonical model from use-timeline) ──

interface Storyboard {
  version: number;
  canvas: { width: number; height: number; fps: number };
  scenes: Scene[];
  audio?: { bgm_path?: string; bgm_volume?: number };
  output?: { format?: string; height?: number };
}

export interface CanvasPlayerState {
  isPlaying: boolean;
  currentTime: number;
  currentSceneIndex: number;
  fps: number;
  totalDuration: number;
  /** Narration-audio progress (0..1) of the current scene, when its TTS clip
   * is synthesized — drives the caption karaoke reveal. */
  narrProgress: number | undefined;
}

export interface UseCanvasPlayerReturn {
  state: CanvasPlayerState;
  canvasRef: React.RefObject<HTMLCanvasElement | null>;
  play: () => void;
  pause: () => void;
  seek: (time: number) => void;
  stepFrame: (delta: number) => void;
  resize: (width: number, height: number) => void;
}

// ── Helpers ──

function totalDuration(scenes: Scene[]): number {
  return scenes.reduce((acc, s) => acc + (Number(s.duration_sec) || 0), 0);
}

function sceneAtTime(scenes: Scene[], time: number): { index: number; localTime: number } {
  let elapsed = 0;
  for (let i = 0; i < scenes.length; i++) {
    const dur = Number(scenes[i]?.duration_sec) || 0;
    if (time < elapsed + dur || i === scenes.length - 1) {
      return { index: i, localTime: time - elapsed };
    }
    elapsed += dur;
  }
  return { index: 0, localTime: 0 };
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => resolve(img);
    img.onerror = () => reject(new Error(`Failed to load image: ${src}`));
    img.src = src;
  });
}

// ── Hook ──

export function useCanvasPlayer(
  storyboard: Storyboard,
  narration?: NarrationAudioController,
): UseCanvasPlayerReturn {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [state, setState] = useState<CanvasPlayerState>({
    isPlaying: false,
    currentTime: 0,
    currentSceneIndex: 0,
    fps: storyboard.canvas.fps,
    totalDuration: totalDuration(storyboard.scenes),
    narrProgress: undefined,
  });

  const isPlayingRef = useRef(false);
  const currentTimeRef = useRef(0);
  const rafRef = useRef<number>(0);
  const lastFrameTimeRef = useRef(0);
  const imageCacheRef = useRef(new Map<string, HTMLImageElement>());
  const loadingImagesRef = useRef(new Set<string>());
  // Reusable offscreen buffers for transition compositing.
  const scratchARef = useRef<HTMLCanvasElement | null>(null);
  const scratchBRef = useRef<HTMLCanvasElement | null>(null);
  // The narration clip currently sounding (scene-keyed so scene switches and
  // loop restarts hand the element back correctly).
  const activeNarrRef = useRef<{ el: HTMLAudioElement; key: string } | null>(null);
  const narrationRef = useRef<NarrationAudioController | undefined>(narration);
  narrationRef.current = narration;
  // Thaw the frame identity so callbacks below don't rebuild per keystroke.
  const scenesRef = useRef(storyboard.scenes);
  scenesRef.current = storyboard.scenes;

  // Preload images (scene sources + image-layer sources)
  useEffect(() => {
    const wanted = new Set<string>();
    for (const scene of storyboard.scenes) {
      if ((scene.type === "image" || scene.type === "video") && scene.source) {
        wanted.add(scene.source);
      }
      for (const layer of scene.layers ?? []) {
        if (layer.kind === "image" && layer.source) wanted.add(layer.source);
      }
    }
    for (const src of wanted) {
      if (imageCacheRef.current.has(src) || loadingImagesRef.current.has(src)) continue;
      loadingImagesRef.current.add(src);
      loadImage(src)
        .then((img) => {
          imageCacheRef.current.set(src, img);
          loadingImagesRef.current.delete(src);
          // Force re-render after image loads
          if (!isPlayingRef.current) {
            renderCurrentFrame();
          }
        })
        .catch(() => {
          loadingImagesRef.current.delete(src);
        });
    }
    // Warm the icon image cache (icon layers) — draws skip until each glyph
    // arrives, then the load listener repaints.
    for (const scene of storyboard.scenes) {
      for (const layer of scene.layers ?? []) {
        if (layer.kind === "icon") getIconImage(layer);
      }
    }
  }, [storyboard.scenes]);

  /** Keep the current scene's narration clip aligned with the playhead:
   * start/pause/resync the HTMLAudioElement, lazily synthesize missing
   * clips (cached by the controller), and pre-synthesize the next scene's
   * audio near the scene tail so playback stays gapless. */
  const syncNarration = useCallback((index: number, localTime: number, playing: boolean) => {
    const narr = narrationRef.current;
    const scenes = scenesRef.current;
    if (!narr) return;
    const scene = scenes[index];
    const text = typeof scene?.narration === "string" ? scene.narration.trim() : "";
    const voice = scene?.narration_voice;
    const key = `${voice ?? ""}::${text}`;

    // Scene switched (or narration edited) — silence the previous clip.
    if (activeNarrRef.current && activeNarrRef.current.key !== key) {
      activeNarrRef.current.el.pause();
      activeNarrRef.current = null;
    }
    if (!text) return;

    const el = narr.get(text, voice);
    if (!el) {
      // Missing clip: synthesize in the background; the next frame picks it
      // up from the controller's cache (single-flight, so rAF spam is safe).
      narr.prepare(text, voice).catch(() => {});
      if (playing) {
        const next = scenes[index + 1];
        if (typeof next?.narration === "string" && next.narration.trim()) {
          narr.prepare(next.narration, next.narration_voice).catch(() => {});
        }
      }
      return;
    }

    const dur = Number.isFinite(el.duration) ? el.duration : 0;
    const offset = Math.max(0, Math.min(localTime, dur - 0.05));
    if (playing) {
      if (el.paused && localTime < dur) {
        el.currentTime = offset;
        activeNarrRef.current = { el, key };
        void el.play().catch(() => {});
      } else if (!el.paused && Math.abs(el.currentTime - localTime) > 0.35 && localTime < dur) {
        // Chase desyncs (tab throttling, loop restart, manual seek).
        el.currentTime = offset;
      }
      if (localTime >= dur + 0.05 && !el.paused) {
        el.pause();
      }
      // Lookahead: warm the next scene's clip while this one plays out.
      if (scene && localTime > scene.duration_sec - 1.2) {
        const next = scenes[index + 1];
        if (typeof next?.narration === "string" && next.narration.trim()) {
          narr.prepare(next.narration, next.narration_voice).catch(() => {});
        }
      }
    } else {
      // Paused scrub: park the clip at the playhead without playing.
      if (!el.paused) el.pause();
      try {
        el.currentTime = offset;
      } catch {
        /* seek before metadata — ignored */
      }
    }
  }, []);

  const renderCurrentFrame = useCallback(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const time = currentTimeRef.current;
    const scenes = scenesRef.current;
    const { index, localTime } = sceneAtTime(scenes, time);
    const scene = scenes[index];
    if (!scene) return;

    syncNarration(index, localTime, isPlayingRef.current);

    // Karaoke progress from the sounding clip (or the parked playhead when
    // paused) — undefined leaves the caption fully visible.
    let narrProgress: number | undefined;
    const text = typeof scene.narration === "string" ? scene.narration.trim() : "";
    if (text) {
      const el = narrationRef.current?.get(text, scene.narration_voice);
      if (el && Number.isFinite(el.duration) && el.duration > 0) {
        narrProgress = Math.max(0, Math.min(1, el.currentTime / el.duration));
      }
    }

    if (!scratchARef.current) scratchARef.current = document.createElement("canvas");
    if (!scratchBRef.current) scratchBRef.current = document.createElement("canvas");
    drawStoryboardFrame(
      ctx,
      canvas,
      scenes,
      index,
      localTime,
      imageCacheRef.current,
      scratchARef.current,
      scratchBRef.current,
      narrProgress,
    );

    setState((prev) => ({
      ...prev,
      currentTime: time,
      currentSceneIndex: index,
      fps: storyboard.canvas.fps,
      totalDuration: totalDuration(scenes),
      narrProgress,
    }));
  }, [storyboard.canvas.fps, syncNarration]);

  const frameLoop = useCallback(
    (timestamp: number) => {
      if (!isPlayingRef.current) return;

      if (lastFrameTimeRef.current === 0) {
        lastFrameTimeRef.current = timestamp;
      }

      const delta = (timestamp - lastFrameTimeRef.current) / 1000;
      lastFrameTimeRef.current = timestamp;

      const total = totalDuration(scenesRef.current);
      currentTimeRef.current = Math.min(currentTimeRef.current + delta, total);

      if (currentTimeRef.current >= total) {
        // Loop back to start
        currentTimeRef.current = 0;
      }

      renderCurrentFrame();
      rafRef.current = requestAnimationFrame(frameLoop);
    },
    [renderCurrentFrame],
  );

  const play = useCallback(() => {
    isPlayingRef.current = true;
    lastFrameTimeRef.current = 0;
    setState((prev) => ({ ...prev, isPlaying: true }));
    rafRef.current = requestAnimationFrame(frameLoop);
  }, [frameLoop]);

  const pause = useCallback(() => {
    isPlayingRef.current = false;
    cancelAnimationFrame(rafRef.current);
    activeNarrRef.current?.el.pause();
    setState((prev) => ({ ...prev, isPlaying: false }));
  }, []);

  const seek = useCallback(
    (time: number) => {
      currentTimeRef.current = Math.max(0, Math.min(time, totalDuration(scenesRef.current)));
      renderCurrentFrame();
    },
    [renderCurrentFrame],
  );

  const stepFrame = useCallback(
    (delta: number) => {
      const frameDuration = 1 / storyboard.canvas.fps;
      currentTimeRef.current = Math.max(
        0,
        Math.min(currentTimeRef.current + delta * frameDuration, totalDuration(scenesRef.current)),
      );
      renderCurrentFrame();
    },
    [storyboard.canvas.fps, renderCurrentFrame],
  );

  const resize = useCallback(
    (width: number, height: number) => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      canvas.width = width;
      canvas.height = height;
      renderCurrentFrame();
    },
    [renderCurrentFrame],
  );

  // Initial render + repaint paused edits (scene/color/duration changes
  // while paused must show up without a play/seek nudge).
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas && canvas.width > 0 && canvas.height > 0 && !isPlayingRef.current) {
      renderCurrentFrame();
    }
  }, [storyboard.scenes, renderCurrentFrame]);

  // Repaint when an icon glyph finishes loading (preview draws skip icons
  // until their image arrives).
  const renderRef = useRef(renderCurrentFrame);
  renderRef.current = renderCurrentFrame;
  useEffect(() => {
    setIconLoadListener(() => {
      if (!isPlayingRef.current) renderRef.current();
    });
    return () => setIconLoadListener(null);
  }, []);

  // Cleanup
  useEffect(() => {
    return () => {
      cancelAnimationFrame(rafRef.current);
      activeNarrRef.current?.el.pause();
    };
  }, []);

  return {
    state,
    canvasRef,
    play,
    pause,
    seek,
    stepFrame,
    resize,
  };
}
