import { useState, useEffect, useCallback } from "react";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useHttp } from "@/hooks/use-ws";
import { toast } from "@/stores/use-toast-store";
import type { MemoryDocument, MemoryDocumentDetail, MemoryChunk } from "@/types/memory";

interface MemoryDocumentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agentId: string;
  document: MemoryDocument | null;
}

export function MemoryDocumentDialog({ open, onOpenChange, agentId, document }: MemoryDocumentDialogProps) {
  const http = useHttp();
  const [tab, setTab] = useState<"content" | "chunks">("content");
  const [detail, setDetail] = useState<MemoryDocumentDetail | null>(null);
  const [chunks, setChunks] = useState<MemoryChunk[]>([]);
  const [content, setContent] = useState("");
  const [saving, setSaving] = useState(false);
  const [loadingDetail, setLoadingDetail] = useState(false);
  const [loadingChunks, setLoadingChunks] = useState(false);

  const loadDetail = useCallback(async () => {
    if (!document || !agentId) return;
    setLoadingDetail(true);
    try {
      const params: Record<string, string> = {};
      if (document.user_id) params.user_id = document.user_id;
      const res = await http.get<MemoryDocumentDetail>(
        `/v1/agents/${agentId}/memory/documents/${document.path}`,
        params,
      );
      setDetail(res);
      setContent(res.content);
    } catch {
      toast.error("Failed to load document");
    } finally {
      setLoadingDetail(false);
    }
  }, [http, agentId, document]);

  const loadChunks = useCallback(async () => {
    if (!document || !agentId) return;
    setLoadingChunks(true);
    try {
      const params: Record<string, string> = { path: document.path };
      if (document.user_id) params.user_id = document.user_id;
      const res = await http.get<MemoryChunk[]>(
        `/v1/agents/${agentId}/memory/chunks`,
        params,
      );
      setChunks(res ?? []);
    } catch {
      setChunks([]);
    } finally {
      setLoadingChunks(false);
    }
  }, [http, agentId, document]);

  useEffect(() => {
    if (open && document) {
      setTab("content");
      setDetail(null);
      setChunks([]);
      loadDetail();
    }
  }, [open, document, loadDetail]);

  useEffect(() => {
    if (tab === "chunks" && chunks.length === 0 && !loadingChunks && document) {
      loadChunks();
    }
  }, [tab, chunks.length, loadingChunks, document, loadChunks]);

  const handleSave = async () => {
    if (!document || !agentId) return;
    setSaving(true);
    try {
      await http.put(`/v1/agents/${agentId}/memory/documents/${document.path}`, {
        content,
        user_id: document.user_id || "",
      });
      toast.success("Document updated", document.path);
      onOpenChange(false);
    } catch (err) {
      toast.error("Failed to save", err instanceof Error ? err.message : "Unknown error");
    } finally {
      setSaving(false);
    }
  };

  const hasChanges = detail != null && content !== detail.content;

  return (
    <Dialog open={open} onOpenChange={(v) => !saving && onOpenChange(v)}>
      <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span className="font-mono text-sm">{document?.path}</span>
            {document?.user_id ? (
              <Badge variant="secondary">Personal</Badge>
            ) : (
              <Badge variant="outline">Global</Badge>
            )}
          </DialogTitle>
        </DialogHeader>

        {/* Tab bar */}
        <div className="flex border-b">
          <button
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px ${tab === "content" ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"}`}
            onClick={() => setTab("content")}
          >
            Content
          </button>
          <button
            className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px ${tab === "chunks" ? "border-primary text-primary" : "border-transparent text-muted-foreground hover:text-foreground"}`}
            onClick={() => setTab("chunks")}
          >
            Chunks {detail && `(${detail.chunk_count})`}
          </button>
        </div>

        <div className="flex-1 min-h-0 overflow-y-auto py-2 px-1 -mx-1">
          {tab === "content" && (
            <div className="grid gap-3">
              {detail && (
                <div className="flex gap-4 text-xs text-muted-foreground">
                  <span>Chunks: {detail.chunk_count}</span>
                  <span>Embedded: {detail.embedded_count}/{detail.chunk_count}</span>
                  <span>Created: {new Date(detail.created_at).toLocaleString()}</span>
                </div>
              )}
              <div className="grid gap-1.5">
                <Label>Content</Label>
                {loadingDetail ? (
                  <div className="h-48 flex items-center justify-center text-muted-foreground">Loading...</div>
                ) : (
                  <Textarea
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                    className="font-mono text-xs min-h-[300px]"
                    rows={15}
                  />
                )}
              </div>
            </div>
          )}

          {tab === "chunks" && (
            <div>
              {loadingChunks ? (
                <div className="py-8 text-center text-muted-foreground">Loading chunks...</div>
              ) : chunks.length === 0 ? (
                <div className="py-8 text-center text-muted-foreground">No chunks. Index this document first.</div>
              ) : (
                <div className="space-y-2">
                  {chunks.map((chunk) => (
                    <div key={chunk.id} className="rounded-md border p-3">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="text-xs font-medium">
                          Lines {chunk.start_line}-{chunk.end_line}
                        </span>
                        <Badge variant={chunk.has_embedding ? "default" : "secondary"} className="text-[10px]">
                          {chunk.has_embedding ? "Embedded" : "No embedding"}
                        </Badge>
                      </div>
                      <pre className="text-xs text-muted-foreground whitespace-pre-wrap break-words">
                        {chunk.text_preview}
                      </pre>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Close
          </Button>
          {tab === "content" && (
            <Button onClick={handleSave} disabled={saving || !hasChanges}>
              {saving ? "Saving..." : "Save"}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
