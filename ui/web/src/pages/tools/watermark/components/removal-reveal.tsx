import { useEffect, useMemo, useState } from "react";
import { cn } from "@/lib/utils";

/** Rect of the watermark inside the media, in fractions (0-1) of its size. */
export interface WatermarkRectFractions {
  x: number;
  y: number;
  w: number;
  h: number;
}

interface RemovalRevealProps {
  /** Watermark position in media fractions — the overlay is placed here. */
  rect: WatermarkRectFractions | null;
  /** Fire once per reveal key change (e.g. item id). */
  revealKey: string;
  label?: string;
  className?: string;
}

const PARTICLE_COUNT = 14;
const SPARKLE_TOTAL_MS = 1900;

/**
 * The "logo erase" moment: corner brackets lock onto the watermark, the
 * sparkle flares, then shatters into particles while a scan line sweeps the
 * rect — after which the overlay unmounts and reveals the cleaned media
 * underneath. Honors prefers-reduced-motion (short plain fade).
 */
export function RemovalReveal({ rect, revealKey, label, className }: RemovalRevealProps) {
  const [visible, setVisible] = useState(true);

  useEffect(() => {
    setVisible(true);
    const timer = setTimeout(() => setVisible(false), SPARKLE_TOTAL_MS + 250);
    return () => clearTimeout(timer);
  }, [revealKey]);

  // Deterministic pseudo-random particle directions per reveal key so SSR /
  // re-renders stay stable without storing state.
  const particles = useMemo(
    () =>
      Array.from({ length: PARTICLE_COUNT }, (_, i) => {
        const seed = revealKey.split("").reduce((acc, c) => acc + c.charCodeAt(0) * (i + 3), 0);
        const angle = ((seed % 360) / 360) * Math.PI * 2;
        const dist = 34 + ((seed >> 3) % 46);
        return {
          tx: Math.cos(angle) * dist,
          ty: Math.sin(angle) * dist,
          size: 3 + ((seed >> 5) % 4),
          delay: i * 26,
        };
      }),
    [revealKey],
  );

  if (!rect || !visible) return null;

  const style: React.CSSProperties & Record<string, string> = {
    left: `${rect.x * 100}%`,
    top: `${rect.y * 100}%`,
    width: `${rect.w * 100}%`,
    height: `${rect.h * 100}%`,
    ["--wm-duration" as string]: `${SPARKLE_TOTAL_MS}ms`,
  };

  return (
    <div className={cn("wm-reveal pointer-events-none absolute z-20", className)} style={style} aria-hidden>
      <style>{wmRevealStyles}</style>

      {/* scan sweep */}
      <div className="wm-reveal__scan absolute inset-0 overflow-hidden rounded-sm">
        <div className="wm-reveal__scanline absolute inset-y-0 w-1/3" />
      </div>

      {/* corner brackets */}
      <span className="wm-reveal__bracket wm-reveal__bracket--tl absolute left-0 top-0 h-2.5 w-2.5 border-l-2 border-t-2" />
      <span className="wm-reveal__bracket wm-reveal__bracket--tr absolute right-0 top-0 h-2.5 w-2.5 border-r-2 border-t-2" />
      <span className="wm-reveal__bracket wm-reveal__bracket--bl absolute bottom-0 left-0 h-2.5 w-2.5 border-b-2 border-l-2" />
      <span className="wm-reveal__bracket wm-reveal__bracket--br absolute bottom-0 right-0 h-2.5 w-2.5 border-b-2 border-r-2" />

      {/* sparkle */}
      <Sparkle className="wm-reveal__sparkle absolute left-1/2 top-1/2 h-[62%] w-[62%] -translate-x-1/2 -translate-y-1/2 text-white drop-shadow-[0_0_10px_rgba(255,255,255,0.9)]" />

      {/* particles */}
      {particles.map((p, i) => (
        <span
          key={i}
          className="wm-reveal__particle absolute left-1/2 top-1/2 rounded-full bg-white/90"
          style={{
            width: p.size,
            height: p.size,
            ["--tx" as string]: `${p.tx}px`,
            ["--ty" as string]: `${p.ty}px`,
            animationDelay: `${500 + p.delay}ms`,
          }}
        />
      ))}

      {label && (
        <span className="wm-reveal__label absolute -top-7 right-0 whitespace-nowrap rounded bg-black/75 px-2 py-0.5 text-[11px] font-medium text-white">
          {label}
        </span>
      )}
    </div>
  );
}

