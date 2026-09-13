import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { DndContext, PointerSensor, KeyboardSensor, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { ArrowRight, ArrowRightLeft, Download, FolderPlus, KeyRound, Loader2, Trash2, Upload, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { DropZone } from "@/components/shared/drop-zone";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useHttp } from "@/hooks/use-ws";
import { queryKeys } from "@/lib/query-keys";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/stores/use-auth-store";
import { toast } from "@/stores/use-toast-store";
import {
  useCloudAccounts,
  useCloudFileOps,
  useCloudStarred,
  type CloudAccount,
  type CloudFileEntry,
  type CloudProvider,
} from "../hooks/use-cloud";
import { DriveShell } from "./drive-shell";
import { PreviewSheet, type PreviewFile } from "./preview-sheet";
import { ShareLinkDialog } from "./share-link-dialog";
import { ShortcutsDialog } from "./shortcuts-dialog";
import { useDriveShortcuts } from "./use-drive-shortcuts";
import { pushRecent } from "@/lib/cloud-recent";
import { DriveTopBar } from "./drive-topbar";
import { DriveGrid } from "./drive-grid";
import { DriveTable } from "./drive-table";
import { DriveGridItem, DriveTableRow } from "./drive-item";
import { NewFolderDialog } from "./new-folder-dialog";
import { MoveCopyDialog } from "./move-copy-dialog";
import { TransferDialog } from "./transfer-dialog";
import { useSelection } from "./use-selection";
import { useCloudUploads } from "./use-cloud-uploads";
import { UploadPanel } from "./upload-panel";
import { opErrorToast } from "./op-error";
import { childPath, parentPath, type SortSpec, type ViewMode } from "./paths";

/** File area of the Drive shell for one account: browsing + the full set of
 * file operations (upload w/ progress, mkdir, rename, move, copy, permanent
 * delete, download), multi-select with bulk actions, drag-move and the
 * read-only/re-grant state for accounts without write scopes. */
export function DriveFileArea({
  account,
  path,
  onNavigatePath,
  search,
  onSearchChange,
  sort,
  onSortChange,
  viewMode,
  onViewModeChange,
  onRefresh,
  right,
}: {
  account: CloudAccount;
  path: string;
  onNavigatePath: (path: string) => void;
  search: string;
  onSearchChange: (v: string) => void;
  sort: SortSpec;
  onSortChange: (s: SortSpec) => void;
  viewMode: ViewMode;
  onViewModeChange: (m: ViewMode) => void;
  onRefresh: () => void;
  /** Extra top-bar actions from the page (settings gear). */
  right?: React.ReactNode;
}) {
  const { t } = useTranslation("cloud");
  const { t: tc } = useTranslation("common");
  const http = useHttp();
  const userId = useAuthStore((s) => s.userId);
  const role = useAuthStore((s) => s.role);
  const isAdmin = role === "admin" || role === "owner";

  const ops = useCloudFileOps(account.id);
  const { accounts, startConnect, completeConnect } = useCloudAccounts();
  const uploads = useCloudUploads(account.id);
  const starred = useCloudStarred();

  // Write access mirrors the backend guard: write OAuth scopes + owner/admin.
  const canWrite =
    account.can_write !== false && (account.user_id === userId || isAdmin);
  const canRegrant = !account.shared || account.user_id === userId;
  // Cross-account transfer is only meaningful with somewhere to go.
  const canTransfer = accounts.length > 1;

  const files = useQuery({
    queryKey: queryKeys.cloud.files(account.id, path),
    staleTime: 30_000,
    queryFn: () =>
      http.get<{ path: string; entries: CloudFileEntry[] }>(
        `/v1/cloud/accounts/${account.id}/files?path=${encodeURIComponent(path)}&limit=200`,
      ),
  });

  const entries = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = (files.data?.entries ?? []).filter((e) => !q || e.name.toLowerCase().includes(q));
    list.sort((a, b) => {
      if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1;
      let cmp = 0;
      switch (sort.key) {
        case "size":
          cmp = a.size - b.size;
          break;
        case "modified":
          cmp = new Date(a.mod_time).getTime() - new Date(b.mod_time).getTime();
          break;
        default:
          cmp = a.name.localeCompare(b.name);
      }
      return sort.dir === "asc" ? cmp : -cmp;
    });
    return list;
  }, [files.data, search, sort]);

  const visibleNames = useMemo(() => entries.map((e) => e.name), [entries]);
  const selection = useSelection(path);

  // ---- dialogs / menus state -------------------------------------------------
  const [newFolderOpen, setNewFolderOpen] = useState(false);
  const [moveCopy, setMoveCopy] = useState<{ mode: "move" | "copy"; sources: string[] } | null>(null);
  const [transferSources, setTransferSources] = useState<CloudFileEntry[] | null>(null);
  const [deleteTargets, setDeleteTargets] = useState<CloudFileEntry[] | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [previewIndex, setPreviewIndex] = useState<number | null>(null);
  const [shareTarget, setShareTarget] = useState<CloudFileEntry | null>(null);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // ---- re-grant flow (embedded client paste-back) -----------------------------
  const [connecting, setConnecting] = useState(false);
  const [pasteOpen, setPasteOpen] = useState(false);
  const [pasteURL, setPasteURL] = useState("");
  const [completing, setCompleting] = useState(false);
  const [pasteError, setPasteError] = useState("");

  // ---- handlers ----------------------------------------------------------------

  /** Record one entry in the device-local recent list (no backend). */
  function recordRecent(entry: CloudFileEntry, entryPath: string) {
    pushRecent(userId, {
      accountId: account.id,
      provider: account.provider,
      email: account.email,
      path: entryPath,
      name: entry.name,
      isDir: entry.is_dir,
    });
  }

  const previewFiles: PreviewFile[] = entries
    .filter((e) => !e.is_dir)
    .map((e) => ({ entry: e, path: childPath(path, e.name) }));

  function openPreview(entry: CloudFileEntry) {
    const idx = previewFiles.findIndex((f) => f.entry.name === entry.name);
    if (idx < 0) return;
    const entryPath = childPath(path, entry.name);
    recordRecent(entry, entryPath);
    setPreviewIndex(idx);
  }

  function handleActivate(entry: CloudFileEntry, index: number, e: React.MouseEvent) {
    if (e.shiftKey) {
      selection.handleClick(entry.name, index, true, visibleNames);
      return;
    }
    if (entry.is_dir) {
      const next = childPath(path, entry.name);
      recordRecent(entry, next);
      onNavigatePath(next);
      return;
    }
    // Drive-style: a plain click on a file previews it (multi-select stays on
    // the checkboxes / shift-click).
    selection.setCursor(index);
    openPreview(entry);
  }

  async function handleRename(entry: CloudFileEntry, newName: string) {
    const trimmed = newName.trim();
    if (!trimmed || trimmed === entry.name) return;
    try {
      await ops.move(childPath(path, entry.name), childPath(path, trimmed));
      toast.success(t("files.done"));
    } catch (e) {
      opErrorToast(e, t);
      throw e;
    }
  }

  async function downloadBlob(entry: CloudFileEntry) {
    recordRecent(entry, childPath(path, entry.name));
    try {
      const blob = await ops.download(childPath(path, entry.name));
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = entry.name;
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      opErrorToast(e, t);
    }
  }

  async function handleBulkDelete() {
    if (!deleteTargets) return;
    setDeleting(true);
    try {
      for (const entry of deleteTargets) {
        try {
          await ops.remove(childPath(path, entry.name), entry.is_dir);
        } catch (e) {
          opErrorToast(e, t);
          return;
        }
      }
      toast.success(t("files.done"));
      selection.clear();
      setDeleteTargets(null);
    } finally {
      setDeleting(false);
    }
  }

  async function handleBulkDownload() {
    const files = entries.filter((e) => !e.is_dir && selection.has(e.name));
    for (const entry of files) {
      await downloadBlob(entry);
    }
  }

  // Ctrl/Cmd+A selects everything visible (unless typing in a field).
  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== "a") return;
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.isContentEditable)) return;
      e.preventDefault();
      selection.selectAll(visibleNames);
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [visibleNames, selection]);

  // ---- keyboard shortcuts (window-level, input-guarded) ---------------------
  const entriesAt = (i: number) => entries[i];

  useDriveShortcuts(
    {
      onCursorMove: (delta) => selection.moveCursor(delta, entries.length),
      onOpen: () => {
        const entry = entriesAt(selection.cursor);
        if (!entry) return;
        if (entry.is_dir) {
          const next = childPath(path, entry.name);
          recordRecent(entry, next);
          onNavigatePath(next);
        } else {
          openPreview(entry);
        }
      },
      onUp: () => onNavigatePath(parentPath(path)),
      onDelete: () => {
        const targets =
          selectedEntries.length > 0
            ? selectedEntries
            : [entriesAt(selection.cursor)].filter(
                (e): e is CloudFileEntry => !!e && canWrite,
              );
        if (targets.length > 0) setDeleteTargets(targets);
      },
      onSelectAll: () => selection.selectAll(visibleNames),
      onSearch: () =>
        (document.querySelector("[data-cloud-search]") as HTMLInputElement | null)?.focus(),
      onHelp: () => setShortcutsOpen((v) => !v),
      onClear: () => selection.clear(),
    },
    true,
  );

  // ---- dnd -----------------------------------------------------------------------
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor),
  );

  function handleDragEnd(e: DragEndEvent) {
    const data = e.active.data.current as { name: string; fromPath: string } | undefined;
    const destFolder = e.over?.data.current?.folderPath as string | undefined;
    if (!data || !destFolder) return;
    if (data.fromPath === destFolder) return;
    // Never drop a folder into itself or its own descendant.
    if (destFolder.startsWith(data.fromPath + "/")) return;
    const to = childPath(destFolder, data.name);
    if (to === data.fromPath) return;
    ops
      .move(data.fromPath, to)
      .then(() => toast.success(t("files.done")))
      .catch((err) => opErrorToast(err, t));
  }

  // ---- re-grant (same flow as the connect button on the clouds home) -------------
  async function handleRegrant() {
    setConnecting(true);
    setPasteError("");
    try {
      const res = await startConnect(account.provider as CloudProvider);
      if (res.mode === "paste") {
        setPasteURL("");
        setPasteOpen(true);
        window.open(res.auth_url, "_blank", "noopener,noreferrer");
      } else {
        window.location.href = res.auth_url;
      }
    } catch {
      setPasteError("");
    } finally {
      setConnecting(false);
    }
  }

  async function handlePasteComplete() {
    if (!pasteURL.trim()) return;
    setCompleting(true);
    setPasteError("");
    try {
      await completeConnect(account.provider as CloudProvider, pasteURL.trim());
      setPasteOpen(false);
      setPasteURL("");
    } catch (e) {
      setPasteError(e instanceof Error ? e.message : String(e));
    } finally {
      setCompleting(false);
    }
  }

  function handleFileInput(files: FileList | null) {
    if (!files || files.length === 0) return;
    uploads.enqueue(Array.from(files), path);
  }

  /** Write controls on a read-only account: explain why and point at the
   * re-grant action (the amber banner keeps the actual "Re-grant access"
   * button). The upload/mkdir itself stays gated — the backend 403s. */
  function handleWriteBlocked() {
    toast.warning(t("files.readonly_banner"), t("files.write_disabled_hint"));
  }

  const selectedEntries = entries.filter((e) => selection.has(e.name));
  const selectedSources = selectedEntries.map((e) => childPath(path, e.name));

  return (
    <>
      <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
        <DriveShell
          accountId={account.id}
          railTitle={t("drive.my_drives")}
          header={
            <DriveTopBar
              path={path}
              onNavigatePath={onNavigatePath}
              rootLabel={t("detail.root")}
              showTools
              search={search}
              onSearchChange={onSearchChange}
              sort={sort}
              onSortChange={onSortChange}
              viewMode={viewMode}
              onViewModeChange={onViewModeChange}
              onRefresh={onRefresh}
              right={
                <>
                  <input
                    ref={fileInputRef}
                    type="file"
                    multiple
                    className="hidden"
                    onChange={(e) => {
                      handleFileInput(e.target.files);
                      e.target.value = "";
                    }}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("files.new_folder")}
                    title={canWrite ? t("files.new_folder") : t("files.write_disabled_hint")}
                    aria-disabled={!canWrite}
                    className={cn(
                      !canWrite &&
                        "text-muted-foreground/60 hover:bg-transparent hover:text-muted-foreground/60 dark:hover:bg-transparent",
                    )}
                    onClick={() => (canWrite ? setNewFolderOpen(true) : handleWriteBlocked())}
                  >
                    <FolderPlus className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("files.upload")}
                    title={canWrite ? t("files.upload") : t("files.write_disabled_hint")}
                    aria-disabled={!canWrite}
                    className={cn(
                      !canWrite &&
                        "text-muted-foreground/60 hover:bg-transparent hover:text-muted-foreground/60 dark:hover:bg-transparent",
                    )}
                    onClick={() => (canWrite ? fileInputRef.current?.click() : handleWriteBlocked())}
                  >
                    <Upload className="h-4 w-4" />
                  </Button>
                  {right}
                </>
              }
            />
          }
        >
          <div className="flex flex-col gap-3 p-3 md:p-4">
            {!canWrite && (
              <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-amber-500/30 bg-amber-500/5 p-2.5 text-xs">
                <span>{t("files.readonly_banner")}</span>
                {canRegrant && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="min-h-11 sm:min-h-8"
                    disabled={connecting}
                    onClick={handleRegrant}
                  >
                    <KeyRound className="mr-2 h-4 w-4" />
                    {t("regrant")}
                  </Button>
                )}
              </div>
            )}

            <DropZone
              onDrop={(files) => {
                if (!canWrite) {
                  handleWriteBlocked();
                  return;
                }
                uploads.enqueue(files, path);
              }}
              title={canWrite ? t("files.drop_overlay") : t("files.write_disabled_hint")}
            >
              {files.isLoading ? (
                <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t("detail.loading_files")}
                </div>
              ) : files.isError ? (
                <p className="py-8 text-sm text-muted-foreground">{t("detail.files_unavailable")}</p>
              ) : entries.length === 0 ? (
                <p className="py-8 text-sm text-muted-foreground">{t("detail.no_files")}</p>
              ) : viewMode === "grid" ? (
                <DriveGrid
                  entries={entries}
                  renderItem={(e, index) => {
                    return (
                      <DriveGridItem
                        key={e.name}
                        entry={e}
                        path={childPath(path, e.name)}
                        accountId={account.id}
                        canWrite={canWrite}
                        selected={selection.has(e.name)}
                        anySelected={selection.count > 0}
                        cursor={selection.cursor === index}
                        starred={starred.isStarred(account.id, childPath(path, e.name))}
                        onActivate={(ev) => handleActivate(e, index, ev)}
                        onToggleSelect={() => selection.handleClick(e.name, index, false, visibleNames)}
                        handlers={{
                          onOpen: e.is_dir ? () => onNavigatePath(childPath(path, e.name)) : undefined,
                          onDownload: e.is_dir ? undefined : () => void downloadBlob(e),
                          onRename: canWrite ? (v) => handleRename(e, v) : undefined,
                          onMove: canWrite
                            ? () => setMoveCopy({ mode: "move", sources: [childPath(path, e.name)] })
                            : undefined,
                          onCopy: canWrite
                            ? () => setMoveCopy({ mode: "copy", sources: [childPath(path, e.name)] })
                            : undefined,
                          onTransfer: canTransfer ? () => setTransferSources([e]) : undefined,
                          onStar: () =>
                            starred
                              .toggle({
                                account_id: account.id,
                                path: childPath(path, e.name),
                                name: e.name,
                                is_dir: e.is_dir,
                              })
                              .catch((err) => opErrorToast(err, t)),
                          starred: starred.isStarred(account.id, childPath(path, e.name)),
                          onShare: canWrite ? () => setShareTarget(e) : undefined,
                          onDelete: canWrite ? () => setDeleteTargets([e]) : undefined,
                        }}
                        dndEnabled
                      />
                    );
                  }}
                />
              ) : (
                <DriveTable
                  entries={entries}
                  renderRow={(e, index) => {
                    return (
                      <DriveTableRow
                        key={e.name}
                        entry={e}
                        path={childPath(path, e.name)}
                        canWrite={canWrite}
                        selected={selection.has(e.name)}
                        anySelected={selection.count > 0}
                        cursor={selection.cursor === index}
                        starred={starred.isStarred(account.id, childPath(path, e.name))}
                        onActivate={(ev) => handleActivate(e, index, ev)}
                        onToggleSelect={() => selection.handleClick(e.name, index, false, visibleNames)}
                        handlers={{
                          onOpen: e.is_dir ? () => onNavigatePath(childPath(path, e.name)) : undefined,
                          onDownload: e.is_dir ? undefined : () => void downloadBlob(e),
                          onRename: canWrite ? (v) => handleRename(e, v) : undefined,
                          onMove: canWrite
                            ? () => setMoveCopy({ mode: "move", sources: [childPath(path, e.name)] })
                            : undefined,
                          onCopy: canWrite
                            ? () => setMoveCopy({ mode: "copy", sources: [childPath(path, e.name)] })
                            : undefined,
                          onTransfer: canTransfer ? () => setTransferSources([e]) : undefined,
                          onStar: () =>
                            starred
                              .toggle({
                                account_id: account.id,
                                path: childPath(path, e.name),
                                name: e.name,
                                is_dir: e.is_dir,
                              })
                              .catch((err) => opErrorToast(err, t)),
                          starred: starred.isStarred(account.id, childPath(path, e.name)),
                          onShare: canWrite ? () => setShareTarget(e) : undefined,
                          onDelete: canWrite ? () => setDeleteTargets([e]) : undefined,
                        }}
                        dndEnabled
                      />
                    );
                  }}
                />
              )}
            </DropZone>
          </div>
        </DriveShell>

        {/* Bulk action bar */}
        {selection.count > 0 && (
          <div className="fixed inset-x-3 bottom-3 z-40 mx-auto flex max-w-lg flex-wrap items-center justify-between gap-2 rounded-lg border bg-background/95 p-2 shadow-lg backdrop-blur safe-bottom md:left-1/2 md:right-auto md:-translate-x-1/2">
            <span className="text-sm font-medium">{t("files.selected_count", { count: selection.count })}</span>
            <div className="flex flex-wrap items-center gap-1">
              <Button size="sm" variant="ghost" className="min-h-11 sm:min-h-8" onClick={() => void handleBulkDownload()}>
                <Download className="mr-1.5 h-4 w-4" />
                {t("files.download")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                className="min-h-11 sm:min-h-8"
                disabled={!canWrite}
                onClick={() => setMoveCopy({ mode: "move", sources: selectedSources })}
              >
                <ArrowRight className="mr-1.5 h-4 w-4" />
                {t("files.move")}
              </Button>
              {canTransfer && (
                <Button
                  size="sm"
                  variant="ghost"
                  className="min-h-11 sm:min-h-8"
                  onClick={() => setTransferSources(selectedEntries)}
                >
                  <ArrowRightLeft className="mr-1.5 h-4 w-4" />
                  {t("transfer.menu_item")}
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                className="min-h-11 text-destructive hover:text-destructive sm:min-h-8"
                disabled={!canWrite}
                onClick={() => setDeleteTargets(selectedEntries)}
              >
                <Trash2 className="mr-1.5 h-4 w-4" />
                {t("files.delete")}
              </Button>
              <Button size="icon-sm" variant="ghost" aria-label={t("files.bulk_clear")} onClick={selection.clear}>
                <X className="h-4 w-4" />
              </Button>
            </div>
          </div>
        )}
      </DndContext>

      <UploadPanel uploads={uploads} />

      <NewFolderDialog
        open={newFolderOpen}
        onOpenChange={setNewFolderOpen}
        accountId={account.id}
        parent={path}
      />

      {moveCopy && (
        <MoveCopyDialog
          open
          onOpenChange={(open) => !open && setMoveCopy(null)}
          accountId={account.id}
          mode={moveCopy.mode}
          sources={moveCopy.sources}
          initialPath={path}
        />
      )}

      {transferSources && (
        <TransferDialog
          open
          onOpenChange={(open) => !open && setTransferSources(null)}
          sourceAccount={account}
          sources={transferSources}
          sourcePath={path}
        />
      )}

      <ConfirmDialog
        open={deleteTargets !== null}
        onOpenChange={(open) => !open && setDeleteTargets(null)}
        title={t("files.delete_confirm_title")}
        description={t("files.delete_confirm_description", { count: deleteTargets?.length ?? 0 })}
        confirmLabel={t("files.delete")}
        variant="destructive"
        loading={deleting}
        onConfirm={() => void handleBulkDelete()}
      />

      {previewIndex !== null && previewFiles.length > 0 && (
        <PreviewSheet
          accountId={account.id}
          files={previewFiles}
          index={Math.min(previewIndex, previewFiles.length - 1)}
          onIndexChange={setPreviewIndex}
          onClose={() => setPreviewIndex(null)}
        />
      )}

      {shareTarget && (
        <ShareLinkDialog
          open
          onOpenChange={(open) => !open && setShareTarget(null)}
          accountId={account.id}
          path={childPath(path, shareTarget.name)}
          name={shareTarget.name}
        />
      )}

      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />

      {/* Paste-back dialog for the embedded client re-grant flow */}
      <Dialog open={pasteOpen} onOpenChange={setPasteOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t("paste.title")}</DialogTitle>
          </DialogHeader>
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">{t("paste.description")}</p>
            <Input
              value={pasteURL}
              onChange={(e) => setPasteURL(e.target.value)}
              placeholder={t("paste.placeholder")}
              className="text-base md:text-sm"
              autoComplete="off"
            />
            {pasteError && <p className="text-xs text-destructive">{pasteError}</p>}
            <div className="flex justify-end gap-2">
              <Button variant="outline" onClick={() => setPasteOpen(false)}>
                {tc("cancel")}
              </Button>
              <Button onClick={() => void handlePasteComplete()} disabled={completing || !pasteURL.trim()}>
                {completing ? t("paste.completing") : t("paste.complete")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
