import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useDraggable, useDroppable } from "@dnd-kit/core";
import { EllipsisVertical, Folder } from "lucide-react";
import { FileIcon } from "@/components/shared/file-tree-file-icon";
import { InlineEditText } from "@/components/ui/inline-edit-text";
import { Checkbox } from "@/components/ui/checkbox";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { formatFileSize, formatRelativeTime } from "@/lib/format";
import type { CloudFileEntry } from "../hooks/use-cloud";
import { buildItemActions, type ItemAction, type ItemActionHandlers } from "./drive-item-menu";

/** One cloud file entry for grid (card) and list (row) mode: selection
 * checkbox, ⋮ actions menu, right-click context menu (same action list),
 * inline rename and dnd (entries draggable, folders also drop targets).
 * Drag + drop refs sit on the same element so both card <div> and row <tr>
 * stay valid HTML. */

function EntryIcon({ entry }: { entry: CloudFileEntry }) {
  if (entry.is_dir) return <Folder className="h-5 w-5 shrink-0 fill-sky-100 text-sky-500 dark:fill-sky-950" />;
  return (
    <span className="flex h-5 w-5 shrink-0 items-center justify-center [&>svg]:h-4 [&>svg]:w-4">
      <FileIcon name={entry.name} />
    </span>
  );
}

/** Shared dnd behaviour for one item: draggable always (when enabled),
 * folders additionally droppable. Returns props to apply on the item root. */
function useItemDnd({
  entry,
  path,
  disabled,
}: {
  entry: CloudFileEntry;
  path: string;
  disabled: boolean;
}) {
  const drag = useDraggable({
    id: `item:${entry.name}`,
    disabled,
    data: { name: entry.name, fromPath: path },
  });
  const drop = useDroppable({
    id: `folder:${entry.name}`,
    disabled: disabled || !entry.is_dir,
    data: { folderPath: path },
  });

  const ref = (node: HTMLElement | null) => {
    drag.setNodeRef(node);
    if (entry.is_dir) drop.setNodeRef(node);
  };

  return {
    ref,
    isDragging: drag.isDragging,
    isOver: drop.isOver,
    spread: disabled ? {} : { ...drag.listeners, ...drag.attributes },
  };
}

/** Render the shared action list with a separator before the destructive group. */
function renderActions(
  items: ItemAction[],
  kind: "dropdown" | "context",
): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  items.forEach((a, i) => {
    const prev = items[i - 1];
    if (i > 0 && Boolean(a.destructive) !== Boolean(prev?.destructive)) {
      out.push(
        kind === "dropdown" ? (
          <DropdownMenuSeparator key={`sep-${i}`} />
        ) : (
          <ContextMenuSeparator key={`sep-${i}`} />
        ),
      );
    }
    if (kind === "dropdown") {
      out.push(
        <DropdownMenuItem
          key={a.key}
          variant={a.destructive ? "destructive" : "default"}
          onSelect={() => a.onSelect()}
        >
          <a.icon className="h-4 w-4" />
          {a.label}
        </DropdownMenuItem>,
      );
    } else {
      out.push(
        <ContextMenuItem
          key={a.key}
          variant={a.destructive ? "destructive" : "default"}
          onSelect={() => a.onSelect()}
        >
          <a.icon className="h-4 w-4" />
          {a.label}
        </ContextMenuItem>,
      );
    }
  });
  return out;
}

function SelectCheckbox({
  entry,
  selected,
  anySelected,
  onToggle,
}: {
  entry: CloudFileEntry;
  selected: boolean;
  anySelected: boolean;
  onToggle: () => void;
}) {
  return (
    <span
      className={cn("shrink-0", anySelected ? "opacity-100" : "opacity-0 group-hover:opacity-100")}
      onClick={(e) => e.stopPropagation()}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={() => onToggle()}
        aria-label={entry.name}
        className="[@media(pointer:coarse)]:size-5"
      />
    </span>
  );
}

function RenameText({
  entry,
  renaming,
  onRename,
  stopRenaming,
  className,
}: {
  entry: CloudFileEntry;
  renaming: boolean;
  onRename: (newName: string) => Promise<void>;
  stopRenaming: () => void;
  className?: string;
}) {
  if (renaming) {
    return (
      <span className="min-w-0 flex-1" onClick={(e) => e.stopPropagation()}>
        <InlineEditText
          value={entry.name}
          onSave={async (v) => {
            await onRename(v);
            stopRenaming();
          }}
          ariaLabel={entry.name}
          className={className}
        />
      </span>
    );
  }
  return (
    <span className={cn("min-w-0 flex-1", className)} title={entry.name}>
      {entry.name}
    </span>
  );
}

