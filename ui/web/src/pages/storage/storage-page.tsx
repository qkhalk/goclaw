import { useState, useEffect, useCallback, useMemo } from "react";
import { RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { buildTree, formatSize } from "@/lib/file-helpers";
import { FileBrowser } from "@/components/shared/file-browser";
import { useStorage } from "./hooks/use-storage";

export function StoragePage() {
  const { files, totalSize, baseDir, loading, listFiles, readFile, deleteFile } = useStorage();

  const [activePath, setActivePath] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<{ content: string; path: string; size: number } | null>(null);
  const [contentLoading, setContentLoading] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<{ path: string; isDir: boolean } | null>(null);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => { listFiles(); }, [listFiles]);

  const tree = useMemo(() => buildTree(files), [files]);

  const handleSelect = useCallback(async (path: string) => {
    setActivePath(path);
    setContentLoading(true);
    try {
      const res = await readFile(path);
      setFileContent(res);
    } catch {
      setFileContent(null);
    } finally {
      setContentLoading(false);
    }
  }, [readFile]);

  const handleDeleteRequest = useCallback((path: string, isDir: boolean) => {
    setDeleteTarget({ path, isDir });
  }, []);

  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteTarget) return;
    setDeleting(true);
    try {
      await deleteFile(deleteTarget.path);
      // Clear content if the deleted file was being viewed
      if (activePath === deleteTarget.path || (deleteTarget.isDir && activePath?.startsWith(deleteTarget.path + "/"))) {
        setActivePath(null);
        setFileContent(null);
      }
      await listFiles();
    } finally {
      setDeleting(false);
      setDeleteTarget(null);
    }
  }, [deleteTarget, deleteFile, listFiles, activePath]);

  const deleteName = deleteTarget?.path.split("/").pop() ?? "";

  return (
    <div className="flex flex-col h-full p-4 sm:p-6">
      <PageHeader
        title="Storage"
        description={baseDir ? `${baseDir} — ${formatSize(totalSize)}` : "Workspace file browser"}
        actions={
          <Button variant="outline" size="sm" onClick={listFiles} disabled={loading}>
            <RefreshCw className={`h-4 w-4 mr-1.5 ${loading ? "animate-spin" : ""}`} />
            Refresh
          </Button>
        }
      />

      <div className="mt-4 flex-1 flex flex-col min-h-0">
        <FileBrowser
          tree={tree}
          filesLoading={loading}
          activePath={activePath}
          onSelect={handleSelect}
          contentLoading={contentLoading}
          fileContent={fileContent}
          onDelete={handleDeleteRequest}
          showSize
        />
      </div>

      {/* Delete confirmation dialog */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {deleteTarget?.isDir ? "folder" : "file"}</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete <span className="font-semibold text-foreground">{deleteName}</span>?
              {deleteTarget?.isDir && " All contents will be removed recursively."}
              {" "}This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)} disabled={deleting}>
              Cancel
            </Button>
            <Button variant="destructive" onClick={handleDeleteConfirm} disabled={deleting}>
              {deleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
