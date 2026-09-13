import { DriveGridItem } from "./drive-item";
import type { CloudFileEntry } from "../hooks/use-cloud";

/** Grid view of the current folder (mobile-first responsive card grid). */
export function DriveGrid({
  entries,
  onOpenFolder,
  renderItem,
}: {
  entries: CloudFileEntry[];
  onOpenFolder?: (entry: CloudFileEntry) => void;
  /** P5: override the item renderer (selection + menu + dnd). */
  renderItem?: (entry: CloudFileEntry) => React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {entries.map((e) =>
        renderItem ? (
          renderItem(e)
        ) : (
          <DriveGridItem
            key={e.name}
            entry={e}
            onOpen={e.is_dir ? () => onOpenFolder?.(e) : undefined}
          />
        ),
      )}
    </div>
  );
}
