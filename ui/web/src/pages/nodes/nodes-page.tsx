import { useState, useMemo } from "react";
import {
  Link as LinkIcon,
  RefreshCw,
  Check,
  X,
  Trash2,
  Server,
  ShieldCheck,
  Copy,
  Plus,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { PageHeader } from "@/components/shared/page-header";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { formatDate, formatRelativeTime } from "@/lib/format";
import {
  useNodes,
  type PendingPairing,
  type PairedDevice,
} from "./hooks/use-nodes";
import {
  useComputeNodes,
  type ComputeNode,
} from "./hooks/use-compute-nodes";
import { useMinLoading } from "@/hooks/use-min-loading";
import { useDeferredLoading } from "@/hooks/use-deferred-loading";
import { useContactResolver } from "@/hooks/use-contact-resolver";
import { formatUserLabel } from "@/lib/format-user-label";

const trustBadgeVariant = (trust: ComputeNode["trust"]): "default" | "secondary" | "destructive" | "outline" => {
  switch (trust) {
    case "trusted":
      return "default";
    case "revoked":
      return "destructive";
    default:
      return "outline";
  }
};

export function NodesPage() {
  const { t } = useTranslation("nodes");
  const { pendingPairings, pairedDevices, loading, refresh, approvePairing, denyPairing, revokePairing } = useNodes();
  const {
    nodes: computeNodes,
    loading: nodesLoading,
    load: reloadNodes,
    createKey,
    trustNode,
    revokeNode,
  } = useComputeNodes();
  const spinning = useMinLoading(loading || nodesLoading);
  const senderIds = useMemo(() => [
    ...pendingPairings.map((p) => p.sender_id),
    ...pairedDevices.map((d) => d.sender_id),
    ...pairedDevices.map((d) => d.paired_by).filter(Boolean),
  ].filter(Boolean) as string[], [pendingPairings, pairedDevices]);
  const { resolve } = useContactResolver(senderIds);
  const isEmpty = pendingPairings.length === 0 && pairedDevices.length === 0;
  const showSkeleton = useDeferredLoading(loading && isEmpty);
  const [revokeTarget, setRevokeTarget] = useState<PairedDevice | null>(null);
  const [approveTarget, setApproveTarget] = useState<PendingPairing | null>(null);
  const [denyTarget, setDenyTarget] = useState<PendingPairing | null>(null);

  // Compute nodes (Phase 2): key creation, trust approval, node revoke.
  const [newNodeName, setNewNodeName] = useState("");
  const [createdKey, setCreatedKey] = useState<{ name: string; key: string } | null>(null);
  const [creating, setCreating] = useState(false);
  const [trustTarget, setTrustTarget] = useState<ComputeNode | null>(null);
  const [nodeRevokeTarget, setNodeRevokeTarget] = useState<ComputeNode | null>(null);

  const handleCreateKey = async () => {
    const name = newNodeName.trim();
    if (!name || creating) return;
    setCreating(true);
    try {
      const res = await createKey(name);
      setCreatedKey({ name: res.node.name, key: res.key });
      setNewNodeName("");
    } catch {
      // error surfaced via toast upstream; nothing to do here
    } finally {
      setCreating(false);
    }
  };

  return (
    <div className="p-4 sm:p-6 pb-10">
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              refresh();
              reloadNodes();
            }}
            disabled={spinning}
            className="gap-1"
          >
            <RefreshCw className={"h-3.5 w-3.5" + (spinning ? " animate-spin" : "")} /> {t("common:refresh", "Refresh")}
          </Button>
        }
      />

      {/* Compute nodes (node runtime Phase 2) */}
      <div className="mt-4">
        <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          <h3 className="flex items-center gap-2 text-sm font-medium">
            <Server className="h-4 w-4" /> {t("compute.title")}
          </h3>
          <div className="flex w-full gap-2 sm:w-auto">
            <Input
              value={newNodeName}
              onChange={(e) => setNewNodeName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCreateKey();
              }}
              placeholder={t("compute.namePlaceholder")}
              className="h-8 w-full text-base md:text-sm sm:w-56"
            />
            <Button size="sm" onClick={handleCreateKey} disabled={creating || !newNodeName.trim()} className="gap-1 shrink-0">
              <Plus className="h-3.5 w-3.5" /> {t("compute.createKey")}
            </Button>
          </div>
        </div>

        {nodesLoading && computeNodes.length === 0 ? (
          <TableSkeleton rows={2} />
        ) : computeNodes.length === 0 ? (
          <EmptyState icon={Server} title={t("compute.emptyTitle")} description={t("compute.emptyDescription")} />
        ) : (
          <div className="rounded-md border overflow-x-auto">
            <table className="w-full min-w-[600px] text-sm">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left font-medium">{t("compute.columns.name")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("compute.columns.platform")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("compute.columns.capabilities")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("compute.columns.trust")}</th>
                  <th className="px-4 py-3 text-left font-medium">{t("compute.columns.lastSeen")}</th>
                  <th className="px-4 py-3 text-right font-medium">{t("compute.columns.actions")}</th>
                </tr>
              </thead>
              <tbody>
                {computeNodes.map((n: ComputeNode) => (
                  <tr key={n.id} className="border-b last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-2">
                        <span
                          className={"h-2 w-2 rounded-full shrink-0 " + (n.online ? "bg-emerald-500" : "bg-muted-foreground/30")}
                          title={n.online ? t("compute.online") : t("compute.offline")}
                        />
                        <span className="font-medium">{n.name}</span>
                      </span>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs text-muted-foreground">{n.platform || "--"}</td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {(n.capabilities ?? []).map((c) => (
                          <Badge key={c} variant="outline" className="font-mono text-xs">
                            {c}
                          </Badge>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={trustBadgeVariant(n.trust)}>{t(`compute.trust.${n.trust}`)}</Badge>
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {n.lastSeenAt ? formatRelativeTime(new Date(n.lastSeenAt)) : "--"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        {n.trust === "pending" && (
                          <Button variant="ghost" size="sm" onClick={() => setTrustTarget(n)} className="gap-1">
                            <ShieldCheck className="h-3.5 w-3.5" /> {t("compute.trustAction")}
                          </Button>
                        )}
                        {n.trust !== "revoked" && (
                          <Button variant="ghost" size="sm" onClick={() => setNodeRevokeTarget(n)} className="gap-1 text-destructive hover:text-destructive">
                            <Trash2 className="h-3.5 w-3.5" /> {t("compute.revoke")}
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      <div className="mt-8">
        {showSkeleton ? (
          <TableSkeleton rows={4} />
        ) : isEmpty ? (
          <EmptyState
            icon={LinkIcon}
            title={t("emptyTitle")}
            description={t("emptyDescription")}
          />
        ) : (
          <div className="space-y-6">
            {/* Pending pairings */}
            {pendingPairings.length > 0 && (
              <div>
                <h3 className="mb-3 text-sm font-medium">
                  {t("pendingRequests", { count: pendingPairings.length })}
                </h3>
                <div className="space-y-2">
                  {pendingPairings.map((p: PendingPairing) => (
                    <div key={p.code} className="flex items-center justify-between rounded-lg border p-4">
                      <div>
                        <div className="flex items-center gap-2">
                          <Badge variant="outline">{p.channel}</Badge>
                          <span className="font-mono text-sm font-medium">{p.code}</span>
                        </div>
                        <div className="mt-1 text-xs text-muted-foreground">
                          {t("sender")}{formatUserLabel(p.sender_id, resolve)}
                          {p.chat_id && ` | ${t("chat")}${p.chat_id}`}
                          {" | "}
                          {formatRelativeTime(new Date(p.created_at))}
                        </div>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDenyTarget(p)}
                          className="gap-1"
                        >
                          <X className="h-3.5 w-3.5" /> {t("deny")}
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => setApproveTarget(p)}
                          className="gap-1"
                        >
                          <Check className="h-3.5 w-3.5" /> {t("approve")}
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Paired devices */}
            {pairedDevices.length > 0 && (
              <div>
                <h3 className="mb-3 text-sm font-medium">
                  {t("pairedDevices", { count: pairedDevices.length })}
                </h3>
                <div className="rounded-md border overflow-x-auto">
                  <table className="w-full min-w-[600px] text-sm">
                    <thead>
                      <tr className="border-b bg-muted/50">
                        <th className="px-4 py-3 text-left font-medium">{t("columns.channel")}</th>
                        <th className="px-4 py-3 text-left font-medium">{t("columns.senderId")}</th>
                        <th className="px-4 py-3 text-left font-medium">{t("columns.paired")}</th>
                        <th className="px-4 py-3 text-left font-medium">{t("columns.by")}</th>
                        <th className="px-4 py-3 text-right font-medium">{t("columns.actions")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {pairedDevices.map((d: PairedDevice) => (
                        <tr key={`${d.channel}-${d.sender_id}`} className="border-b last:border-0 hover:bg-muted/30">
                          <td className="px-4 py-3">
                            <Badge variant="outline">{d.channel}</Badge>
                          </td>
                          <td className="px-4 py-3 font-medium">{formatUserLabel(d.sender_id, resolve)}</td>
                          <td className="px-4 py-3 text-muted-foreground">
                            {formatDate(new Date(d.paired_at))}
                          </td>
                          <td className="px-4 py-3 text-muted-foreground">{d.paired_by ? formatUserLabel(d.paired_by, resolve) : "--"}</td>
                          <td className="px-4 py-3 text-right">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => setRevokeTarget(d)}
                              className="gap-1"
                            >
                              <Trash2 className="h-3.5 w-3.5" /> {t("revoke")}
                            </Button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {approveTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setApproveTarget(null)}
          title={t("confirmApprove.title")}
          description={t("confirmApprove.description", { channel: approveTarget.channel, senderId: approveTarget.sender_id, code: approveTarget.code })}
          confirmLabel={t("confirmApprove.confirmLabel")}
          onConfirm={async () => {
            await approvePairing(approveTarget.code);
            setApproveTarget(null);
          }}
        />
      )}

      {denyTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setDenyTarget(null)}
          title={t("confirmDeny.title")}
          description={t("confirmDeny.description", { channel: denyTarget.channel, senderId: denyTarget.sender_id, code: denyTarget.code })}
          confirmLabel={t("confirmDeny.confirmLabel")}
          variant="destructive"
          onConfirm={async () => {
            await denyPairing(denyTarget.code);
            setDenyTarget(null);
          }}
        />
      )}

      {revokeTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setRevokeTarget(null)}
          title={t("confirmRevoke.title")}
          description={t("confirmRevoke.description", { channel: revokeTarget.channel, senderId: revokeTarget.sender_id })}
          confirmLabel={t("confirmRevoke.confirmLabel")}
          variant="destructive"
          onConfirm={async () => {
            await revokePairing(revokeTarget.sender_id, revokeTarget.channel);
            setRevokeTarget(null);
          }}
        />
      )}

      {/* Reveal-once node key */}
      <Dialog open={!!createdKey} onOpenChange={(open) => !open && setCreatedKey(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("compute.keyCreatedTitle")}</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">{t("compute.keyCreatedWarning", { name: createdKey?.name })}</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 break-all rounded bg-muted px-3 py-2 font-mono text-xs">{createdKey?.key}</code>
            <Button
              variant="outline"
              size="sm"
              className="gap-1 shrink-0"
              onClick={() => {
                if (createdKey) navigator.clipboard.writeText(createdKey.key).catch(() => {});
              }}
            >
              <Copy className="h-3.5 w-3.5" /> {t("compute.copy")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {trustTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setTrustTarget(null)}
          title={t("compute.confirmTrust.title")}
          description={t("compute.confirmTrust.description", { name: trustTarget.name })}
          confirmLabel={t("compute.confirmTrust.confirmLabel")}
          onConfirm={async () => {
            await trustNode(trustTarget.id, "trusted");
            setTrustTarget(null);
          }}
        />
      )}

      {nodeRevokeTarget && (
        <ConfirmDialog
          open
          onOpenChange={() => setNodeRevokeTarget(null)}
          title={t("compute.confirmRevoke.title")}
          description={t("compute.confirmRevoke.description", { name: nodeRevokeTarget.name })}
          confirmLabel={t("compute.confirmRevoke.confirmLabel")}
          variant="destructive"
          onConfirm={async () => {
            await revokeNode(nodeRevokeTarget.id);
            setNodeRevokeTarget(null);
          }}
        />
      )}
    </div>
  );
}
