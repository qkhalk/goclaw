import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChevronDown, ChevronRight, SlidersHorizontal } from "lucide-react";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ToolNameSelect } from "@/components/shared/tool-name-select";

/** Subagent builder extras in the create dialog: an explicit tool allowlist,
 * a custom system prompt (stored as IDENTITY.md and preserved through LLM
 * summoning), and the AGENTS.md workspace-instructions injection toggle. */
export function AgentSubagentSection({
  allowedTools,
  onAllowedToolsChange,
  systemPrompt,
  onSystemPromptChange,
  injectAgentsMd,
  onInjectAgentsMdChange,
}: {
  allowedTools: string[];
  onAllowedToolsChange: (tools: string[]) => void;
  systemPrompt: string;
  onSystemPromptChange: (prompt: string) => void;
  injectAgentsMd: boolean;
  onInjectAgentsMdChange: (on: boolean) => void;
}) {
  const { t } = useTranslation("agents");
  const [open, setOpen] = useState(false);

  return (
    <div className="rounded-lg border">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-sm"
      >
        <span className="flex items-center gap-2 font-medium">
          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          <SlidersHorizontal className="h-4 w-4 text-muted-foreground" />
          {t("create.subagent.title")}
        </span>
        <span className="text-xs text-muted-foreground">
          {allowedTools.length > 0
            ? t("create.subagent.toolCount", { count: allowedTools.length })
            : t("create.subagent.allTools")}
        </span>
      </button>

      {open && (
        <div className="space-y-4 border-t px-3 py-3">
          <div className="space-y-1.5">
            <Label className="text-sm">{t("create.subagent.allowedTools")}</Label>
            <p className="text-xs text-muted-foreground">
              {t("create.subagent.allowedToolsHint")}
            </p>
            <ToolNameSelect
              value={allowedTools}
              onChange={onAllowedToolsChange}
              placeholder={t("create.subagent.allowedToolsPlaceholder")}
            />
          </div>

          <div className="space-y-1.5">
            <Label className="text-sm">{t("create.subagent.systemPrompt")}</Label>
            <p className="text-xs text-muted-foreground">
              {t("create.subagent.systemPromptHint")}
            </p>
            <Textarea
              value={systemPrompt}
              onChange={(e) => onSystemPromptChange(e.target.value)}
              placeholder={t("create.subagent.systemPromptPlaceholder")}
              rows={5}
              className="text-base md:text-sm"
            />
          </div>

          <div className="flex items-center justify-between gap-4">
            <div className="space-y-0.5">
              <Label htmlFor="inject-agents-md" className="cursor-pointer text-sm font-normal">
                {t("create.subagent.injectAgents")}
              </Label>
              <p className="text-xs text-muted-foreground">
                {t("create.subagent.injectAgentsHint")}
              </p>
            </div>
            <Switch
              id="inject-agents-md"
              checked={injectAgentsMd}
              onCheckedChange={onInjectAgentsMdChange}
            />
          </div>
        </div>
      )}
    </div>
  );
}
