import { useState, useEffect, useCallback, useMemo } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { MarkdownRenderer } from "@/components/shared/markdown-renderer";
import { useClipboard } from "@/hooks/use-clipboard";
import {
  Folder,
  FolderOpen,
  FileText,
  FileCode2,
  File,
  ChevronRight,
  Check,
  Copy,
  Loader2,
} from "lucide-react";
import type { SkillInfo, SkillFile, SkillVersions } from "@/types/skill";

// --- Types ---

interface SkillDetailDialogProps {
  skill: SkillInfo & { content: string };
  onClose: () => void;
  getSkillVersions: (id: string) => Promise<SkillVersions>;
  getSkillFiles: (id: string, version?: number) => Promise<SkillFile[]>;
  getSkillFileContent: (id: string, path: string, version?: number) => Promise<{ content: string; path: string; size: number }>;
}

interface TreeNode {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  children: TreeNode[];
}

// --- Helpers ---

const CODE_EXTENSIONS = new Set([
  "ts", "tsx", "js", "jsx", "py", "go", "json", "yaml", "yml", "toml",
  "sh", "bash", "css", "html", "sql", "rs", "rb", "java", "kt", "swift",
  "c", "cpp", "h", "xml", "graphql", "proto", "lua", "zig", "env",
]);

function extOf(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot >= 0 ? name.slice(dot + 1).toLowerCase() : "";
}

function langFor(ext: string): string {
  const map: Record<string, string> = {
    ts: "typescript", tsx: "tsx", js: "javascript", jsx: "jsx",
    py: "python", go: "go", rs: "rust", rb: "ruby", sh: "bash", bash: "bash",
    yml: "yaml", yaml: "yaml", json: "json", toml: "toml",
    css: "css", html: "html", sql: "sql", xml: "xml",
    java: "java", kt: "kotlin", swift: "swift",
    c: "c", cpp: "cpp", h: "c", graphql: "graphql",
    proto: "protobuf", lua: "lua", zig: "zig",
  };
  return map[ext] ?? ext;
}

function buildTree(files: SkillFile[]): TreeNode[] {
  const root: TreeNode[] = [];
  const dirMap = new Map<string, TreeNode>();

  // Sort: dirs first, then alphabetically
  const sorted = [...files].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    return a.path.localeCompare(b.path);
  });

  for (const f of sorted) {
    const node: TreeNode = {
      name: f.name,
      path: f.path,
      isDir: f.isDir,
      size: f.size,
      children: [],
    };

    if (f.isDir) {
      dirMap.set(f.path, node);
    }

    // Find parent dir
    const parentPath = f.path.includes("/")
      ? f.path.slice(0, f.path.lastIndexOf("/"))
      : "";

    if (parentPath && dirMap.has(parentPath)) {
      dirMap.get(parentPath)!.children.push(node);
    } else {
      root.push(node);
    }
  }

  // Sort children: dirs first, then alphabetically
  const sortChildren = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    for (const n of nodes) {
      if (n.children.length > 0) sortChildren(n.children);
    }
  };
  sortChildren(root);
  return root;
}

