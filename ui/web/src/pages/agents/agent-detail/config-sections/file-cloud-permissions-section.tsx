import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { FolderPlus, FolderOpen, PencilLine } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { toast } from "@/stores/use-toast-store";
import type { AgentData } from "@/types/agent";

/** Per-agent file/cloud capability toggles (agents.other_config.file_policy).
 * Read gates file/cloud browsing tools (list_files, read_file, cloud_ls,
 * cloud_read, cloud_fetch, cloud_about, media readers), write gates modifying
 * existing content (write_file overwrite, edit, and the cloud mutations
 * cloud_write/cloud_delete/cloud_move/cloud_share), create gates new files,
 * folders and drive copies (write_file to a new path, cloud_mkdir,
 * cloud_copy). Unset capabilities default to enabled on the backend, so the
 * UI treats a missing file_policy as all-on. */

interface FilePolicyState {
  read: boolean;
  write: boolean;
  create: boolean;
}

const ALL_ON: FilePolicyState = { read: true, write: true, create: true };

function readFilePolicy(agent: AgentData): FilePolicyState {
  const bag = (agent.other_config ?? {}) as Record<string, unknown>;
  const fp = bag.file_policy as Partial<FilePolicyState> | undefined;
  if (!fp || typeof fp !== "object") return ALL_ON;
  return {
    read: fp.read !== false,
    write: fp.write !== false,
    create: fp.create !== false,
  };
}

/** Builds the next other_config bag with file_policy applied (or removed when
 * all-on so the stored config stays clean). Spread-merge preserves unrelated
 * keys owned by other sections. */
export function applyFilePolicy(
  currentBag: Record<string, unknown> | null | undefined,
  state: FilePolicyState,
): Record<string, unknown> {
  const bag = { ...(currentBag ?? {}) };
  if (state.read && state.write && state.create) {
    delete bag.file_policy;
  } else {
    bag.file_policy = { read: state.read, write: state.write, create: state.create };
  }
  return bag;
}

interface FileCloudPermissionsSectionProps {
  agent: AgentData;
  onUpdate: (updates: Record<string, unknown>) => Promise<void>;
}

export function FileCloudPermissionsSection({ agent, onUpdate }: FileCloudPermissionsSectionProps) {
  const { t } = useTranslation("agents");
  const s = "fileCloudPermissions";

  const [state, setState] = useState<FilePolicyState>(() => readFilePolicy(agent));
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setState(readFilePolicy(agent));
  }, [agent.other_config]);

  const saved = readFilePolicy(agent);
  const dirty =
    state.read !== saved.read || state.write !== saved.write || state.create !== saved.create;

  const handleSave = async () => {
    setSaving(true);
    try {
      await onUpdate({
        other_config: applyFilePolicy(agent.other_config, state),
      });
      toast.success(t(`${s}.saved`));
    } catch {
      // toast shown by the update hook
    } finally {
      setSaving(false);
    }
  };

  const rows: Array<{
    key: keyof FilePolicyState;
    label: string;
    desc: string;
    icon: typeof FolderOpen;
  }> = [
    { key: "read", label: t(`${s}.read`), desc: t(`${s}.readDesc`), icon: FolderOpen },
    { key: "write", label: t(`${s}.write`), desc: t(`${s}.writeDesc`), icon: PencilLine },
    { key: "create", label: t(`${s}.create`), desc: t(`${s}.createDesc`), icon: FolderPlus },
  ];

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">{t(`${s}.title`)}</h3>
        <p className="text-xs text-muted-foreground">{t(`${s}.description`)}</p>
      </div>
      <div className="rounded-lg border p-3 sm:p-4">
        <div className="space-y-1">
          {rows.map(({ key, label, desc, icon: Icon }) => (
            <div
              key={key}
              className="flex min-h-11 items-center justify-between gap-3 rounded-md px-1 py-2"
            >
              <div className="flex min-w-0 items-start gap-2.5">
                <Icon className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
                <div className="min-w-0">
                  <p className="text-sm font-medium leading-tight">{label}</p>
                  <p className="text-xs text-muted-foreground">{desc}</p>
                </div>
              </div>
              <Switch
                checked={state[key]}
                onCheckedChange={(v) => setState((prev) => ({ ...prev, [key]: v }))}
                aria-label={label}
              />
            </div>
          ))}
        </div>
        {dirty && (
          <div className="mt-3 flex justify-end">
            <Button size="sm" onClick={handleSave} disabled={saving} className="min-h-11 sm:min-h-9">
              {saving ? t(`${s}.saving`) : t(`${s}.save`)}
            </Button>
          </div>
        )}
      </div>
    </section>
  );
}
