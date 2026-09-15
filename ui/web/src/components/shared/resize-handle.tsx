import { useCallback, useRef } from "react";
import { cn } from "@/lib/utils";

interface ResizeHandleProps {
  /** Drag delta in px since the previous move event (positive = drag right). */
  onResize: (dx: number) => void;
  /** Double-click — reset the column to its default width. */
  onReset?: () => void;
  /** Drag lifecycle. Drag start/end let owners shield iframes underneath:
   *  pointer capture does not carry across iframe documents, so an unshielded
   *  frame swallows pointermove and freezes the drag mid-motion. */
  onDragStart?: () => void;
  onDragEnd?: () => void;
  /** Which edge of the neighbouring column this handle sits on (drag direction semantics are identical). */
  side?: "left" | "right";
  className?: string;
  ariaLabel: string;
}

/**
 * Vertical drag divider for resizable chat columns. Pointer-events based
 * (with pointer capture) so it works for mouse and touch alike; the visual
 * is a 1px hairline with a 4px hit area that tints on hover/drag.
 */
export function ResizeHandle({ onResize, onReset, onDragStart, onDragEnd, side = "left", className, ariaLabel }: ResizeHandleProps) {
  const dragging = useRef(false);
  const lastX = useRef(0);

  const stop = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!dragging.current) return;
    dragging.current = false;
    e.currentTarget.releasePointerCapture(e.pointerId);
    onDragEnd?.();
  }, [onDragEnd]);

  const onPointerDown = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    dragging.current = true;
    lastX.current = e.clientX;
    e.currentTarget.setPointerCapture(e.pointerId);
    onDragStart?.();
  }, [onDragStart]);

  const onPointerMove = useCallback(
    (e: React.PointerEvent<HTMLDivElement>) => {
      if (!dragging.current) return;
      const dx = e.clientX - lastX.current;
      lastX.current = e.clientX;
      if (dx !== 0) onResize(dx);
    },
    [onResize],
  );

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={ariaLabel}
      title={ariaLabel}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={stop}
      onPointerCancel={stop}
      onDoubleClick={onReset}
      className={cn(
        "group/handle relative z-10 w-1 shrink-0 cursor-col-resize select-none touch-none",
        "before:absolute before:inset-y-0 before:left-0 before:right-0",
        side === "left" ? "before:-left-px" : "before:-right-px",
        "before:w-px before:bg-border before:transition-colors before:duration-150",
        "hover:before:bg-primary/40 active:before:bg-primary/60",
        className,
      )}
    />
  );
}
