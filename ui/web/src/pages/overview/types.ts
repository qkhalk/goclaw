import type { ChannelRuntimeStatus } from "@/types/channel";

export interface ClientInfo {
  id: string;
  remoteAddr: string;
  userId: string;
  role: string;
  connectedAt: string;
}

export interface HealthPayload {
  status?: string;
  version?: string;
  uptime?: number;
  mode?: string;
  database?: string;
  tools?: number;
  clients?: ClientInfo[];
  currentId?: string;
  latestVersion?: string;
  updateAvailable?: boolean;
  updateUrl?: string;
  releaseNotes?: string;
}

export interface AgentInfo {
  id: string;
  model: string;
  isRunning: boolean;
}

export interface StatusPayload {
  agents?: AgentInfo[];
  agentTotal?: number;
  sessions?: number;
  clients?: number;
}

export type ChannelStatusEntry = ChannelRuntimeStatus;

export interface ChannelStatusPayload {
  channels: Record<string, ChannelStatusEntry>;
}

export interface QuotaUsage {
  used: number;
  limit: number;
}

export interface QuotaUsageEntry {
  userId: string;
  hour: QuotaUsage;
  day: QuotaUsage;
  week: QuotaUsage;
}

export interface QuotaUsageResult {
  enabled: boolean;
  requestsToday: number;
  inputTokensToday: number;
  outputTokensToday: number;
  costToday: number;
  uniqueUsersToday: number;
  entries: QuotaUsageEntry[];
}

export interface CronJob {
  id: string;
  name: string;
  enabled: boolean;
  state: {
    nextRunAtMs?: number;
    lastStatus?: string;
  };
}

export interface CronListPayload {
  jobs: CronJob[];
}

/** One provider→model pair from GET /v1/usage/routing. */
export interface RoutingEdge {
  provider: string;
  model: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  errors: number;
}

/** One recent LLM API call, matching GET /v1/usage/recent-requests. */
export interface RecentLLMRequest {
  span_id: string;
  trace_id: string;
  model: string;
  provider: string;
  input_tokens: number;
  output_tokens: number;
  status: string;
  error?: string;
  start_time: string;
  duration_ms: number;
  cost_usd?: number;
}

/** GET /v1/system/stats response (fields with pointers may be absent). */
export interface SystemStats {
  cpu: {
    percent: number;
    cores: number;
    load1?: number;
    load5?: number;
    load15?: number;
  };
  memory: {
    total: number;
    used: number;
    available: number;
    used_percent: number;
    swap_total?: number;
    swap_used?: number;
  };
  disk?: {
    path: string;
    total: number;
    used: number;
    used_percent: number;
  };
  host: {
    os: string;
    platform?: string;
    host_uptime_secs: number;
  };
  proc: {
    pid: number;
    goroutines: number;
    heap_alloc_bytes: number;
    rss_bytes?: number;
    go_version: string;
  };
  samples: number;
}
