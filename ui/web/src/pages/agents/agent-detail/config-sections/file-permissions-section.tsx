import { useTranslation } from "react-i18next";
import { FileEdit, FilePlus2, FileSearch, Send } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import type { ToolPolicyConfig } from "@/types/agent";

/** File capability groups → the filesystem tools they gate. Compiles into
 * tools_config.deny (deny wins over allow, so the toggles hold even when an
 * allow-list profile is active). */
const FILE_TOOL_GROUPS: {
  key: "read" | "write" | "create" | "send";
  tools: string[];
}[] = [
  { key: "read", tools: ["read_file", "list_files"] },
  { key: "write", tools: ["edit"] },
  { key: "create", tools: ["write_file"] },
  { key: "send", tools: ["send_file"] },
];

interface FilePermissionsSectionProps {
  enabled: boolean;
  value: ToolPolicyConfig;
  onToggle: (v: boolean) => void;
  onChange: (v: ToolPolicyConfig) => void;
}

/** Per-agent file permissions: friendly switches for read / edit / create /
 * send file capabilities, stored as deny entries inside tools_config. */
export function FilePermissionsSection({
  enabled,
  value,
  onToggle,
  onChange,
}: FilePermissionsSectionProps) {
  const { t } = useTranslation("agents");
  const s = "configSections.filePermissions";

  const denied = new Set(value.deny ?? []);
  const isEnabled = (tools: string[]) => !tools.some((tool) => denied.has(tool));

  const setGroup = (tools: string[], on: boolean) => {
    const next = new Set(denied);
    for (const tool of tools) {
      if (on) next.delete(tool);
      else next.add(tool);
    }
    const deny = [...next];
    const nextValue: ToolPolicyConfig = {
      ...value,
      ...(deny.length > 0 ? { deny } : { deny: [] }),
    };
    if (deny.length === 0 && !value.profile && !value.allow && !value.alsoAllow) {
      // Nothing left restricted and no other custom policy — drop back to default.
      if (enabled) onToggle(false);
      return;
    }
    if (!enabled) onToggle(true);
    onChange(nextValue);
  };

  const icons = {
    read: FileSearch,
    write: FileEdit,
    create: FilePlus2,
    send: Send,
  } as const;

  // No master switch here — the per-capability switches ARE the toggle, and
  // flipping one off implicitly enables the custom tool policy below.
  return (
    <section className="space-y-2">
      <div>
        <h3 className="text-sm font-medium">{t(`${s}.title`)}</h3>
        <p className="text-xs text-muted-foreground">{t(`${s}.description`)}</p>
      </div>
      <div className="space-y-1 rounded-lg border p-3">
        {FILE_TOOL_GROUPS.map((g) => {
          const Icon = icons[g.key];
          const on = isEnabled(g.tools);
          return (
            <div
              key={g.key}
              className="flex items-center justify-between gap-4 rounded-md border px-3 py-2"
            >
              <div className="flex min-w-0 items-center gap-2.5">
                <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
                <div className="min-w-0">
                  <p className="text-sm font-medium leading-tight">{t(`${s}.${g.key}`)}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {t(`${s}.${g.key}Hint`)}
                  </p>
                </div>
              </div>
              <Switch checked={on} onCheckedChange={(v) => setGroup(g.tools, v)} />
            </div>
          );
        })}
        <p className="px-1 text-xs text-muted-foreground">{t(`${s}.footnote`)}</p>
      </div>
    </section>
  );
}
