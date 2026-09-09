import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ShieldCheck, Check, X, RefreshCw, History, Timer } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { formatRelativeTime, formatDate } from "@/lib/format";
import {
  useApprovals,
  type PendingApproval,
  type ApprovalScope,
} from "./hooks/use-approvals";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";

const SCOPES: ApprovalScope[] = ["once", "session", "always"];

export function ApprovalsPage() {
  const { t } = useTranslation("approvals");
  const { t: tc } = useTranslation("common");
  const { pending, history, loading, refresh, refreshHistory, approve, deny } = useApprovals();
  const spinning = useMinLoading(loading);
  const showSkeleton = useDeferredLoading(loading && pending.length === 0);
  const [tab, setTab] = useState("pending");
  const [denyTarget, setDenyTarget] = useState<PendingApproval | null>(null);
  const [approveTarget, setApproveTarget] = useState<{ approval: PendingApproval; scope: ApprovalScope } | null>(null);
  const [scopes, setScopes] = useState<Record<string, ApprovalScope>>({});

  const scopeFor = (id: string): ApprovalScope => scopes[id] ?? "once";

  const handleTabChange = (value: string) => {
    setTab(value);
    if (value === "history") refreshHistory();
  };

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <div className="flex items-center gap-2">
            {pending.length > 0 && (
              <Badge variant="destructive">{t("pending", { count: pending.length })}</Badge>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => (tab === "history" ? refreshHistory() : refresh())}
              disabled={spinning}
              className="gap-1"
            >
              <RefreshCw className={"h-3.5 w-3.5" + (spinning ? " animate-spin" : "")} /> {tc("refresh")}
            </Button>
          </div>
        }
      />

      <Tabs value={tab} onValueChange={handleTabChange} className="mt-4">
        <TabsList>
          <TabsTrigger value="pending">{t("tabs.pending")}</TabsTrigger>
          <TabsTrigger value="history">{t("tabs.history")}</TabsTrigger>
        </TabsList>

        <TabsContent value="pending">
          {showSkeleton ? (
            <TableSkeleton rows={3} />
          ) : pending.length === 0 ? (
            <EmptyState
              icon={ShieldCheck}
              title={t("emptyTitle")}
              description={t("emptyDescription")}
            />
          ) : (
            <div className="space-y-3">
              {pending.map((approval: PendingApproval) => (
                <div key={approval.id} className="rounded-lg border p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="outline" className="uppercase">
                          {approval.toolClass || "exec"}
                        </Badge>
                        {approval.toolName && (
                          <Badge variant="secondary">{approval.toolName}</Badge>
                        )}
                        <Badge variant="outline">{approval.agentId}</Badge>
                        <span className="text-xs text-muted-foreground">
                          {formatRelativeTime(new Date(approval.createdAt))}
                        </span>
                        {approval.expiresAt && (
                          <span className="flex items-center gap-1 text-xs text-muted-foreground">
                            <Timer className="h-3 w-3" />
                            {t("expiresIn", { time: formatDate(new Date(approval.expiresAt)) })}
                          </span>
                        )}
                      </div>
                      {approval.risk && (
                        <p className="mt-2 text-sm text-amber-600 dark:text-amber-400">
                          {t("riskLabel")}: {approval.risk}
                        </p>
                      )}
                      <pre className="mt-2 overflow-x-auto rounded-md bg-muted p-3 text-sm">
                        {approval.command}
                      </pre>
                    </div>
                    <div className="flex flex-row flex-wrap items-center gap-2 sm:flex-col sm:items-stretch">
                      <Select
                        value={scopeFor(approval.id)}
                        onValueChange={(v) => setScopes((prev) => ({ ...prev, [approval.id]: v as ApprovalScope }))}
                      >
                        <SelectTrigger className="w-full text-base sm:w-[140px] md:text-sm" aria-label={t("scopeLabel")}>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {SCOPES.map((s) => (
                            <SelectItem key={s} value={s} className="text-base md:text-sm">
                              {t(`scope.${s}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button
                        size="sm"
                        onClick={() => setApproveTarget({ approval, scope: scopeFor(approval.id) })}
                        className="min-h-[44px] flex-1 gap-1 sm:flex-none"
                      >
                        <Check className="h-3.5 w-3.5" /> {t("approve")}
                      </Button>
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setDenyTarget(approval)}
                        className="min-h-[44px] flex-1 gap-1 sm:flex-none"
                      >
                        <X className="h-3.5 w-3.5" /> {t("deny")}
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="history">
          {history.length === 0 ? (
            <EmptyState
              icon={History}
              title={t("historyEmptyTitle")}
              description={t("historyEmptyDescription")}
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[600px] text-sm">
                <thead>
                  <tr className="border-b text-left text-muted-foreground">
                    <th className="px-2 py-2 font-medium">{t("history.class")}</th>
                    <th className="px-2 py-2 font-medium">{t("history.command")}</th>
                    <th className="px-2 py-2 font-medium">{t("history.status")}</th>
                    <th className="px-2 py-2 font-medium">{t("history.decision")}</th>
                    <th className="px-2 py-2 font-medium">{t("history.created")}</th>
                    <th className="px-2 py-2 font-medium">{t("history.grantExpires")}</th>
                  </tr>
                </thead>
                <tbody>
                  {history.map((item) => (
                    <tr key={item.id} className="border-b last:border-0">
                      <td className="px-2 py-2">
                        <Badge variant="outline" className="uppercase">
                          {item.actionType}
                        </Badge>
                      </td>
                      <td className="max-w-[280px] truncate px-2 py-2 font-mono text-xs" title={item.command}>
                        {item.command}
                      </td>
                      <td className="px-2 py-2">
                        <Badge variant={item.status === "approved" ? "default" : item.status === "denied" ? "destructive" : "outline"}>
                          {t(`status.${item.status}`, item.status)}
                        </Badge>
                      </td>
                      <td className="px-2 py-2">{item.decision || "—"}</td>
                      <td className="px-2 py-2 text-muted-foreground">
                        {formatRelativeTime(new Date(item.createdAt))}
                      </td>
                      <td className="px-2 py-2 text-muted-foreground">
                        {item.grantExpiresAt ? formatDate(new Date(item.grantExpiresAt)) : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={!!approveTarget}
        onOpenChange={() => setApproveTarget(null)}
        title={t(`confirmApprove.${approveTarget?.scope ?? "once"}.title`)}
        description={t(`confirmApprove.${approveTarget?.scope ?? "once"}.description`, {
          command: approveTarget?.approval.command,
          agentId: approveTarget?.approval.agentId,
        })}
        confirmLabel={t("approve")}
        onConfirm={async () => {
          if (approveTarget) {
            await approve(approveTarget.approval.id, approveTarget.scope);
            setApproveTarget(null);
          }
        }}
      />

      <ConfirmDialog
        open={!!denyTarget}
        onOpenChange={() => setDenyTarget(null)}
        title={t("confirmDeny.title")}
        description={t("confirmDeny.description", { command: denyTarget?.command, agentId: denyTarget?.agentId })}
        confirmLabel={t("confirmDeny.confirmLabel")}
        variant="destructive"
        onConfirm={async () => {
          if (denyTarget) {
            await deny(denyTarget.id);
            setDenyTarget(null);
          }
        }}
      />
    </div>
  );
}
