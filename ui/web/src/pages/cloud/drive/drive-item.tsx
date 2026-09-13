import { Folder } from "lucide-react";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { cn } from "@/lib/utils";
import { formatFileSize, formatRelativeTime } from "@/lib/format";
import type { CloudFileEntry } from "../hooks/use-cloud";

/** Shared visuals for one cloud file entry in grid (card) and list (row) mode.
 * `leading`/`trailing` slots are filled by the shell (selection checkbox,
 * actions menu, drag handle). */

function EntryIcon({ entry }: { entry: CloudFileEntry }) {
  if (entry.is_dir) return <Folder className="h-5 w-5 shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950" />;
  return (
    <span className="flex h-5 w-5 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
      <FileIcon name={entry.name} />
    </span>
  );
}

export function DriveGridItem({
  entry,
  onOpen,
  selected,
  leading,
  trailing,
}: {
  entry: CloudFileEntry;
  onOpen?: () => void;
  selected?: boolean;
  leading?: React.ReactNode;
  trailing?: React.ReactNode;
}) {
  return (
    <div
      className={cn(
        "group relative flex min-h-[52px] items-center gap-2 rounded-lg border p-2 transition-colors",
        onOpen && "cursor-pointer hover:bg-muted/40",
        selected && "border-primary/60 bg-primary/5",
      )}
      role={onOpen ? "button" : undefined}
      tabIndex={onOpen ? 0 : undefined}
      onClick={onOpen}
      onKeyDown={
        onOpen
          ? (e) => {
              if (e.key === "Enter" || e.key === " ") onOpen();
            }
          : undefined
      }
    >
      {leading}
      <EntryIcon entry={entry} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium" title={entry.name}>
          {entry.name}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {entry.is_dir
            ? formatRelativeTime(entry.mod_time)
            : `${formatFileSize(entry.size)} · ${formatRelativeTime(entry.mod_time)}`}
        </span>
      </span>
      {trailing}
    </div>
  );
}

export function DriveTableRow({
  entry,
  onOpen,
  selected,
  leading,
  trailing,
}: {
  entry: CloudFileEntry;
  onOpen?: () => void;
  selected?: boolean;
  leading?: React.ReactNode;
  trailing?: React.ReactNode;
}) {
  return (
    <tr
      className={cn(
        "border-b last:border-0",
        onOpen && "cursor-pointer hover:bg-muted/40",
        selected && "bg-primary/5",
      )}
      onClick={onOpen}
    >
      <td className="py-2 pl-2 pr-2">
        <div className="flex items-center gap-2">
          {leading}
          <EntryIcon entry={entry} />
          <span className="max-w-[320px] truncate text-sm" title={entry.name}>
            {entry.name}
          </span>
        </div>
      </td>
      <td className="px-2 text-right text-xs tabular-nums text-muted-foreground">
        {entry.is_dir ? "—" : formatFileSize(entry.size)}
      </td>
      <td className="px-2 text-xs text-muted-foreground">{formatRelativeTime(entry.mod_time)}</td>
      <td className="w-10 py-1 pr-1 text-right">{trailing}</td>
    </tr>
  );
}
