import { useCallback, useState } from "react";

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
  const [history, setHistory] = useState<Scene[][]>([initialScenes ?? [emptyScene()]]);
  const [historyIndex, setHistoryIndex] = useState(0);

  const pushHistory = useCallback(
    (next: Scene[]) => {
      setHistory((prev) => {
        // Trim any future states beyond current index
        const trimmed = prev.slice(0, historyIndex + 1);
        const nextHistory = [...trimmed, next];
        // Cap at MAX_HISTORY
        if (nextHistory.length > MAX_HISTORY) {
          nextHistory.shift();
        }
        return nextHistory;
      });
      setHistoryIndex((prev) => Math.min(prev + 1, MAX_HISTORY - 1));
    },
    [historyIndex],
  );

  const selectScene = useCallback(
    (index: number) => {
      setSelectedIndex(Math.max(0, Math.min(index, scenes.length - 1)));
    },
    [scenes.length],
  );

  const addScene = useCallback(
    (patch?: Partial<Scene>) => {
      setScenes((prev) => {
        const next = [...prev, { ...emptyScene(), ...patch }];
        pushHistory(next);
        setSelectedIndex(next.length - 1);
        return next;
      });
    },
    [pushHistory],
  );

  const removeScene = useCallback(
    (index: number) => {
      setScenes((prev) => {
        if (prev.length <= 1) return prev;
        const next = prev.filter((_, i) => i !== index);
        pushHistory(next);
        setSelectedIndex((i) => Math.min(i, next.length - 1));
        return next;
      });
    },
    [pushHistory],
  );

  const moveScene = useCallback(
    (from: number, to: number) => {
      setScenes((prev) => {
        if (to < 0 || to >= prev.length) return prev;
        const next = [...prev];
        const [moved] = next.splice(from, 1);
        next.splice(to, 0, moved!);
        pushHistory(next);
        setSelectedIndex(to);
        return next;
      });
    },
    [pushHistory],
  );

  const updateScene = useCallback(
    (index: number, patch: Partial<Scene>) => {
      setScenes((prev) => {
        const next = prev.map((s, i) => (i === index ? { ...s, ...patch } : s));
        pushHistory(next);
        return next;
      });
    },
    [pushHistory],
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
