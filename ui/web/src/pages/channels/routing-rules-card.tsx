import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowDownNarrowWide, Pencil, Plus, Trash2, Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ConfirmDeleteDialog } from "@/components/shared/confirm-delete-dialog";
import { EmptyState } from "@/components/shared/empty-state";
import { TableSkeleton } from "@/components/shared/loading-skeleton";
import { toast } from "@/stores/use-toast-store";
import { userFriendlyError } from "@/lib/error-utils";
import type { AgentData } from "@/types/agent";
import {
  useRoutingRules,
  type RoutingRule,
  type RoutingRuleInput,
  type RoutingRuleMatch,
} from "./hooks/use-routing-rules";

interface RoutingRulesCardProps {
  agents: AgentData[];
}

interface RuleFormState {
  id?: string;
  channel: string;
  accountId: string;
  peerKind: string; // "", "direct", "group"
  peerId: string;
  guildId: string;
  priority: number;
  targetAgentId: string;
  enabled: boolean;
}

const emptyForm: RuleFormState = {
  channel: "",
  accountId: "",
  peerKind: "",
  peerId: "",
  guildId: "",
  priority: 100,
  targetAgentId: "",
  enabled: true,
};

function ruleToForm(rule: RoutingRule): RuleFormState {
  return {
    id: rule.id,
    channel: rule.match.channel ?? "",
    accountId: rule.match.accountId ?? "",
    peerKind: rule.match.peerKind ?? "",
    peerId: rule.match.peerId ?? "",
    guildId: rule.match.guildId ?? "",
    priority: rule.priority,
    targetAgentId: rule.targetAgentId,
    enabled: rule.enabled,
  };
}

function formToPayload(form: RuleFormState): RoutingRuleInput {
  const match: RoutingRuleMatch = {};
  if (form.channel.trim()) match.channel = form.channel.trim();
  if (form.accountId.trim()) match.accountId = form.accountId.trim();
  if (form.peerKind) match.peerKind = form.peerKind;
  if (form.peerId.trim()) match.peerId = form.peerId.trim();
  if (form.guildId.trim()) match.guildId = form.guildId.trim();
  return {
    id: form.id,
    priority: Number.isFinite(form.priority) ? Math.trunc(form.priority) : 100,
    match,
    targetAgentId: form.targetAgentId,
    enabled: form.enabled,
  };
}

/** Compact "field=value" summary of a rule's match block. */
function matchSummary(rule: RoutingRule): string[] {
  const parts: string[] = [];
  if (rule.match.channel) parts.push(`channel=${rule.match.channel}`);
  if (rule.match.accountId) parts.push(`account=${rule.match.accountId}`);
  if (rule.match.peerKind) parts.push(`peer=${rule.match.peerKind}`);
  if (rule.match.peerId) parts.push(`peerId=${rule.match.peerId}`);
  if (rule.match.guildId) parts.push(`guild=${rule.match.guildId}`);
  if (parts.length === 0) parts.push("*");
  return parts;
}

