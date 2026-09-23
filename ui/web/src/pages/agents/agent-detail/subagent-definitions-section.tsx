import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Pencil, Plus, Trash2, Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Combobox } from "@/components/ui/combobox";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ConfirmDeleteDialog } from "@/components/shared/confirm-delete-dialog";
import { EmptyState } from "@/components/shared/empty-state";
import { useProviders } from "@/pages/providers/hooks/use-providers";
import { useProviderModels } from "@/pages/providers/hooks/use-provider-models";
import { useBuiltinTools } from "@/pages/builtin-tools/hooks/use-builtin-tools";
import { useSubagentDefinitions } from "./hooks/use-subagent-definitions";
import type { SubagentDefinition } from "@/types/agent";

/**
 * "Subagent Definitions" tab of the agent detail page. Manages named,
 * reusable spawn templates persisted in the agent's subagents_config JSONB.
 * The LLM picks one by name via the spawn tool's "definition" parameter; the
 * backend then applies its model, allowed tools, system prompt, and optional
 * AGENTS.md injection.
 */

interface DraftState {
  originalName: string | null; // null = create
  name: string;
  model: string;
  description: string;
  /** "all" = unrestricted; "custom" = only draft.allowedTools. */
  toolsMode: "all" | "custom";
  allowedTools: string[];
  systemPrompt: string;
  injectAgentsMd: boolean;
}

function emptyDraft(): DraftState {
  return {
    originalName: null,
    name: "",
    model: "",
    description: "",
    toolsMode: "all",
    allowedTools: [],
    systemPrompt: "",
    injectAgentsMd: false,
  };
}

const SLUG_RE = /^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$/;

