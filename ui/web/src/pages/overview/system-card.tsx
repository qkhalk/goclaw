import { useQuery } from "@tanstack/react-query";
import { Cpu, HardDrive, MemoryStick, Server } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatFileSize } from "@/lib/format";
import { useHttp } from "@/hooks/use-ws";
import type { SystemStats } from "./types";

const REFRESH_INTERVAL = 15_000;

/** Thresholds for the utilization bars (percent). */
const WARN = 80;
const CRIT = 95;

function barClass(pct: number): string {
  if (pct >= CRIT) return "bg-red-500";
  if (pct >= WARN) return "bg-amber-500";
  return "bg-primary";
}

function valueClass(pct: number): string {
  if (pct >= CRIT) return "text-red-600 dark:text-red-400";
  if (pct >= WARN) return "text-amber-600 dark:text-amber-400";
  return "";
}

/** One utilization tile: icon + label + big percent + used/total + bar. */
function UsageTile({
  icon: Icon,
  label,
  pct,
  detail,
}: {
  icon: React.ElementType;
  label: string;
  pct: number | null;
  detail: string;
}) {
  const shown = pct ?? 0;
  return (
    <div className="rounded-lg border bg-muted/30 p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Icon className="h-3.5 w-3.5" />
          {label}
        </p>
        <p className={`text-lg font-semibold tabular-nums leading-none ${valueClass(shown)}`}>
          {pct === null ? "—" : `${shown.toFixed(0)}%`}
        </p>
      </div>
      <div className="mt-2.5 h-1.5 w-full overflow-hidden rounded-full bg-muted">
        <div
          className={`h-full rounded-full ${barClass(shown)}`}
          style={{ width: `${Math.min(shown, 100)}%` }}
        />
      </div>
      <p className="mt-2 truncate text-xs text-muted-foreground tabular-nums" title={detail}>
        {detail}
      </p>
    </div>
  );
}

function MetaChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md bg-muted/50 px-2 py-1 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium tabular-nums">{value}</span>
    </span>
  );
}

/** Host system card at the bottom of the overview tab: CPU / RAM / Disk
 * utilization tiles plus host+process meta chips, polled from
 * GET /v1/system/stats. Hidden entirely when the endpoint is missing (older
 * backend) so the overview never shows a dead card. */
export function SystemCard() {
  const { t } = useTranslation("overview");
  const http = useHttp();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["system", "stats"],
    refetchInterval: REFRESH_INTERVAL,
    retry: false,
    queryFn: () => http.get<SystemStats>("/v1/system/stats"),
  });

  if (isError) return null;

  const cpuPct = data && data.samples >= 2 ? data.cpu.percent : null;
  const loadAvg =
    data?.cpu.load1 !== undefined
      ? `${data.cpu.load1.toFixed(2)} ${data.cpu.load5?.toFixed(2) ?? ""} ${data.cpu.load15?.toFixed(2) ?? ""}`.trim()
      : null;

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Server className="h-4 w-4" /> {t("system.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading || !data ? (
          <div className="grid gap-3 sm:grid-cols-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        ) : (
          <>
            <div className="grid gap-3 sm:grid-cols-3">
              <UsageTile
                icon={Cpu}
                label={`${t("system.cpu")} · ${t("system.cores", { count: data.cpu.cores })}`}
                pct={cpuPct}
                detail={loadAvg ? `${t("system.loadAvg")} ${loadAvg}` : t("system.cpu")}
              />
              <UsageTile
                icon={MemoryStick}
                label={t("system.memory")}
                pct={data.memory.used_percent}
                detail={`${formatFileSize(data.memory.used)} / ${formatFileSize(data.memory.total)}`}
              />
              {data.disk ? (
                <UsageTile
                  icon={HardDrive}
                  label={t("system.disk")}
                  pct={data.disk.used_percent}
                  detail={`${formatFileSize(data.disk.used)} / ${formatFileSize(data.disk.total)}`}
                />
              ) : (
                <div className="flex items-center justify-center rounded-lg border border-dashed p-3 text-xs text-muted-foreground">
                  {t("system.disk")} —
                </div>
              )}
            </div>
            <div className="flex flex-wrap gap-1.5">
              <MetaChip label={t("system.hostUptime")} value={formatUptimeShort(data.host.host_uptime_secs)} />
              {data.host.platform && (
                <MetaChip label={t("system.platform")} value={data.host.platform} />
              )}
              {data.memory.swap_total !== undefined && data.memory.swap_total > 0 && (
                <MetaChip
                  label={t("system.swap")}
                  value={`${formatFileSize(data.memory.swap_used ?? 0)} / ${formatFileSize(data.memory.swap_total)}`}
                />
              )}
              <MetaChip label={t("system.goroutines")} value={String(data.proc.goroutines)} />
              <MetaChip label={t("system.heap")} value={formatFileSize(data.proc.heap_alloc_bytes)} />
              {data.proc.rss_bytes !== undefined && (
                <MetaChip label="RSS" value={formatFileSize(data.proc.rss_bytes)} />
              )}
              <MetaChip label={t("system.goVersion")} value={data.proc.go_version.replace(/^go/, "")} />
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function formatUptimeShort(secs: number): string {
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