export function RoutingRulesCard({ agents }: RoutingRulesCardProps) {
  const { t } = useTranslation("channels");
  const { rules, loading, saveRule, deleteRule } = useRoutingRules();
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState<RuleFormState>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<RoutingRule | null>(null);
  const [deleteLoading, setDeleteLoading] = useState(false);

  const agentName = (id: string) => {
    const agent = agents.find((a) => a.id === id);
    return agent?.display_name || agent?.agent_key || id.slice(0, 8);
  };

  const openCreate = () => {
    setForm(emptyForm);
    setFormOpen(true);
  };

  const openEdit = (rule: RoutingRule) => {
    setForm(ruleToForm(rule));
    setFormOpen(true);
  };

  const handleSave = async () => {
    if (!form.targetAgentId) {
      toast.error(t("routingRules.form.agentRequired"));
      return;
    }
    setSaving(true);
    try {
      await saveRule(formToPayload(form));
      setFormOpen(false);
      toast.success(t("routingRules.toasts.saved"));
    } catch (err) {
      toast.error(t("routingRules.toasts.saveFailed"), userFriendlyError(err));
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!deleteTarget) return;
    setDeleteLoading(true);
    try {
      await deleteRule(deleteTarget.id);
      setDeleteTarget(null);
      toast.success(t("routingRules.toasts.deleted"));
    } catch (err) {
      toast.error(t("routingRules.toasts.deleteFailed"), userFriendlyError(err));
    } finally {
      setDeleteLoading(false);
    }
  };

  const set = (patch: Partial<RuleFormState>) => setForm((f) => ({ ...f, ...patch }));

  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Workflow className="h-4 w-4" />
          {t("routingRules.title")}
        </CardTitle>
        <CardDescription>{t("routingRules.description")}</CardDescription>
        <div className="ms-auto">
          <Button size="sm" onClick={openCreate} className="gap-1">
            <Plus className="h-3.5 w-3.5" /> {t("routingRules.add")}
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <TableSkeleton />
        ) : rules.length === 0 ? (
          <EmptyState icon={ArrowDownNarrowWide} title={t("routingRules.emptyTitle")} description={t("routingRules.emptyDescription")} />
        ) : (
          <ul className="flex flex-col gap-2">
            {rules.map((rule) => (
              <li
                key={rule.id}
                className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
                      #{rule.priority}
                    </span>
                    <span className="truncate font-medium">{agentName(rule.targetAgentId)}</span>
                    {!rule.enabled && (
                      <span className="text-muted-foreground text-xs">({t("routingRules.disabled")})</span>
                    )}
                  </div>
                  <div className="text-muted-foreground mt-1 truncate font-mono text-xs">
                    {matchSummary(rule).join(" · ")}
                  </div>
                </div>
                <div className="flex shrink-0 gap-1">
                  <Button variant="ghost" size="icon" onClick={() => openEdit(rule)} aria-label={t("routingRules.edit")}>
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => setDeleteTarget(rule)}
                    aria-label={t("routingRules.delete")}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-destructive" />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>

      {formOpen && (
        <div className="border-t px-6 py-4">
          <h4 className="mb-3 text-sm font-semibold">
            {form.id ? t("routingRules.form.editTitle") : t("routingRules.form.createTitle")}
          </h4>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rr-channel">{t("routingRules.form.channel")}</Label>
              <Input
                id="rr-channel"
                value={form.channel}
                onChange={(e) => set({ channel: e.target.value })}
                placeholder={t("routingRules.form.anyPlaceholder")}
                className="text-base md:text-sm"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rr-account">{t("routingRules.form.accountId")}</Label>
              <Input
                id="rr-account"
                value={form.accountId}
                onChange={(e) => set({ accountId: e.target.value })}
                placeholder={t("routingRules.form.anyPlaceholder")}
                className="text-base md:text-sm"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>{t("routingRules.form.peerKind")}</Label>
              <Select value={form.peerKind || "any"} onValueChange={(v) => set({ peerKind: v === "any" ? "" : v })}>
                <SelectTrigger className="text-base md:text-sm">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="any">{t("routingRules.form.any")}</SelectItem>
                  <SelectItem value="direct">{t("routingRules.form.peerDirect")}</SelectItem>
                  <SelectItem value="group">{t("routingRules.form.peerGroup")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rr-peer-id">{t("routingRules.form.peerId")}</Label>
              <Input
                id="rr-peer-id"
                value={form.peerId}
                onChange={(e) => set({ peerId: e.target.value })}
                placeholder={t("routingRules.form.anyPlaceholder")}
                className="text-base md:text-sm"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rr-guild">{t("routingRules.form.guildId")}</Label>
              <Input
                id="rr-guild"
                value={form.guildId}
                onChange={(e) => set({ guildId: e.target.value })}
                placeholder={t("routingRules.form.anyPlaceholder")}
                className="text-base md:text-sm"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="rr-priority">{t("routingRules.form.priority")}</Label>
              <Input
                id="rr-priority"
                type="number"
                value={form.priority}
                onChange={(e) => set({ priority: Number(e.target.value) })}
                className="text-base md:text-sm"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label>{t("routingRules.form.targetAgent")}</Label>
              <Select value={form.targetAgentId} onValueChange={(v) => set({ targetAgentId: v })}>
                <SelectTrigger className="text-base md:text-sm">
                  <SelectValue placeholder={t("routingRules.form.selectAgent")} />
                </SelectTrigger>
                <SelectContent>
                  {agents.map((a) => (
                    <SelectItem key={a.id} value={a.id}>
                      {a.display_name || a.agent_key || a.id.slice(0, 8)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end gap-2 pb-1">
              <Switch id="rr-enabled" checked={form.enabled} onCheckedChange={(v) => set({ enabled: v })} />
              <Label htmlFor="rr-enabled">{t("routingRules.form.enabled")}</Label>
            </div>
          </div>
          <div className="mt-4 flex gap-2">
            <Button size="sm" onClick={handleSave} disabled={saving}>
              {t("routingRules.form.save")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => setFormOpen(false)}>
              {t("routingRules.form.cancel")}
            </Button>
          </div>
        </div>
      )}

      <ConfirmDeleteDialog
        open={!!deleteTarget}
        onOpenChange={(v) => !v && setDeleteTarget(null)}
        title={t("routingRules.deleteTitle")}
        description={t("routingRules.deleteDescription")}
        confirmValue={deleteTarget ? agentName(deleteTarget.targetAgentId) : ""}
        confirmLabel={t("routingRules.delete")}
        onConfirm={handleDelete}
        loading={deleteLoading}
      />
    </Card>
  );
}
