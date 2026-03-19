import { useState, useEffect, useMemo } from "react";
import { Plus, Trash2, Loader2, Shield } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { Combobox, type ComboboxOption } from "@/components/ui/combobox";
import { useConfigPermissions } from "../hooks/use-config-permissions";
import { useContactSearch } from "../hooks/use-contact-search";

const CONFIG_TYPES = [
  { value: "heartbeat", label: "Heartbeat" },
  { value: "cron", label: "Cron" },
  { value: "context_files", label: "Context Files" },
  { value: "*", label: "All (*)" },
] as const;

const SCOPES = [
  { value: "agent", label: "Agent" },
  { value: "*", label: "Global (*)" },
] as const;

interface AgentPermissionsTabProps {
  agentId: string;
}

export function AgentPermissionsTab({ agentId }: AgentPermissionsTabProps) {
  const { t } = useTranslation("agents");
  const { permissions, loading, load, grant, revoke } = useConfigPermissions(agentId);

  const [userId, setUserId] = useState("");
  const [configType, setConfigType] = useState("heartbeat");
  const [scope, setScope] = useState("agent");
  const [permission, setPermission] = useState("allow");
  const [adding, setAdding] = useState(false);

  const { contacts } = useContactSearch(userId);
  const contactOptions: ComboboxOption[] = useMemo(() =>
    contacts.map((c) => {
      const name = c.display_name || c.sender_id;
      const username = c.username ? ` @${c.username}` : "";
      const channel = c.channel_type ? ` [${c.channel_type}]` : "";
      return {
        value: c.sender_id,
        label: `${name}${username} (${c.sender_id})${channel}`,
      };
    }),
    [contacts],
  );

  useEffect(() => { load(); }, [load]);

  const handleAdd = async () => {
    if (!userId.trim()) return;
    setAdding(true);
    await grant(scope, configType, userId.trim(), permission);
    setUserId("");
    setAdding(false);
  };

  return (
    <div className="space-y-3">
      <div>
        <h3 className="text-sm font-medium flex items-center gap-2">
          <Shield className="h-4 w-4 text-amber-500" />
          {t("permissions.title")}
        </h3>
        <p className="text-xs text-muted-foreground mt-1">{t("permissions.description")}</p>
      </div>

      {/* Inline add row */}
      <div className="flex flex-wrap items-end gap-2">
        <Combobox
          value={userId}
          onChange={setUserId}
          options={contactOptions}
          placeholder={t("permissions.userIdPlaceholder")}
          className="flex-1 min-w-[140px]"
        />
        <Select value={configType} onValueChange={setConfigType}>
          <SelectTrigger className="w-[120px] text-base md:text-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CONFIG_TYPES.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={scope} onValueChange={setScope}>
          <SelectTrigger className="w-[100px] text-base md:text-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SCOPES.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={permission} onValueChange={setPermission}>
          <SelectTrigger className="w-[90px] text-base md:text-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="allow">Allow</SelectItem>
            <SelectItem value="deny">Deny</SelectItem>
          </SelectContent>
        </Select>
        <Button size="icon" className="h-9 w-9 shrink-0" onClick={handleAdd} disabled={adding || !userId.trim()}>
          {adding ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
        </Button>
      </div>

      {/* Rules list */}
      {loading ? (
        <div className="flex items-center justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : permissions.length === 0 ? (
        <p className="text-xs text-muted-foreground text-center py-6">{t("permissions.empty")}</p>
      ) : (
        <div className="rounded-lg border divide-y">
          {permissions.map((p) => (
            <div key={p.id} className="flex items-center justify-between gap-2 px-3 py-2">
              <div className="flex items-center gap-2 min-w-0 text-sm">
                <Badge
                  variant={p.permission === "allow" ? "success" : "destructive"}
                  className="text-[10px] shrink-0"
                >
                  {p.permission}
                </Badge>
                <span className="font-medium truncate">{p.userId}</span>
                <span className="text-[11px] text-muted-foreground shrink-0">{p.configType}</span>
                <span className="text-[11px] text-muted-foreground shrink-0">@ {p.scope}</span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="h-7 w-7 p-0 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() => revoke(p.scope, p.configType, p.userId)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
