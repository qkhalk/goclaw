/**
 * Durable agent run records + archived run timeline items.
 * Mirrors store.AgentRun / store.RunTimelineItem JSON tags (snake_case).
 */

export type RunStatus =
  | "pending"
  | "running"
  | "compacting"
  | "completed"
  | "failed"
  | "cancelled";

export interface RunRecord {
  id: string;
  tenant_id: string;
  run_id: string;
  session_key: string;
  agent_id?: string | null;
  user_id?: string;
  channel?: string;
  chat_id?: string;
  status: string;
  attempt: number;
  checkpoint?: unknown;
  heartbeat_at: string;
  started_at: string;
  completed_at?: string | null;
  error?: string;
  metadata?: unknown;
  updated_at: string;
  created_at: string;
}

export type RunTimelineItemType =
  | "chunk"
  | "thinking"
  | "tool.started"
  | "tool.call"
  | "tool.result"
  | "run.status"
  | "checkpoint"
  | "activity"
  | "assistant.message"
  | string;

export interface RunTimelineItem {
  id: string;
  tenant_id: string;
  run_id: string;
  session_key: string;
  agent_id?: string | null;
  user_id?: string;
  channel?: string;
  chat_id?: string;
  seq: number;
  item_type: RunTimelineItemType;
  status?: string;
  title?: string;
  preview?: string;
  content?: string;
  tool_name?: string;
  tool_call_id?: string;
  trace_id?: string | null;
  span_id?: string | null;
  metadata?: unknown;
  created_at: string;
}

/** Content JSON emitted with llm.completed activity items (durations/tokens). */
export interface LlmCompletedContent {
  provider?: string;
  model?: string;
  duration_ms?: string | number;
  prompt_tokens?: string | number;
  completion_tokens?: string | number;
  is_error?: boolean;
}

/** Content JSON emitted with checkpoint items ({iteration,status}). */
export interface CheckpointContent {
  iteration?: string | number;
  status?: string;
}

export function parseItemContent<T>(item: RunTimelineItem): T | null {
  if (!item.content) return null;
  try {
    return JSON.parse(item.content) as T;
  } catch {
    return null;
  }
}
