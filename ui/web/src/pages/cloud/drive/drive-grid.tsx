import type { CloudFileEntry } from "../hooks/use-cloud";

/** Grid view of the current folder (mobile-first responsive card grid). */
export function DriveGrid({
  entries,
  renderItem,
}: {
  entries: CloudFileEntry[];
  renderItem: (entry: CloudFileEntry, index: number) => React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {entries.map((e, i) => renderItem(e, i))}
    </div>
  );
}
