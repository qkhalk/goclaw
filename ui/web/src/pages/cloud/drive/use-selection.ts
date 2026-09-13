import { useCallback, useEffect, useRef, useState } from "react";

/** Multi-select state for one folder view. Keys are entry names (unique within
 * a folder). Shift-click selects the range from the anchor to the clicked
 * index; selection resets whenever `resetKey` (the current path) changes. */
export function useSelection(resetKey: string) {
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [cursor, setCursorState] = useState<number>(-1);
  const anchorRef = useRef<number | null>(null);

  // Selection is per-folder: clear it when the folder changes.
  useEffect(() => {
    setSelected(new Set());
    setCursorState(-1);
    anchorRef.current = null;
  }, [resetKey]);

  const handleClick = useCallback(
    (name: string, index: number, shiftKey: boolean, visibleNames: string[]) => {
      setSelected((prev) => {
        const next = new Set(prev);
        if (shiftKey && anchorRef.current !== null) {
          next.clear();
          const [lo, hi] = anchorRef.current <= index
            ? [anchorRef.current, index]
            : [index, anchorRef.current];
          for (let i = lo; i <= hi && i < visibleNames.length; i++) {
            const name = visibleNames[i];
            if (name !== undefined) next.add(name);
          }
        } else {
          if (next.has(name)) next.delete(name);
          else next.add(name);
          anchorRef.current = index;
        }
        return next;
      });
    },
    [],
  );

  const selectAll = useCallback((names: string[]) => {
    setSelected(new Set(names));
  }, []);

  const clear = useCallback(() => {
    setSelected(new Set());
    setCursorState(-1);
    anchorRef.current = null;
  }, []);

  /** Keyboard cursor: move by ±delta, clamped to the visible list. */
  const moveCursor = useCallback((delta: number, visibleCount: number) => {
    setCursorState((prev) => {
      const next = prev + delta;
      if (visibleCount <= 0) return -1;
      if (prev < 0) return delta < 0 ? visibleCount - 1 : 0;
      return Math.max(0, Math.min(visibleCount - 1, next));
    });
  }, []);

  return {
    selected,
    count: selected.size,
    has: (name: string) => selected.has(name),
    cursor,
    setCursor: setCursorState,
    moveCursor,
    handleClick,
    selectAll,
    clear,
  };
}

export type SelectionApi = ReturnType<typeof useSelection>;
