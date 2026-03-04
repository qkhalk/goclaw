import { useEffect, useCallback } from "react";
import {
  Activity,
  Bot,
  Hash,
  Radio,
  AlertTriangle,
  ArrowRight,
  Clock,
  Timer,
} from "lucide-react";
import { Link } from "react-router";
import { PageHeader } from "@/components/shared/page-header";
import { StatusBadge } from "@/components/shared/status-badge";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { useAuthStore } from "@/stores/use-auth-store";
import { useWsCall } from "@/hooks/use-ws-call";
import { useWsEvent } from "@/hooks/use-ws-event";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { useTraces } from "@/pages/traces/hooks/use-traces";
import { Methods, Events } from "@/api/protocol";
import { ROUTES } from "@/lib/constants";
import {
  formatRelativeTime,
  formatTokens,
  formatDuration,
} from "@/lib/format";

// --- Types ---

interface HealthPayload {
  status?: string;
  uptime?: number;
}

interface AgentInfo {
  id: string;
  model: string;
  isRunning: boolean;
}

interface StatusPayload {
  agents?: AgentInfo[];
  sessions?: number;
  clients?: number;
}

interface ChannelStatusEntry {
  enabled: boolean;
  running: boolean;
}

interface ChannelStatusPayload {
  channels: Record<string, ChannelStatusEntry>;
}

interface QuotaUsage {
  used: number;
  limit: number;
}

interface QuotaUsageEntry {
  userId: string;
  hour: QuotaUsage;
  day: QuotaUsage;
  week: QuotaUsage;
}

interface QuotaUsageResult {
  enabled: boolean;
  requestsToday: number;
  inputTokensToday: number;
  outputTokensToday: number;
  uniqueUsersToday: number;
  entries: QuotaUsageEntry[];
}

interface CronJob {
  id: string;
  name: string;
  enabled: boolean;
  state: {
    nextRunAtMs?: number;
    lastStatus?: string;
  };
}

interface CronListPayload {
  jobs: CronJob[];
}

// --- Helpers ---

function StatCard({
  icon: Icon,
  label,
  value,
  sub,
}: {
  icon: React.ElementType;
  label: string;
  value: string | number;
  sub?: string;
}) {
  return (
    <div className="rounded-lg border bg-card p-5">
      <div className="flex items-center gap-3">
        <div className="rounded-md bg-muted p-2">
          <Icon className="h-4 w-4 text-muted-foreground" />
        </div>
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold">{value}</p>
          {sub && <p className="text-xs text-muted-foreground">{sub}</p>}
        </div>
      </div>
    </div>
  );
}

