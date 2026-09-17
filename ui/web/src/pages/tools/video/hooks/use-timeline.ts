import { useCallback, useState } from "react";
import type { SceneTransition } from "../components/scene-transition";

// ── Types (matching storyboard types) ──

interface KenBurns {
  zoom_from: number;
  zoom_to: number;
  pan: "none" | "left" | "right" | "up" | "down";
}
interface Caption {
  text: string;
  position?: "top" | "center" | "bottom";
  font_size?: number;
  /** Visual style: "" plain | chip | mono — mirrored by preview + server. */
  style?: "" | "chip" | "mono";
}
/** One timed overlay inside a scene — mirrors internal/video.Layer (Go).
 * Geometry is normalized 0..1 (top-left origin); a layer is visible while
 * start <= t < start+duration (duration 0 = until the scene ends). */
export interface Layer {
  kind: "text" | "shape" | "image";
  text?: string;
  source?: string;
  shape?: string; // "rect"
  start?: number;
  duration?: number;
  x?: number;
  y?: number;
  w?: number;
  h?: number;
  fill?: string;
  opacity?: number;
  font_size?: number;
  align?: "left" | "center" | "right";
}

export interface Scene {
  type: "image" | "video" | "color";
  source?: string;
  color?: string;
  /** Color scenes: second gradient stop — empty = darker shade of color
   * (mirrors the server's gradients c0/c1 derivation). */
  color2?: string;
  /** Color scenes: faint blueprint grid overlay (server drawgrid). */
  grid?: boolean;
  /** Color scenes: drifting radial glow orbs tinted with this color
   * (server overlay PNGs, visual v2). */
  glow?: string;
  /** Darkened frame edges (server vignette filter, visual v2). */
  vignette?: boolean;
  /** Subtle animated film grain — server render only, preview skips. */
  grain?: boolean;
  duration_sec: number;
  fit?: "cover" | "contain";
  mute?: boolean;
  ken_burns?: KenBurns;
  caption?: Caption;
  narration?: string;
  /** Per-scene TTS voice override (edge-tts id); empty = storyboard default. */
  narration_voice?: string;
  /** How this scene ENTERS — browser preview, client export, and the server
   * render (xfade) all honor it. */
  transition?: SceneTransition;
  /** OpenCut-style per-scene transform (image/video scenes): scale multiple
   * around the frame center, x/y offset in % of frame size, rotation in
   * degrees, 0-1 opacity. */
  transform?: {
    scale?: number;
    x?: number;
    y?: number;
    rotate?: number;
    opacity?: number;
  };
  /** Color grading applied to the scene's media (CSS filter values). */
  filter?: {
    brightness?: number;
    contrast?: number;
    saturate?: number;
    blur?: number;
  };
  /** Timed overlays drawn on the base visual (and under the caption), in
   * array order. Rendered by the browser preview AND the server worker. */
  layers?: Layer[];
}

/** Resolve a layer's visible window within its scene (duration 0 = to end). */
export function layerWindow(l: Layer, sceneSec: number): { start: number; end: number } {
  const start = Math.max(0, l.start ?? 0);
  const end = start + (l.duration && l.duration > 0 ? l.duration : sceneSec - start);
  return { start, end };
}

const MAX_HISTORY = 50;

function emptyScene(): Scene {
  return { type: "image", source: "", duration_sec: 5, fit: "cover" };
}

export interface TimelineState {
  scenes: Scene[];
  selectedIndex: number;
}

export interface UseTimelineReturn {
  state: TimelineState;
  selectScene: (index: number) => void;
  addScene: (scene?: Partial<Scene>) => void;
  removeScene: (index: number) => void;
  moveScene: (from: number, to: number) => void;
  updateScene: (index: number, patch: Partial<Scene>) => void;
  addLayer: (sceneIndex: number, layer: Layer) => void;
  updateLayer: (sceneIndex: number, layerIndex: number, patch: Partial<Layer>) => void;
  removeLayer: (sceneIndex: number, layerIndex: number) => void;
  moveLayer: (sceneIndex: number, layerIndex: number, dir: -1 | 1) => void;
  undo: () => void;
  redo: () => void;
  canUndo: boolean;
  canRedo: boolean;
  replaceScenes: (scenes: Scene[]) => void;
}

