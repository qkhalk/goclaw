// Workspace domain + file explorer types (Paseo plan Phase 2/3).
// Wire shapes mirror the gateway's camelCase JSON responses.

export interface WorkspaceInfo {
  id: string;
  tenantId: string | null;
  ownerId: string;
  name: string;
  rootPath: string;
  description: string | null;
  status: "active" | "archived";
  repoUrl: string | null;
  branch: string | null;
  worktreePath: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface FileEntry {
  /** Entry name (basename). */
  name: string;
  /** Root-relative slash path usable in subsequent workspace.files calls. */
  path: string;
  type: "file" | "dir";
  size: number;
  modTime: string;
}

export interface AgentJob {
  id: string;
  tenantId: string | null;
  workspaceId: string | null;
  sessionKey: string;
  agentId: string | null;
  kind: string;
  status:
    | "queued"
    | "starting"
    | "running"
    | "waiting_input"
    | "waiting_approval"
    | "paused"
    | "completed"
    | "failed"
    | "cancelled";
  priority: number;
  title: string;
  resultRef: string | null;
  error: string | null;
  startedAt: string | null;
  completedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface TaskNode {
  id: string;
  tenantId: string | null;
  workspaceId: string | null;
  parentId: string | null;
  ownerAgentId: string | null;
  sessionKey: string | null;
  title: string;
  status: string;
  priority: number;
  dependsOn: string[];
  resultRef: string | null;
  createdAt: string;
  updatedAt: string;
}