function QuotaBar({ used, limit }: { used: number; limit: number }) {
  if (limit === 0) {
    return <span className="text-xs text-muted-foreground">no limit</span>;
  }
  const pct = Math.min((used / limit) * 100, 100);
  const color =
    pct > 85
      ? "bg-red-500"
      : pct > 60
        ? "bg-amber-500"
        : "bg-emerald-500";
  return (
    <div className="h-1.5 w-full rounded-full bg-muted">
      <div
        className={`h-full rounded-full transition-all ${color}`}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

function QuotaCell({ usage }: { usage: QuotaUsage }) {
  const label =
    usage.limit === 0
      ? String(usage.used)
      : `${usage.used}/${usage.limit}`;
  return (
    <div className="space-y-1">
      <span className="text-sm tabular-nums">{label}</span>
      <QuotaBar used={usage.used} limit={usage.limit} />
    </div>
  );
}

function formatUptime(ms: number | undefined): string {
  if (!ms) return "—";
  const sec = Math.floor(ms / 1000);
  const min = Math.floor(sec / 60) % 60;
  const hr = Math.floor(sec / 3600) % 24;
  const day = Math.floor(sec / 86400);
  if (day > 0) return `${day}d ${hr}h ${min}m`;
  if (hr > 0) return `${hr}h ${min}m`;
  return `${min}m`;
}

// --- Page ---

export function OverviewPage() {
  const connected = useAuthStore((s) => s.connected);
  const { call: fetchHealth, data: health } = useWsCall<HealthPayload>(Methods.HEALTH);
  const { call: fetchStatus, data: status } = useWsCall<StatusPayload>(Methods.STATUS);
  const { call: fetchQuota, data: quota } = useWsCall<QuotaUsageResult>(Methods.QUOTA_USAGE);
  const { call: fetchCron, data: cronData } = useWsCall<CronListPayload>(Methods.CRON_LIST);
  const { call: fetchChannels, data: channelStatusData } = useWsCall<ChannelStatusPayload>(Methods.CHANNELS_STATUS);
  const { providers, loading: providersLoading } = useProviders();
  const { traces } = useTraces({ limit: 8 });

  const hasNoProviders = !providersLoading && providers.length === 0;
  const hasNoEnabledProviders =
    !providersLoading && providers.length > 0 && !providers.some((p) => p.enabled);

  useEffect(() => {
    if (connected) {
      fetchHealth();
      fetchStatus();
      fetchQuota();
      fetchCron({ includeDisabled: true });
      fetchChannels();
    }
  }, [connected, fetchHealth, fetchStatus, fetchQuota, fetchCron, fetchChannels]);

  const handleHealth = useCallback(() => {
    fetchHealth();
    fetchStatus();
  }, [fetchHealth, fetchStatus]);

  useWsEvent(Events.HEALTH, handleHealth);

  // Compute stats
  const agents = status?.agents ?? [];
  const runningAgents = agents.filter((a) => a.isRunning).length;
  const channelEntries = channelStatusData?.channels
    ? Object.values(channelStatusData.channels)
    : [];
  const channelsOnline = channelEntries.filter((c) => c.running).length;
  const cronJobs = cronData?.jobs ?? [];
  const enabledProviders = providers.filter((p) => p.enabled).length;

  return (
    <div className="space-y-6 p-6">
      <PageHeader
        title="Dashboard"
        description="Gateway overview and quota usage"
        actions={
          <StatusBadge
            status={connected ? "success" : "error"}
            label={connected ? "Connected" : "Disconnected"}
          />
        }
      />

      {/* Provider warning */}
      {(hasNoProviders || hasNoEnabledProviders) && (
        <Alert>
          <AlertTriangle className="h-4 w-4" />
          <AlertTitle>
            {hasNoProviders
              ? "No LLM providers configured"
              : "No LLM providers enabled"}
          </AlertTitle>
          <AlertDescription>
            {hasNoProviders
              ? "You need to add at least one LLM provider before agents can work. "
              : "All providers are currently disabled. Enable at least one to start using agents. "}
            <Link
              to={ROUTES.PROVIDERS}
              className="font-medium underline underline-offset-4 hover:text-foreground"
            >
              Go to Provider Settings
            </Link>
          </AlertDescription>
        </Alert>
      )}

      {/* Summary cards */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          icon={Activity}
          label="Requests Today"
          value={quota?.requestsToday ?? 0}
          sub={quota?.uniqueUsersToday ? `${quota.uniqueUsersToday} users` : undefined}
        />
        <StatCard
          icon={Hash}
          label="Tokens Today"
          value={formatTokens(
            (quota?.inputTokensToday ?? 0) + (quota?.outputTokensToday ?? 0),
          )}
          sub={
            quota
              ? `${formatTokens(quota.inputTokensToday)} in / ${formatTokens(quota.outputTokensToday)} out`
              : undefined
          }
        />
        <StatCard
          icon={Bot}
          label="Agents"
          value={agents.length > 0 ? `${runningAgents} / ${agents.length}` : "0"}
          sub={agents.length > 0 ? "running" : undefined}
        />
        <StatCard
          icon={Radio}
          label="Channels"
          value={
            channelEntries.length > 0
              ? `${channelsOnline} / ${channelEntries.length}`
              : "0"
          }
          sub={channelEntries.length > 0 ? "online" : undefined}
        />
      </div>

      {/* Quota Usage */}
      {quota?.enabled && quota.entries.length > 0 && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-3">
            <CardTitle className="text-base">Quota Usage</CardTitle>
            <StatusBadge status="success" label="Enabled" />
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 pr-4 font-medium">User / Group</th>
                    <th className="pb-2 px-4 font-medium w-36">Hour</th>
                    <th className="pb-2 px-4 font-medium w-36">Day</th>
                    <th className="pb-2 pl-4 font-medium w-36">Week</th>
                  </tr>
                </thead>
                <tbody>
                  {quota.entries.map((entry) => (
                    <tr key={entry.userId} className="border-b last:border-0">
                      <td className="py-3 pr-4">
                        <span className="font-mono text-xs">{entry.userId}</span>
                      </td>
                      <td className="py-3 px-4">
                        <QuotaCell usage={entry.hour} />
                      </td>
                      <td className="py-3 px-4">
                        <QuotaCell usage={entry.day} />
                      </td>
                      <td className="py-3 pl-4">
                        <QuotaCell usage={entry.week} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Recent Requests */}
      {traces.length > 0 && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-3">
            <CardTitle className="text-base">Recent Requests</CardTitle>
            <Link
              to={ROUTES.TRACES}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              View All <ArrowRight className="h-3 w-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="pb-2 pr-4 font-medium">Time</th>
                    <th className="pb-2 px-4 font-medium">Name</th>
                    <th className="pb-2 px-4 font-medium">User</th>
                    <th className="pb-2 px-4 font-medium">Channel</th>
                    <th className="pb-2 px-4 font-medium text-right">Tokens</th>
                    <th className="pb-2 px-4 font-medium text-right">Duration</th>
                    <th className="pb-2 pl-4 font-medium">Status</th>
                  </tr>
                </thead>
                <tbody>
                  {traces.map((t) => (
                    <tr key={t.id} className="border-b last:border-0">
                      <td className="py-2.5 pr-4 text-muted-foreground whitespace-nowrap">
                        {formatRelativeTime(t.created_at)}
                      </td>
                      <td className="py-2.5 px-4 max-w-[160px] truncate">
                        {t.name || "—"}
                      </td>
                      <td className="py-2.5 px-4 font-mono text-xs">
                        {t.user_id || "—"}
                      </td>
                      <td className="py-2.5 px-4">{t.channel || "—"}</td>
                      <td className="py-2.5 px-4 text-right tabular-nums">
                        {formatTokens(t.total_input_tokens + t.total_output_tokens)}
                      </td>
                      <td className="py-2.5 px-4 text-right tabular-nums">
                        {formatDuration(t.duration_ms)}
                      </td>
                      <td className="py-2.5 pl-4">
                        <StatusBadge
                          status={
                            t.status === "completed"
                              ? "success"
                              : t.status === "error"
                                ? "error"
                                : t.status === "running"
                                  ? "info"
                                  : "default"
                          }
                          label={t.status}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Bottom row: System + Cron */}
      <div className="grid gap-4 lg:grid-cols-2">
        {/* System */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">System</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="space-y-3 text-sm">
              <div className="flex justify-between">
                <dt className="flex items-center gap-2 text-muted-foreground">
                  <Clock className="h-3.5 w-3.5" /> Uptime
                </dt>
                <dd className="font-medium">{formatUptime(health?.uptime)}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Sessions</dt>
                <dd className="font-medium">{status?.sessions ?? 0}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Connected Clients</dt>
                <dd className="font-medium">{status?.clients ?? 0}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-muted-foreground">Providers</dt>
                <dd className="font-medium">
                  {enabledProviders > 0
                    ? `${enabledProviders} active`
                    : "none"}
                </dd>
              </div>
            </dl>
          </CardContent>
        </Card>

        {/* Cron Jobs */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-3">
            <CardTitle className="text-base">Cron Jobs</CardTitle>
            {cronJobs.length > 0 && (
              <Link
                to={ROUTES.CRON}
                className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
              >
                Manage <ArrowRight className="h-3 w-3" />
              </Link>
            )}
          </CardHeader>
          <CardContent>
            {cronJobs.length === 0 ? (
              <p className="text-sm text-muted-foreground">No cron jobs configured</p>
            ) : (
              <div className="space-y-2.5">
                {cronJobs.slice(0, 5).map((job) => (
                  <div
                    key={job.id}
                    className="flex items-center justify-between text-sm"
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className={`h-1.5 w-1.5 rounded-full ${
                          job.enabled ? "bg-emerald-500" : "bg-muted-foreground/40"
                        }`}
                      />
                      <span className={job.enabled ? "" : "text-muted-foreground"}>
                        {job.name}
                      </span>
                    </div>
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      {job.enabled && job.state.nextRunAtMs ? (
                        <>
                          <Timer className="h-3 w-3" />
                          {formatRelativeTime(new Date(job.state.nextRunAtMs)).replace(" ago", "")}
                        </>
                      ) : !job.enabled ? (
                        "disabled"
                      ) : (
                        "—"
                      )}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