function Sparkle({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className={className}>
      <path d="M12 1.5c.9 5.7 4.8 9.6 10.5 10.5-5.7.9-9.6 4.8-10.5 10.5C11.1 16.8 7.2 12.9 1.5 12 7.2 11.1 11.1 7.2 12 1.5Z" />
    </svg>
  );
}

/** Keyframes live with the component — the effect is only used here. */
const wmRevealStyles = `
@keyframes wm-bracket-in {
  from { opacity: 0; transform: scale(1.6); }
  to   { opacity: 1; transform: scale(1); }
}
@keyframes wm-sparkle-flare {
  0%   { opacity: 0.85; transform: translate(-50%, -50%) scale(0.55); filter: blur(0.5px); }
  38%  { opacity: 1;    transform: translate(-50%, -50%) scale(1.08); filter: blur(0px); }
  58%  { opacity: 1;    transform: translate(-50%, -50%) scale(1); }
  86%  { opacity: 0.35; transform: translate(-50%, -50%) scale(1.25); filter: blur(1.5px); }
  100% { opacity: 0;    transform: translate(-50%, -50%) scale(1.55); filter: blur(2.5px); }
}
@keyframes wm-particle-fly {
  0%   { opacity: 0; transform: translate(-50%, -50%) scale(0.6); }
  18%  { opacity: 0.95; }
  100% { opacity: 0; transform: translate(calc(-50% + var(--tx)), calc(-50% + var(--ty))) scale(0.2); }
}
@keyframes wm-scan-sweep {
  0%   { transform: translateX(-120%); opacity: 0; }
  30%  { opacity: 0.9; }
  100% { transform: translateX(380%); opacity: 0; }
}
@keyframes wm-label-in {
  from { opacity: 0; transform: translateY(4px); }
  to   { opacity: 1; transform: translateY(0); }
}
.wm-reveal__bracket {
  border-color: rgb(255 255 255 / 0.95);
  box-shadow: 0 0 8px rgb(96 165 250 / 0.8);
  opacity: 0;
  animation: wm-bracket-in 260ms cubic-bezier(0.22, 1, 0.36, 1) forwards;
}
.wm-reveal__bracket--tl, .wm-reveal__bracket--br { animation-delay: 60ms; }
.wm-reveal__bracket--tr, .wm-reveal__bracket--bl { animation-delay: 130ms; }
.wm-reveal__sparkle { animation: wm-sparkle-flare 1.15s cubic-bezier(0.3, 0.6, 0.3, 1) 350ms forwards; opacity: 0; }
.wm-reveal__particle { opacity: 0; animation: wm-particle-fly 620ms cubic-bezier(0.16, 1, 0.3, 1) forwards; }
.wm-reveal__scanline {
  background: linear-gradient(90deg, transparent, rgb(147 197 253 / 0.35), transparent);
  animation: wm-scan-sweep 1s cubic-bezier(0.4, 0, 0.2, 1) 250ms forwards;
  opacity: 0;
}
.wm-reveal__label { animation: wm-label-in 220ms ease-out 80ms backwards; }
@media (prefers-reduced-motion: reduce) {
  .wm-reveal__bracket, .wm-reveal__sparkle, .wm-reveal__particle, .wm-reveal__scanline, .wm-reveal__label {
    animation: none !important;
    opacity: 0 !important;
  }
}
`;