function stripFrontmatter(content: string): string {
  if (!content.startsWith("---")) return content;
  const end = content.indexOf("---", 3);
  if (end < 0) return content;
  return content.slice(end + 3).replace(/^\n+/, "");
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function FileIcon({ name }: { name: string }) {
  const ext = extOf(name);
  if (ext === "md") return <FileText className="h-4 w-4 shrink-0 text-blue-500" />;
  if (CODE_EXTENSIONS.has(ext)) return <FileCode2 className="h-4 w-4 shrink-0 text-orange-500" />;
  return <File className="h-4 w-4 shrink-0 text-muted-foreground" />;
}

// --- Components ---

function TreeItem({
  node,
  depth,
  activePath,
  onSelect,
}: {
  node: TreeNode;
  depth: number;
  activePath: string | null;
  onSelect: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(depth === 0);
  const isActive = activePath === node.path;

  if (node.isDir) {
    return (
      <div>
        <button
          type="button"
          className="flex w-full items-center gap-1 rounded px-2 py-1 text-left text-sm hover:bg-accent cursor-pointer"
          style={{ paddingLeft: `${depth * 16 + 8}px` }}
          onClick={() => setExpanded(!expanded)}
        >
          <ChevronRight
            className={`h-3 w-3 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`}
          />
          {expanded ? (
            <FolderOpen className="h-4 w-4 shrink-0 text-yellow-600" />
          ) : (
            <Folder className="h-4 w-4 shrink-0 text-yellow-600" />
          )}
          <span className="truncate">{node.name}</span>
        </button>
        {expanded && node.children.map((child) => (
          <TreeItem
            key={child.path}
            node={child}
            depth={depth + 1}
            activePath={activePath}
            onSelect={onSelect}
          />
        ))}
      </div>
    );
  }

  return (
    <button
      type="button"
      className={`flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm cursor-pointer ${
        isActive ? "bg-accent text-accent-foreground" : "hover:bg-accent/50"
      }`}
      style={{ paddingLeft: `${depth * 16 + 20}px` }}
      onClick={() => onSelect(node.path)}
    >
      <FileIcon name={node.name} />
      <span className="truncate">{node.name}</span>
    </button>
  );
}

function CodeViewer({ content, language }: { content: string; language: string }) {
  const { copied, copy } = useClipboard();

  return (
    <div className="group relative overflow-hidden rounded-md border">
      <div className="flex items-center justify-between bg-muted px-3 py-1 text-xs text-muted-foreground">
        <span>{language || "text"}</span>
        <button
          type="button"
          onClick={() => copy(content)}
          className="cursor-pointer opacity-0 transition-opacity group-hover:opacity-100"
          title="Copy"
        >
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
        </button>
      </div>
      <pre className="overflow-auto bg-muted/50 p-4 text-sm text-foreground max-h-[60vh]">
        <code className={language ? `language-${language}` : ""}>{content}</code>
      </pre>
    </div>
  );
}

function FileContentViewer({ path, content, size }: { path: string; content: string; size: number }) {
  const ext = extOf(path);
  const displayContent = ext === "md" ? stripFrontmatter(content) : content;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between text-xs text-muted-foreground border-b pb-2">
        <span className="font-mono">{path}</span>
        <span>{formatSize(size)}</span>
      </div>
      {ext === "md" ? (
        <MarkdownRenderer content={displayContent} />
      ) : CODE_EXTENSIONS.has(ext) ? (
        <CodeViewer content={displayContent} language={langFor(ext)} />
      ) : (
        <pre className="whitespace-pre-wrap rounded-md border bg-muted/30 p-4 text-sm">
          {displayContent}
        </pre>
      )}
    </div>
  );
}

// --- Main Component ---

export function SkillDetailDialog({
  skill,
  onClose,
  getSkillVersions,
  getSkillFiles,
  getSkillFileContent,
}: SkillDetailDialogProps) {
  const hasFiles = !!skill.id;

  // Version state
  const [versions, setVersions] = useState<SkillVersions | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);

  // File tree state
  const [files, setFiles] = useState<SkillFile[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const [activePath, setActivePath] = useState<string | null>(null);

  // File content state
  const [fileContent, setFileContent] = useState<{ content: string; path: string; size: number } | null>(null);
  const [contentLoading, setContentLoading] = useState(false);

  const tree = useMemo(() => buildTree(files), [files]);

  // Load versions when Files tab is first opened
  const loadVersions = useCallback(async () => {
    if (!skill.id || versions) return;
    const v = await getSkillVersions(skill.id);
    setVersions(v);
    setSelectedVersion(v.current);
  }, [skill.id, versions, getSkillVersions]);

  // Load files for selected version
  const loadFiles = useCallback(async (version?: number) => {
    if (!skill.id) return;
    setFilesLoading(true);
    try {
      const f = await getSkillFiles(skill.id, version);
      setFiles(f);
      setActivePath(null);
      setFileContent(null);
    } finally {
      setFilesLoading(false);
    }
  }, [skill.id, getSkillFiles]);

  // Load file content
  const loadFileContent = useCallback(async (path: string) => {
    if (!skill.id) return;
    setActivePath(path);
    setContentLoading(true);
    try {
      const c = await getSkillFileContent(skill.id, path, selectedVersion ?? undefined);
      setFileContent(c);
    } finally {
      setContentLoading(false);
    }
  }, [skill.id, selectedVersion, getSkillFileContent]);

  // On version change
  useEffect(() => {
    if (selectedVersion != null) {
      loadFiles(selectedVersion);
    }
  }, [selectedVersion, loadFiles]);

  const handleTabChange = (tab: string) => {
    if (tab === "files" && hasFiles) {
      loadVersions();
      if (files.length === 0 && !filesLoading) {
        loadFiles(selectedVersion ?? undefined);
      }
    }
  };

  return (
    <Dialog open onOpenChange={() => onClose()}>
      <DialogContent className="max-h-[85vh] overflow-hidden flex flex-col sm:max-w-6xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 flex-wrap">
            {skill.name}
            <Badge variant="outline">{skill.source || "file"}</Badge>
            {skill.visibility && (
              <Badge variant="secondary">{skill.visibility}</Badge>
            )}
            {skill.version ? (
              <span className="text-xs font-normal text-muted-foreground">v{skill.version}</span>
            ) : null}
          </DialogTitle>
          {skill.description && (
            <p className="text-sm text-muted-foreground">{skill.description}</p>
          )}
          {skill.tags && skill.tags.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-1">
              {skill.tags.map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs">{tag}</Badge>
              ))}
            </div>
          )}
        </DialogHeader>

        <Tabs defaultValue="content" className="flex-1 overflow-hidden flex flex-col" onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="content">Content</TabsTrigger>
            {hasFiles && <TabsTrigger value="files">Files</TabsTrigger>}
          </TabsList>

          <TabsContent value="content" className="flex-1 overflow-y-auto mt-2">
            {skill.content ? (
              <div className="overflow-hidden rounded-md border bg-muted/30 p-4">
                <MarkdownRenderer content={skill.content} />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">No content available.</p>
            )}
          </TabsContent>

          {hasFiles && (
            <TabsContent value="files" className="flex-1 overflow-hidden flex flex-col mt-2 gap-2">
              {/* Version selector */}
              {versions && versions.versions.length > 1 && (
                <div className="flex items-center gap-2">
                  <span className="text-sm text-muted-foreground">Version:</span>
                  <Select
                    value={String(selectedVersion ?? versions.current)}
                    onValueChange={(v) => setSelectedVersion(Number(v))}
                  >
                    <SelectTrigger className="w-40 h-8">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {versions.versions.map((v) => (
                        <SelectItem key={v} value={String(v)}>
                          v{v}{v === versions.current ? " (current)" : ""}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              )}

              {/* File browser */}
              <div className="flex-1 flex border rounded-md overflow-hidden min-h-0">
                {/* Tree panel */}
                <div className="w-[35%] border-r overflow-y-auto bg-muted/20 py-1">
                  {filesLoading ? (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                    </div>
                  ) : tree.length === 0 ? (
                    <p className="px-3 py-4 text-sm text-muted-foreground">No files found.</p>
                  ) : (
                    tree.map((node) => (
                      <TreeItem
                        key={node.path}
                        node={node}
                        depth={0}
                        activePath={activePath}
                        onSelect={loadFileContent}
                      />
                    ))
                  )}
                </div>

                {/* Content panel */}
                <div className="w-[65%] overflow-y-auto p-3">
                  {contentLoading ? (
                    <div className="flex items-center justify-center py-8">
                      <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
                    </div>
                  ) : fileContent ? (
                    <FileContentViewer
                      path={fileContent.path}
                      content={fileContent.content}
                      size={fileContent.size}
                    />
                  ) : (
                    <div className="flex items-center justify-center py-8 text-sm text-muted-foreground">
                      Select a file to view its content
                    </div>
                  )}
                </div>
              </div>
            </TabsContent>
          )}
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
