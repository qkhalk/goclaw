// Read-only lazy file explorer side panel for the chat console (Paseo §24).
// write/mkdir actions arrive with editor integration.
import {
  ChevronDown,
  ChevronRight,
  FileText,
  Folder,
  FolderOpen,
  Loader2,
  RefreshCw,
  AlertTriangle,
  X,
} from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Methods } from "@/api/protocol";
import { useWs } from "@/hooks/use-ws";
import { cn } from "@/lib/utils";
import type { FileEntry } from "@/types/workspace";
import { type UseFileTreeResult, useFileTree } from "@/pages/chat/hooks/use-file-tree";

interface FileExplorerPanelProps {
  open: boolean;
  onClose: () => void;
  workspaceId: string | null;
}

interface LoadedFile {
  path: string;
  content: string;
}

export function FileExplorerPanel({ open, onClose, workspaceId }: FileExplorerPanelProps) {
  const { t } = useTranslation("chat");
  const ws = useWs();
  const tree = useFileTree(workspaceId);
  const [file, setFile] = useState<LoadedFile | null>(null);
  const [fileError, setFileError] = useState<string | null>(null);

  if (!open) return null;

  const openFile = async (entry: FileEntry) => {
    if (!workspaceId) return;
    setFileError(null);
    setFileLoading(true);
    try {
      const res = await ws.call<{ path: string; content: string }>(
        Methods.WORKSPACE_FILES_READ,
        { workspaceId, path: entry.path },
      );
      setFile({ path: res.path ?? entry.path, content: res.content });
    } catch (err) {
      setFile(null);
      setFileError(err instanceof Error ? err.message : String(err));
    } finally {
      setFileLoading(false);
    }
  };

  return (
    <div className={cn(
      "flex h-full w-72 shrink-0 flex-col border-l bg-background",
      "max-sm:fixed max-sm:inset-y-0 max-sm:right-0 max-sm:z-50 max-sm:w-full max-sm:max-w-[85vw] max-sm:shadow-xl",
    )}>
      {/* Header */}
      <div className="flex items-center justify-between border-b px-3 py-2">
        <span className="text-sm font-medium">{t("fileExplorer.title")}</span>
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={() => void tree.loadDir("", { force: true })}
            disabled={!workspaceId}
            title={t("fileExplorer.refresh")}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
          >
            <RefreshCw className="h-4 w-4" />
          </button>
          <button
            type="button"
            onClick={() => {
              setFile(null);
              onClose();
            }}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Body */}
      {!workspaceId ? (
        <p className="px-3 py-6 text-center text-xs text-muted-foreground">
          {t("fileExplorer.noWorkspace")}
        </p>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col">
          <div
            role="tree"
            aria-label={t("fileExplorer.title")}
            className="flex-1 overflow-y-auto overscroll-contain p-1"
          >
            {tree.error && (
              <p className="flex items-start gap-1.5 px-2 py-2 text-xs text-destructive">
                <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span className="min-w-0 break-words">{tree.error}</span>
              </p>
            )}
            {!tree.rootLoaded && !tree.error ? (
              <p className="flex items-center gap-1.5 px-2 py-2 text-xs text-muted-foreground">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                {t("fileExplorer.loading")}
              </p>
            ) : (
              <TreeChildren tree={tree} path="" depth={0} onOpenFile={openFile} />
            )}
          </div>

          {fileError && (
            <p className="flex items-start gap-1.5 px-2 py-1.5 text-xs text-destructive">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span className="min-w-0 break-words">{fileError}</span>
            </p>
          )}
          {/* File detail view */}
          {(file || fileLoading) && (
            <div className="flex max-h-[45%] shrink-0 flex-col border-t">
              <div className="flex items-center justify-between border-b px-2 py-1">
                <span
                  className="min-w-0 truncate font-mono text-xs-plus text-muted-foreground"
                  title={file?.path}
                >
                  {file ? t("fileExplorer.viewing", { path: file.path }) : t("fileExplorer.loadingFile")}
                </span>
                <div className="ml-2 flex shrink-0 items-center gap-0.5">
                  {fileLoading && <Loader2 className="h-3.5 w-3.5 animate-spin text-muted-foreground" />}
                  <button
                    type="button"
                    onClick={() => setFile(null)}
                    aria-label={t("fileExplorer.closeFile")}
                    className="rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
              </div>
              {file && (
                <pre className="flex-1 overflow-auto overscroll-contain whitespace-pre p-2 font-mono text-xs-plus leading-snug">
                  {file.content}
                </pre>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

/** Child rows of a directory listing plus its empty/loading states. */
function TreeChildren({
  tree,
  path,
  depth,
  onOpenFile,
}: {
  tree: UseFileTreeResult;
  path: string;
  depth: number;
  onOpenFile: (entry: FileEntry) => void;
}) {
  const { t } = useTranslation("chat");
  const entries = tree.nodes[path];

  if (entries === undefined) {
    return (
      <p className="flex items-center gap-1.5 px-2 py-2 text-xs text-muted-foreground">
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
        {t("fileExplorer.loading")}
      </p>
    );
  }
  if (entries.length === 0) {
    return <p className="px-2 py-2 text-xs text-muted-foreground">{t("fileExplorer.emptyDir")}</p>;
  }

  return (
    <>
      {entries.map((entry) => (
        <TreeRow key={entry.path} entry={entry} depth={depth} tree={tree} onOpenFile={onOpenFile} />
      ))}
    </>
  );
}

function TreeRow({
  entry,
  depth,
  tree,
  onOpenFile,
}: {
  entry: FileEntry;
  depth: number;
  tree: UseFileTreeResult;
  onOpenFile: (entry: FileEntry) => void;
}) {
  const { t } = useTranslation("chat");
  const isDir = entry.type === "dir";
  const isExpanded = tree.expanded.has(entry.path);
  const isLoading = Boolean(tree.loading[entry.path]);
  // Depth derives from slash count so restored cache rows stay aligned.
  const indent = Math.max(depth, entry.path.split("/").length - 1);

  return (
    <div>
      <button
        type="button"
        role="treeitem"
        aria-expanded={isDir ? isExpanded : undefined}
        onClick={() => (isDir ? tree.toggle(entry.path) : onOpenFile(entry))}
        style={{ paddingLeft: `${8 + indent * 14}px` }}
        className="flex min-h-[44px] w-full items-center gap-1.5 pr-2 text-left text-sm hover:bg-accent hover:text-accent-foreground md:min-h-0 md:py-1 md:text-xs-plus"
      >
        {isDir ? (
          <>
            {isExpanded
              ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
              : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />}
            {isExpanded
              ? <FolderOpen className="h-4 w-4 shrink-0 text-muted-foreground" />
              : <Folder className="h-4 w-4 shrink-0 text-muted-foreground" />}
          </>
        ) : (
          <>
            <span className="w-3.5 shrink-0" />
            <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
          </>
        )}
        <span className="min-w-0 truncate">{entry.name}</span>
        {isLoading && <Loader2 className="ml-auto h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />}
        {!isLoading && isDir && isExpanded && tree.truncated[entry.path] && (
          <span className="ml-auto shrink-0 rounded bg-muted px-1 py-0.5 text-2xs tabular-nums text-muted-foreground">
            {t("fileExplorer.truncated")}
          </span>
        )}
      </button>
      {isDir && isExpanded && (
        <TreeChildren tree={tree} path={entry.path} depth={depth + 1} onOpenFile={onOpenFile} />
      )}
    </div>
  );
}