export function SubagentDefinitionsSection({ agentId }: { agentId: string }) {
  const { t } = useTranslation("agents");
  const { definitions, loading, createDefinition, updateDefinition, deleteDefinition } =
    useSubagentDefinitions(agentId);

  const [dialogOpen, setDialogOpen] = useState(false);
  const [draft, setDraft] = useState<DraftState>(emptyDraft);
  const [nameError, setNameError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<SubagentDefinition | null>(null);

  // Model options: enabled providers + the selected provider's models
  // (same source the agent create dialog uses).
  const { providers } = useProviders();
  const enabledProviders = useMemo(
    () => providers.filter((p) => p.enabled),
    [providers],
  );
  const [draftProviderName, setDraftProviderName] = useState<string>("");
  const draftProvider = useMemo(
    () => enabledProviders.find((p) => p.name === draftProviderName) ?? enabledProviders[0],
    [enabledProviders, draftProviderName],
  );
  const { models, loading: modelsLoading } = useProviderModels(draftProvider?.id);

  // Registered tool names for the allowed-tools multi-select.
  const { tools: builtinTools } = useBuiltinTools();

  const openCreate = () => {
    setDraft(emptyDraft());
    setDraftProviderName("");
    setNameError(null);
    setDialogOpen(true);
  };

  const openEdit = (def: SubagentDefinition) => {
    setDraft({
      originalName: def.name,
      name: def.name,
      model: def.model ?? "",
      description: def.description ?? "",
      toolsMode: (def.allowedTools?.length ?? 0) > 0 ? "custom" : "all",
      allowedTools: def.allowedTools ?? [],
      systemPrompt: def.systemPrompt ?? "",
      injectAgentsMd: def.injectAgentsMd ?? false,
    });
    setDraftProviderName("");
    setNameError(null);
    setDialogOpen(true);
  };

  const handleSave = async () => {
    const name = draft.name.trim();
    if (!SLUG_RE.test(name)) {
      setNameError(t("subagentDefs.form.nameInvalid"));
      return;
    }
    const lower = name.toLowerCase();
    const conflict = definitions.some(
      (d) => d.name.toLowerCase() === lower && d.name !== draft.originalName,
    );
    if (conflict) {
      setNameError(t("subagentDefs.form.nameTaken"));
      return;
    }
    const payload: SubagentDefinition = {
      name,
      model: draft.model.trim() || undefined,
      description: draft.description.trim() || undefined,
      allowedTools:
        draft.toolsMode === "custom" && draft.allowedTools.length > 0
          ? draft.allowedTools
          : undefined,
      systemPrompt: draft.systemPrompt.trim() || undefined,
      injectAgentsMd: draft.injectAgentsMd,
    };
    setSaving(true);
    try {
      if (draft.originalName) {
        await updateDefinition(draft.originalName, payload);
      } else {
        await createDefinition(payload);
      }
      setDialogOpen(false);
    } catch {
      // toast already shown by the hook
    } finally {
      setSaving(false);
    }
  };

  const toggleTool = (tool: string) => {
    setDraft((d) => ({
      ...d,
      allowedTools: d.allowedTools.includes(tool)
        ? d.allowedTools.filter((x) => x !== tool)
        : [...d.allowedTools, tool],
    }));
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="text-sm font-semibold">{t("subagentDefs.title")}</h3>
          <p className="text-xs text-muted-foreground">{t("subagentDefs.description")}</p>
        </div>
        <Button size="sm" onClick={openCreate} className="gap-1">
          <Plus className="h-4 w-4" /> {t("subagentDefs.add")}
        </Button>
      </div>

      {loading ? (
        <div className="rounded-lg border p-4 text-sm text-muted-foreground">
          {t("subagentDefs.loading")}
        </div>
      ) : definitions.length === 0 ? (
        <EmptyState
          icon={Workflow}
          title={t("subagentDefs.emptyTitle")}
          description={t("subagentDefs.emptyDescription")}
        />
      ) : (
        <div className="flex flex-col gap-2">
          {definitions.map((def) => (
            <div
              key={def.name}
              className="rounded-lg border p-3"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{def.name}</span>
                {def.model && (
                  <Badge variant="secondary" className="max-w-[220px] truncate font-normal">
                    {def.model}
                  </Badge>
                )}
                {def.injectAgentsMd && (
                  <Badge variant="outline" className="font-normal">
                    {t("subagentDefs.badgeAgentsMd")}
                  </Badge>
                )}
                <Badge variant="outline" className="font-normal">
                  {t("subagentDefs.badgeTools", { count: def.allowedTools?.length ?? 0 })}
                </Badge>
                <div className="ml-auto flex items-center gap-1">
                  <Button variant="ghost" size="icon" className="h-8 w-8" onClick={() => openEdit(def)}>
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive"
                    onClick={() => setDeleteTarget(def)}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
              {def.description && (
                <p className="mt-1 text-xs text-muted-foreground">{def.description}</p>
              )}
              {def.systemPrompt && (
                <p className="mt-1 line-clamp-2 text-xs text-muted-foreground/80 whitespace-pre-wrap">
                  {def.systemPrompt}
                </p>
              )}
            </div>
          ))}
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {draft.originalName ? t("subagentDefs.dialog.editTitle") : t("subagentDefs.dialog.createTitle")}
            </DialogTitle>
            <DialogDescription>{t("subagentDefs.dialog.description")}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="subdef-name">{t("subagentDefs.form.name")}</Label>
              <Input
                id="subdef-name"
                value={draft.name}
                placeholder="research-helper"
                disabled={draft.originalName != null}
                onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
              />
              {draft.originalName != null && (
                <p className="text-xs text-muted-foreground">{t("subagentDefs.form.nameLocked")}</p>
              )}
              {nameError && <p className="text-xs text-destructive">{nameError}</p>}
            </div>

            <div className="space-y-2">
              <Label>{t("subagentDefs.form.model")}</Label>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <Select
                  value={draftProvider?.name ?? "__none__"}
                  onValueChange={(v) => setDraftProviderName(v === "__none__" ? "" : v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t("subagentDefs.form.provider")} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__none__">{t("subagentDefs.form.provider")}</SelectItem>
                    {enabledProviders.map((p) => (
                      <SelectItem key={p.name} value={p.name}>
                        {p.display_name || p.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Combobox
                  value={draft.model}
                  onChange={(v) => setDraft((d) => ({ ...d, model: v }))}
                  options={models.map((m) => ({ value: m.id, label: m.name ?? m.id }))}
                  placeholder={
                    modelsLoading ? t("subagentDefs.form.loadingModels") : t("subagentDefs.form.modelInherit")
                  }
                  allowCustom
                />
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="subdef-desc">{t("subagentDefs.form.description")}</Label>
              <Input
                id="subdef-desc"
                value={draft.description}
                placeholder={t("subagentDefs.form.descriptionPlaceholder")}
                onChange={(e) => setDraft((d) => ({ ...d, description: e.target.value }))}
              />
            </div>

            <div className="space-y-2">
              <Label>{t("subagentDefs.form.allowedTools")}</Label>
              <div className="flex flex-wrap items-center gap-2">
                <Select
                  value={draft.toolsMode}
                  onValueChange={(v) =>
                    setDraft((d) => ({ ...d, toolsMode: v === "custom" ? "custom" : "all" }))
                  }
                >
                  <SelectTrigger className="w-full sm:w-[240px]">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t("subagentDefs.form.toolsModeAll")}</SelectItem>
                    <SelectItem value="custom">{t("subagentDefs.form.toolsModeCustom")}</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">
                  {t("subagentDefs.form.allowedToolsHint")}
                </p>
              </div>
              {draft.toolsMode === "custom" && (
                <>
                  {draft.allowedTools.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {draft.allowedTools.map((tool) => (
                        <button
                          key={tool}
                          type="button"
                          className="inline-flex max-w-full items-center gap-1 rounded-md border bg-secondary px-2 py-0.5 text-xs hover:bg-secondary/80"
                          onClick={() => toggleTool(tool)}
                        >
                          <span className="truncate">{tool}</span>
                          <span aria-hidden>×</span>
                        </button>
                      ))}
                    </div>
                  )}
                  <div className="max-h-44 overflow-y-auto rounded-md border p-2">
                    {builtinTools.length === 0 ? (
                      <p className="p-1 text-xs text-muted-foreground">{t("subagentDefs.form.noToolsLoaded")}</p>
                    ) : (
                      <div className="grid grid-cols-1 gap-1 sm:grid-cols-2">
                        {builtinTools.map((tool) => (
                          <label
                            key={tool.name}
                            className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-sm hover:bg-accent"
                          >
                            <Checkbox
                              checked={draft.allowedTools.includes(tool.name)}
                              onCheckedChange={() => toggleTool(tool.name)}
                            />
                            <span className="truncate">{tool.display_name || tool.name}</span>
                          </label>
                        ))}
                      </div>
                    )}
                  </div>
                </>
              )}
            </div>

            <div className="space-y-2">
              <Label htmlFor="subdef-prompt">{t("subagentDefs.form.systemPrompt")}</Label>
              <Textarea
                id="subdef-prompt"
                value={draft.systemPrompt}
                rows={6}
                placeholder={t("subagentDefs.form.systemPromptPlaceholder")}
                onChange={(e) => setDraft((d) => ({ ...d, systemPrompt: e.target.value }))}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
              <div>
                <Label htmlFor="subdef-agentsmd">{t("subagentDefs.form.injectAgentsMd")}</Label>
                <p className="text-xs text-muted-foreground">{t("subagentDefs.form.injectAgentsMdHint")}</p>
              </div>
              <Switch
                id="subdef-agentsmd"
                checked={draft.injectAgentsMd}
                onCheckedChange={(checked) => setDraft((d) => ({ ...d, injectAgentsMd: !!checked }))}
              />
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              {t("subagentDefs.form.cancel")}
            </Button>
            <Button onClick={handleSave} disabled={saving || !draft.name.trim()}>
              {saving ? t("subagentDefs.form.saving") : t("subagentDefs.form.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDeleteDialog
        open={!!deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title={t("subagentDefs.delete.title")}
        description={t("subagentDefs.delete.description", { name: deleteTarget?.name ?? "" })}
        confirmValue={deleteTarget?.name ?? ""}
        onConfirm={async () => {
          if (deleteTarget) {
            await deleteDefinition(deleteTarget.name);
            setDeleteTarget(null);
          }
        }}
      />
    </div>
  );
}
