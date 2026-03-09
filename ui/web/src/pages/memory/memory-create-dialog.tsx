import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useAgents } from "@/pages/agents/hooks/use-agents";
import { useMemoryDocuments } from "./hooks/use-memory";
import type { AgentData } from "@/types/agent";

interface MemoryCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Pre-selected agent from parent filter (optional) */
  agentId?: string;
  agentName?: string;
  /** Known user/group IDs from existing docs */
  knownUserIds?: string[];
}

export function MemoryCreateDialog({ open, onOpenChange, agentId: parentAgentId, knownUserIds = [] }: MemoryCreateDialogProps) {
  const { agents } = useAgents();
  const [selectedAgentId, setSelectedAgentId] = useState("");
  const [path, setPath] = useState("");
  const [content, setContent] = useState("");
  const [scopeMode, setScopeMode] = useState<"global" | "existing" | "custom">("global");
  const [selectedUserId, setSelectedUserId] = useState("");
  const [customUserId, setCustomUserId] = useState("");
  const [autoIndex, setAutoIndex] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const effectiveAgentId = selectedAgentId || parentAgentId || "";
  const { createDocument, indexDocument } = useMemoryDocuments({ agentId: effectiveAgentId || undefined });

  const selectedAgent: AgentData | undefined = agents.find((a) => a.id === effectiveAgentId);

  useEffect(() => {
    if (open) {
      setSelectedAgentId(parentAgentId || "");
      setPath("");
      setContent("");
      setScopeMode("global");
      setSelectedUserId("");
      setCustomUserId("");
      setAutoIndex(true);
      setError("");
    }
  }, [open, parentAgentId]);

  const resolvedUserId = (): string | undefined => {
    if (scopeMode === "global") return undefined;
    if (scopeMode === "existing") return selectedUserId || undefined;
    return customUserId.trim() || undefined;
  };

  const handleSubmit = async () => {
    if (!effectiveAgentId) {
      setError("Please select an agent");
      return;
    }
    if (!path.trim()) {
      setError("Path is required");
      return;
    }
    if (!content.trim()) {
      setError("Content is required");
      return;
    }

    setLoading(true);
    setError("");
    try {
      const uid = resolvedUserId();
      await createDocument(path.trim(), content, uid);
      if (autoIndex) {
        await indexDocument(path.trim(), uid);
      }
      onOpenChange(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create");
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !loading && onOpenChange(v)}>
      <DialogContent className="max-w-3xl max-h-[85vh] flex flex-col">
        <DialogHeader>
          <DialogTitle>Create Memory Document</DialogTitle>
        </DialogHeader>

        <div className="grid gap-4 py-2 px-1 -mx-1 overflow-y-auto min-h-0">
          {/* Agent selector */}
          <div className="grid gap-1.5">
            <Label htmlFor="mc-agent">Agent *</Label>
            <select
              id="mc-agent"
              value={selectedAgentId || parentAgentId || ""}
              onChange={(e) => setSelectedAgentId(e.target.value)}
              className="h-9 rounded-md border bg-background px-3 text-sm"
            >
              <option value="">Select agent...</option>
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.display_name || a.agent_key}
                </option>
              ))}
            </select>
            {selectedAgent?.workspace && (
              <p className="font-mono text-[10px] text-muted-foreground">{selectedAgent.workspace}</p>
            )}
          </div>

          {/* Scope selector */}
          <div className="grid gap-1.5">
            <Label>Scope</Label>
            <div className="flex gap-2">
              <Button
                type="button"
                variant={scopeMode === "global" ? "default" : "outline"}
                size="sm"
                onClick={() => setScopeMode("global")}
              >
                Global
              </Button>
              {knownUserIds.length > 0 && (
                <Button
                  type="button"
                  variant={scopeMode === "existing" ? "default" : "outline"}
                  size="sm"
                  onClick={() => setScopeMode("existing")}
                >
                  Existing scope
                </Button>
              )}
              <Button
                type="button"
                variant={scopeMode === "custom" ? "default" : "outline"}
                size="sm"
                onClick={() => setScopeMode("custom")}
              >
                Custom
              </Button>
            </div>
            {scopeMode === "existing" && knownUserIds.length > 0 && (
              <select
                value={selectedUserId}
                onChange={(e) => setSelectedUserId(e.target.value)}
                className="h-9 rounded-md border bg-background px-3 text-sm"
              >
                <option value="">Select group/user...</option>
                {knownUserIds.map((uid) => (
                  <option key={uid} value={uid}>
                    {formatScopeLabel(uid)}
                  </option>
                ))}
              </select>
            )}
            {scopeMode === "custom" && (
              <Input
                value={customUserId}
                onChange={(e) => setCustomUserId(e.target.value)}
                placeholder="e.g. group:telegram:-100123456"
                className="font-mono text-sm"
              />
            )}
            <p className="text-xs text-muted-foreground">
              Global = shared across all users. Personal = scoped to a specific user or group chat.
            </p>
          </div>

          {/* Path */}
          <div className="grid gap-1.5">
            <Label htmlFor="mc-path">Path *</Label>
            <Input
              id="mc-path"
              value={path}
              onChange={(e) => setPath(e.target.value)}
              placeholder="e.g. notes/project-overview.md"
              className="font-mono text-sm"
            />
          </div>

          {/* Content */}
          <div className="grid gap-1.5">
            <Label htmlFor="mc-content">Content *</Label>
            <Textarea
              id="mc-content"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="Document content..."
              className="font-mono text-xs min-h-[200px]"
              rows={10}
            />
          </div>

          <div className="flex items-center gap-2">
            <Switch id="mc-index" checked={autoIndex} onCheckedChange={setAutoIndex} />
            <Label htmlFor="mc-index">Auto-index after creation</Label>
          </div>

          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={loading}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading ? "Creating..." : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function formatScopeLabel(userId: string): string {
  if (userId.startsWith("group:")) {
    const parts = userId.split(":");
    if (parts.length >= 3) {
      const channel = parts[1]!.charAt(0).toUpperCase() + parts[1]!.slice(1);
      return `${channel} ${parts.slice(2).join(":")}`;
    }
  }
  return userId;
}