export interface DriveItemProps {
  entry: CloudFileEntry;
  /** Full remote path of this entry (encoded form). */
  path: string;
  canWrite: boolean;
  selected: boolean;
  anySelected: boolean;
  /** Item click (shift/selection semantics handled by the area). */
  onActivate: (e: React.MouseEvent) => void;
  onToggleSelect: () => void;
  /** Operation handlers. onRename saves the inline rename edit (the menu item
   * itself opens the editor). */
  handlers: Omit<ItemActionHandlers, "onRename"> & {
    onRename?: (newName: string) => Promise<void>;
  };
  dndEnabled: boolean;
}

function useItemController({
  entry,
  path,
  canWrite,
  handlers,
  dndEnabled,
}: DriveItemProps) {
  const { t } = useTranslation("cloud");
  const [renaming, setRenaming] = useState(false);
  const dnd = useItemDnd({ entry, path, disabled: !dndEnabled || !canWrite });

  // The rename menu action opens the inline editor; everything else delegates.
  const menuHandlers: ItemActionHandlers = {
    ...handlers,
    onRename: canWrite ? () => setRenaming(true) : undefined,
  };
  const items = buildItemActions(entry, canWrite, menuHandlers, t);

  return { t, renaming, setRenaming, dnd, items };
}

/** Full-featured grid card. */
export function DriveGridItem(props: DriveItemProps) {
  const { entry, selected, anySelected, onActivate, onToggleSelect, handlers } = props;
  const { t, renaming, setRenaming, dnd, items } = useItemController(props);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <div
          ref={dnd.ref as React.Ref<HTMLDivElement>}
          {...dnd.spread}
          role="button"
          tabIndex={0}
          onClick={onActivate}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") onActivate(e as unknown as React.MouseEvent);
          }}
          className={cn(
            "group relative flex min-h-[52px] cursor-pointer items-center gap-2 rounded-lg border p-2 transition-colors hover:bg-muted/40",
            selected && "border-primary/60 bg-primary/5",
            dnd.isDragging && "opacity-40",
            dnd.isOver && "ring-2 ring-primary/60",
          )}
        >
          <SelectCheckbox entry={entry} selected={selected} anySelected={anySelected} onToggle={onToggleSelect} />
          <EntryIcon entry={entry} />
          <RenameText
            entry={entry}
            renaming={renaming}
            onRename={handlers.onRename ?? (async () => {})}
            stopRenaming={() => setRenaming(false)}
            className="block truncate text-sm font-medium"
          />
          <span className="shrink-0 text-xs text-muted-foreground">
            {entry.is_dir ? formatRelativeTime(entry.mod_time) : formatFileSize(entry.size)}
          </span>
          <ActionMenuTrigger items={items} label={t("files.menu")} />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent onClick={(e) => e.stopPropagation()}>
        {renderActions(items, "context")}
      </ContextMenuContent>
    </ContextMenu>
  );
}

/** Full-featured table row. */
export function DriveTableRow(props: DriveItemProps) {
  const { entry, selected, anySelected, onActivate, onToggleSelect, handlers } = props;
  const { t, renaming, setRenaming, dnd, items } = useItemController(props);

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <tr
          ref={dnd.ref as React.Ref<HTMLTableRowElement>}
          {...dnd.spread}
          onClick={onActivate}
          className={cn(
            "group cursor-pointer border-b last:border-0 hover:bg-muted/40",
            selected && "bg-primary/5",
            dnd.isDragging && "opacity-40",
            dnd.isOver && "bg-primary/10",
          )}
        >
          <td className="py-1.5 pl-2 pr-2">
            <div className="flex items-center gap-2">
              <SelectCheckbox entry={entry} selected={selected} anySelected={anySelected} onToggle={onToggleSelect} />
              <EntryIcon entry={entry} />
              <RenameText
                entry={entry}
                renaming={renaming}
                onRename={handlers.onRename ?? (async () => {})}
                stopRenaming={() => setRenaming(false)}
                className="block max-w-[320px] truncate text-sm"
              />
            </div>
          </td>
          <td className="px-2 text-right text-xs tabular-nums text-muted-foreground">
            {entry.is_dir ? "—" : formatFileSize(entry.size)}
          </td>
          <td className="px-2 text-xs text-muted-foreground">{formatRelativeTime(entry.mod_time)}</td>
          <td className="w-10 py-1 pr-1 text-right">
            <ActionMenuTrigger items={items} label={t("files.menu")} />
          </td>
        </tr>
      </ContextMenuTrigger>
      <ContextMenuContent onClick={(e) => e.stopPropagation()}>
        {renderActions(items, "context")}
      </ContextMenuContent>
    </ContextMenu>
  );
}

function ActionMenuTrigger({ items, label }: { items: ItemAction[]; label: string }) {
  if (items.length === 0) return null;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={label}
          title={label}
          aria-haspopup="menu"
          className={cn(
            "shrink-0 rounded p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
            "[@media(pointer:coarse)]:min-h-[44px] [@media(pointer:coarse)]:min-w-[44px]",
          )}
          onClick={(e) => e.stopPropagation()}
        >
          <EllipsisVertical className="h-4 w-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
        {renderActions(items, "dropdown")}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
