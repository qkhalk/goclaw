/** Dropbox brand mark (five-diamond glyph), fill = currentColor so it
 * inherits text color like the surrounding lucide icons. */
export function DropboxIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" aria-hidden className={className}>
      <path d="M6 2 0 6.5l6 4.5 6-4.5L6 2Zm12 0-6 4.5 6 4.5 6-4.5L18 2ZM0 15.5 6 20l6-4.5L6 11l-6 4.5ZM18 11l-6 4.5L18 20l6-4.5-6-4.5ZM6 21.25l6 4.5 6-4.5-6-4.5-6 4.5Z" transform="scale(0.85) translate(2.1 -1.5)" />
    </svg>
  );
}
