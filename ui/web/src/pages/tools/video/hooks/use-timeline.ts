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
}
export interface Scene {
  type: "image" | "video" | "color";
  source?: string;
  color?: string;
  duration_sec: number;
  fit?: "cover" | "contain";
  mute?: boolean;
  ken_burns?: KenBurns;
  caption?: Caption;
  narration?: string;
  /** How this scene ENTERS (browser preview + client export; the server
   * render pipeline ignores it and cuts hard). */
  transition?: SceneTransition;
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
    undo,
    redo,
    canUndo: historyIndex > 0,
    canRedo: historyIndex < history.length - 1,
    replaceScenes,
  };
}
