import { useMemo, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Building2, MessageSquare, Plus, Trash2, UserCircle2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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

/** Per-provider "Phạm vi sử dụng" panel (admin): tenant default + per-group +
 * per-user account assignments. Resolution order in the agent is
 * explicit > group > user > tenant default > own accounts. */
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

  const providerAccounts = accounts.filter((a) => a.provider === provider);
  const accountOptions = providerAccounts.map((a) => ({
    id: a.id,
    label: a.shared ? `🏢 ${a.email}` : a.email,
  }));

  const providerBindings = bindings.filter((b) => b.provider === provider);
  const tenantBinding = providerBindings.find((b) => b.scope_type === "tenant");
  const groupBindings = providerBindings.filter((b) => b.scope_type === "group");
  const userBindings = providerBindings.filter((b) => b.scope_type === "user");

  const emailOf = (id: string) => providerAccounts.find((a) => a.id === id)?.email ?? id;

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

  async function handleAdd() {
    setError("");
    if (!addKey.trim() || !addAccount) {
      setError(t("scope.missing_fields"));
      return;
    }
    setSaving(true);
    try {
      await upsertBinding({ scope_type: addScope, scope_key: addKey.trim(), provider, account_id: addAccount });
      setAddKey("");
      setAddAccount("");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }

  const keySuggestions = addScope === "group" ? groupCandidates.map((g) => ({ id: g.id, label: g.label })) : userCandidates.map((u) => ({ id: u, label: u }));

  const renderBindingRows = (rows: typeof providerBindings, icon: ReactNode) =>
    rows.map((b) => (
      <div key={b.id} className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2">
        <div className="flex min-w-0 items-center gap-2">
          {icon}
          <div className="min-w-0">
            <p className="truncate text-sm">{b.scope_key || t("scope.tenant_default")}</p>
            <p className="truncate text-xs text-muted-foreground">→ {emailOf(b.account_id)}</p>
          </div>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="min-h-9 shrink-0 text-destructive hover:text-destructive"
          onClick={() => void deleteBinding(b.id)}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    ));

  return (
    <div className="rounded-lg border p-4">
      <p className="text-sm font-medium">{t("scope.title")}</p>
      <p className="mt-1 text-xs text-muted-foreground">{t("scope.description")}</p>

      {/* Tenant default */}
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

      {/* Existing group + user bindings */}
      {(groupBindings.length > 0 || userBindings.length > 0) && (
        <div className="mt-3 space-y-2">
          {renderBindingRows(groupBindings, <MessageSquare className="h-4 w-4 shrink-0" />)}
          {renderBindingRows(userBindings, <UserCircle2 className="h-4 w-4 shrink-0" />)}
        </div>
      )}

      {/* Add binding */}
      <div className="mt-4 border-t pt-3">
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-[130px_1fr_1fr_auto]">
          <Select value={addScope} onValueChange={(v) => { setAddScope(v as CloudBindingScopeType); setAddKey(""); }}>
            <SelectTrigger className="w-full min-w-0" dir="ltr">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="group">{t("scope.group")}</SelectItem>
              <SelectItem value="user">{t("scope.user")}</SelectItem>
            </SelectContent>
          </Select>
          <Input
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
          {addScope === "group" ? t("scope.key_hint_group") : t("scope.key_hint_user")}
        </p>
      </div>

      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
    </div>
  );
}