export function useTimeline(initialScenes?: Scene[]): UseTimelineReturn {
  const [scenes, setScenes] = useState<Scene[]>(() => initialScenes ?? [emptyScene()]);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [history, setHistory] = useState<Scene[][]>(() => [initialScenes ?? [emptyScene()]]);
  const [historyIndex, setHistoryIndex] = useState(0);

  /** Record the next state: drop any redo tail, append, cap the stack.
   * Pure updater — no side effects (React may double-invoke updaters). */
  const pushHistory = useCallback((next: Scene[]) => {
    setHistory((prev) => {
      const trimmed = prev.slice(0, historyIndex + 1);
      trimmed.push(next);
      if (trimmed.length > MAX_HISTORY) trimmed.shift();
      return trimmed;
    });
    setHistoryIndex((prev) => Math.min(prev + 1, MAX_HISTORY - 1));
  }, [historyIndex]);

  const selectScene = useCallback(
    (index: number) => {
      setSelectedIndex(Math.max(0, Math.min(index, scenes.length - 1)));
    },
    [scenes.length],
  );

  const addScene = useCallback(
    (patch?: Partial<Scene>) => {
      const next = [...scenes, { ...emptyScene(), ...patch }];
      setScenes(next);
      setSelectedIndex(next.length - 1);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const removeScene = useCallback(
    (index: number) => {
      if (scenes.length <= 1) return;
      const next = scenes.filter((_, i) => i !== index);
      setScenes(next);
      setSelectedIndex((i) => Math.min(i, next.length - 1));
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const moveScene = useCallback(
    (from: number, to: number) => {
      if (to < 0 || to >= scenes.length || from === to) return;
      const next = [...scenes];
      const [moved] = next.splice(from, 1);
      if (!moved) return;
      next.splice(to, 0, moved);
      setScenes(next);
      setSelectedIndex(to);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const updateScene = useCallback(
    (index: number, patch: Partial<Scene>) => {
      const next = scenes.map((s, i) => (i === index ? { ...s, ...patch } : s));
      setScenes(next);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  // ── Layer ops (edits ride updateScene's history snapshots) ──

  const addLayer = useCallback(
    (sceneIndex: number, layer: Layer) => {
      const target = scenes[sceneIndex];
      if (!target || (target.layers?.length ?? 0) >= 8) return;
      const next = scenes.map((s, i) =>
        i === sceneIndex ? { ...s, layers: [...(s.layers ?? []), layer] } : s,
      );
      setScenes(next);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const updateLayer = useCallback(
    (sceneIndex: number, layerIndex: number, patch: Partial<Layer>) => {
      const next = scenes.map((s, i) => {
        if (i !== sceneIndex) return s;
        return {
          ...s,
          layers: (s.layers ?? []).map((l, j) => (j === layerIndex ? { ...l, ...patch } : l)),
        };
      });
      setScenes(next);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const removeLayer = useCallback(
    (sceneIndex: number, layerIndex: number) => {
      const next = scenes.map((s, i) =>
        i === sceneIndex ? { ...s, layers: (s.layers ?? []).filter((_, j) => j !== layerIndex) } : s,
      );
      setScenes(next);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const moveLayer = useCallback(
    (sceneIndex: number, layerIndex: number, dir: -1 | 1) => {
      const target = scenes[sceneIndex];
      if (!target?.layers) return;
      const to = layerIndex + dir;
      if (to < 0 || to >= target.layers.length) return;
      const layers = [...target.layers];
      const [moved] = layers.splice(layerIndex, 1);
      if (!moved) return;
      layers.splice(to, 0, moved);
      const next = scenes.map((s, i) => (i === sceneIndex ? { ...s, layers } : s));
      setScenes(next);
      pushHistory(next);
    },
    [scenes, pushHistory],
  );

  const undo = useCallback(() => {
    if (historyIndex <= 0) return;
    const prevIndex = historyIndex - 1;
    const prevScenes = history[prevIndex];
    if (prevScenes) {
      setScenes(prevScenes);
      setHistoryIndex(prevIndex);
      setSelectedIndex((i) => Math.min(i, prevScenes.length - 1));
    }
  }, [history, historyIndex]);

  const redo = useCallback(() => {
    if (historyIndex >= history.length - 1) return;
    const nextIndex = historyIndex + 1;
    const nextScenes = history[nextIndex];
    if (nextScenes) {
      setScenes(nextScenes);
      setHistoryIndex(nextIndex);
      setSelectedIndex((i) => Math.min(i, nextScenes.length - 1));
    }
  }, [history, historyIndex]);

  const replaceScenes = useCallback(
    (newScenes: Scene[]) => {
      const next = newScenes.length > 0 ? newScenes : [emptyScene()];
      setScenes(next);
      setSelectedIndex(0);
      pushHistory(next);
    },
    [pushHistory],
  );

  return {
    state: { scenes, selectedIndex },
    selectScene,
    addScene,
    removeScene,
    moveScene,
    updateScene,
    addLayer,
    updateLayer,
    removeLayer,
    moveLayer,
    undo,
    redo,
    canUndo: historyIndex > 0,
    canRedo: historyIndex < history.length - 1,
    replaceScenes,
  };
}
