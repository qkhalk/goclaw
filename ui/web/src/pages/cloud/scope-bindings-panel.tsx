import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Building2, Plus, Trash2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSessions } from "@/pages/sessions/hooks/use-sessions";
import {
  useCloudAccounts,
  useCloudBindings,
  type CloudBinding,
  type CloudBindingScopeType,
  type CloudProvider,
} from "./hooks/use-cloud";

/** Sentinel for "no account bound" (Radix Select forbids empty values). */
const NONE = "__none__";

/** Derive group-chat candidates from session keys
 * (`agent:<agent>:<channel>:group:<chatId>[:topic:<n>]`). */
function useGroupCandidates() {
  const { sessions } = useSessions({ limit: 200 });
  return useMemo(() => {
    const seen = new Map<string, string>(); // chatId → display label
    for (const s of sessions) {
      const idx = s.key?.indexOf(":group:");
      if (!s.key || idx < 0) continue;
      const rest = s.key.slice(idx + ":group:".length);
      const chatId = rest.split(":")[0];
      if (!chatId || seen.has(chatId)) continue;
      const channel = s.key.split(":")[2] ?? "";
      seen.set(chatId, `${channel} · ${chatId}`);
    }
    return [...seen.entries()].map(([id, label]) => ({ id, label }));
  }, [sessions]);
}

/** Derive user candidates from sessions (userID values). */
function useUserCandidates() {
  const { sessions } = useSessions({ limit: 200 });
  return useMemo(() => {
    const seen = new Set<string>();
    for (const s of sessions) {
      const uid = s.userID?.trim();
      if (uid && !uid.includes(":")) seen.add(uid);
    }
    return [...seen];
  }, [sessions]);
}

function AccountSelect({
  value,
  onChange,
  options,
  placeholder,
  className,
}: {
  value: string;
  onChange: (v: string) => void;
  options: { id: string; label: string }[];
  placeholder: string;
  className?: string;
}) {
  return (
    <Select value={value || undefined} onValueChange={onChange}>
      <SelectTrigger className={className ?? "w-full min-w-0"} dir="ltr">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent className="max-h-64">
        {options.map((o) => (
          <SelectItem key={o.id} value={o.id}>
            {o.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** Per-provider "Usage scopes" panel (admin, rendered in the settings sheet):
 * tenant default on its own row, then a table of group/user rules with
 * enable/disable, inline account switch, priority and delete. Resolution
 * order in the agent is explicit > group > user > tenant default > own
 * accounts; disabled rules are ignored, lower priority wins within a tier. */
export function ScopeBindingsPanel({ provider }: { provider: CloudProvider }) {
  const { t } = useTranslation("cloud");
  const { accounts } = useCloudAccounts();
  const { bindings, upsertBinding, deleteBinding } = useCloudBindings(true);
  const groupCandidates = useGroupCandidates();
  const userCandidates = useUserCandidates();

  const [addScope, setAddScope] = useState<CloudBindingScopeType>("group");
  const [addKey, setAddKey] = useState("");
  const [addAccount, setAddAccount] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [addedFlash, setAddedFlash] = useState(false);
  const keyInputRef = useRef<HTMLInputElement>(null);

  // "Rule added" confirmation fades after a moment.
  useEffect(() => {
    if (!addedFlash) return;
    const id = setTimeout(() => setAddedFlash(false), 2000);
    return () => clearTimeout(id);
  }, [addedFlash]);

  const providerAccounts = accounts.filter((a) => a.provider === provider);
  const accountOptions = providerAccounts.map((a) => ({
    id: a.id,
    label: a.shared ? `🏢 ${a.email}` : a.email,
  }));

  const providerBindings = bindings.filter((b) => b.provider === provider);
  const tenantBinding = providerBindings.find((b) => b.scope_type === "tenant");
  const ruleBindings = providerBindings.filter((b) => b.scope_type !== "tenant");

  // Radix Select forbids empty-string item values — use a sentinel for "no
  // default account" and translate it to a delete.
  async function handleSetTenant(selection: string) {
    setError("");
    try {
      if (selection !== NONE) {
        await upsertBinding({ scope_type: "tenant", scope_key: "", provider, account_id: selection });
      } else if (tenantBinding) {
        await deleteBinding(tenantBinding.id);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  /** Shared upsert for inline edits (toggle / account / priority): same
   * scope + key overwrites the existing rule. */
  async function patchBinding(b: CloudBinding, patch: Partial<{ account_id: string; enabled: boolean; priority: number }>) {
    setError("");
    try {
      await upsertBinding({
        scope_type: b.scope_type,
        scope_key: b.scope_key,
        provider: b.provider,
        account_id: patch.account_id ?? b.account_id,
        enabled: patch.enabled ?? b.enabled,
        priority: patch.priority ?? b.priority,
      });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  async function handleAdd() {
    setError("");
    if (!addKey.trim() || !addAccount) {
      setError(t("scope.missing_fields"));
      return;
    }
    setSaving(true);
    try {
      await upsertBinding({ scope_type: addScope, scope_key: addKey.trim(), provider, account_id: addAccount });
      // Ready for the next rule: keep type + account, clear the key, refocus.
      setAddKey("");
      setAddedFlash(true);
      keyInputRef.current?.focus();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  const keySuggestions = addScope === "group" ? groupCandidates.map((g) => ({ id: g.id, label: g.label })) : userCandidates.map((u) => ({ id: u, label: u }));

  const scopeTypeLabel = (st: CloudBindingScopeType) =>
    st === "group" ? t("scope.group") : t("scope.user");

  return (
    <div className="rounded-lg border p-4">
      <p className="text-sm font-medium">{t("scope.title")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("scope.description")}</p>

      {/* Tenant default (single rule, separate row — NONE unbinds) */}
      <div className="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="flex shrink-0 items-center gap-1.5 text-sm sm:w-52">
          <Building2 className="h-4 w-4 shrink-0" />
          {t("scope.tenant_default")}
        </div>
        <AccountSelect
          value={tenantBinding?.account_id || NONE}
          onChange={(v) => void handleSetTenant(v)}
          options={[{ id: NONE, label: t("scope.none") }, ...accountOptions]}
          placeholder={t("scope.none")}
        />
      </div>

      {/* Group + user rules table */}
      <div className="mt-4">
        <p className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
          {t("settings.scope_rules")}
        </p>
        <div className="mt-2 overflow-x-auto">
          <table className="w-full min-w-[600px] text-sm">
            <thead>
              <tr className="border-b text-left text-xs text-muted-foreground">
                <th className="py-2 pr-3 font-medium">{t("scope.enabled")}</th>
                <th className="py-2 pr-3 font-medium">{t("scope.col_type")}</th>
                <th className="py-2 pr-3 font-medium">{t("scope.col_target")}</th>
                <th className="py-2 pr-3 font-medium">{t("scope.col_account")}</th>
                <th className="py-2 pr-3 font-medium">{t("scope.priority")}</th>
                <th className="py-2 text-right font-medium">{t("scope.col_actions")}</th>
              </tr>
            </thead>
            <tbody>
              {ruleBindings.map((b) => (
                <tr key={b.id} className="border-b last:border-0">
                  <td className="py-2 pr-3">
                    <Switch
                      size="sm"
                      checked={b.enabled}
                      onCheckedChange={(v) => void patchBinding(b, { enabled: v })}
                      aria-label={b.enabled ? t("scope.enabled") : t("scope.disabled")}
                    />
                  </td>
                  <td className="py-2 pr-3">
                    <Badge variant={b.enabled ? "info" : "outline"}>{scopeTypeLabel(b.scope_type)}</Badge>
                  </td>
                  <td className="max-w-[180px] py-2 pr-3">
                    <p className="truncate" title={b.scope_key}>{b.scope_key}</p>
                  </td>
                  <td className="py-2 pr-3">
                    <AccountSelect
                      value={b.account_id}
                      onChange={(v) => void patchBinding(b, { account_id: v })}
                      options={accountOptions}
                      placeholder={t("scope.pick_account")}
                      className="w-[180px] min-w-0"
                    />
                  </td>
                  <td className="py-2 pr-3">
                    <Input
                      type="number"
                      min={0}
                      max={1000}
                      defaultValue={b.priority}
                      key={`${b.id}-${b.priority}`}
                      className="h-8 w-20 text-base md:text-sm"
                      aria-label={t("scope.priority")}
                      onBlur={(e) => {
                        const v = Number.parseInt(e.target.value, 10);
                        if (!Number.isNaN(v) && v >= 0 && v <= 1000 && v !== b.priority) {
                          void patchBinding(b, { priority: v });
                        }
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
                      }}
                    />
                  </td>
                  <td className="py-2 text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="min-h-9 text-destructive hover:text-destructive"
                      onClick={() => void deleteBinding(b.id)}
                      aria-label={t("scope.col_actions")}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {ruleBindings.length === 0 && (
            <p className="py-3 text-center text-xs text-muted-foreground">{t("scope.table_empty")}</p>
          )}
        </div>
      </div>

      {/* Add rule */}
      <div className="mt-4 border-t pt-3">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-[130px_1fr_1fr_auto]">
          <Select
            value={addScope}
            onValueChange={(v) => { setAddScope(v as CloudBindingScopeType); setAddKey(""); }}
          >
            <SelectTrigger className="w-full min-w-0" dir="ltr" aria-label={t("scope.scope_type")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="group">{t("scope.group")}</SelectItem>
              <SelectItem value="user">{t("scope.user")}</SelectItem>
            </SelectContent>
          </Select>
          <Input
            ref={keyInputRef}
            value={addKey}
            onChange={(e) => setAddKey(e.target.value)}
            list="cloud-scope-key-suggestions"
            placeholder={addScope === "group" ? t("scope.key_placeholder_group") : t("scope.key_placeholder_user")}
            className="text-base md:text-sm"
            autoComplete="off"
          />
          <datalist id="cloud-scope-key-suggestions">
            {keySuggestions.map((k) => (
              <option key={k.id} value={k.id}>{k.label}</option>
            ))}
          </datalist>
          <AccountSelect
            value={addAccount}
            onChange={setAddAccount}
            options={accountOptions}
            placeholder={t("scope.pick_account")}
          />
          <Button size="sm" onClick={handleAdd} disabled={saving} className="min-h-11 sm:min-h-9">
            <Plus className="mr-1 h-4 w-4" />
            {t("scope.add")}
          </Button>
        </div>
        <p className="mt-1.5 text-[11px] text-muted-foreground">
          {addedFlash ? t("scope.rule_added") : addScope === "group" ? t("scope.key_hint_group") : t("scope.key_hint_user")}
        </p>
      </div>

      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
    </div>
  );
}
