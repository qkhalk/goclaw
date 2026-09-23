import { useTranslation } from "react-i18next";
import type { AgentData } from "@/types/agent";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { PipelineTab } from "./pipeline-tab";
import { MemoryTab } from "./memory-tab";
import { KnowledgeTab } from "./knowledge-tab";
import { OrchestrationTab } from "./orchestration-tab";

interface V3CapabilitiesModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Agent whose live capability state is shown alongside the explainer. */
  agent: AgentData;
  /** Switch the agent-detail page to its Evolution tab (orchestration strip). */
  onOpenEvolution?: () => void;
}

export function V3CapabilitiesModal({
  open,
  onOpenChange,
  agent,
  onOpenEvolution,
}: V3CapabilitiesModalProps) {
  const { t } = useTranslation("v3-capabilities");
  const agentId = agent.id;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl max-h-[85dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("subtitle")}</DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="pipeline">
          <TabsList className="grid h-auto w-full grid-cols-2 sm:inline-flex sm:h-9">
            <TabsTrigger value="pipeline">{t("tabs.pipeline")}</TabsTrigger>
            <TabsTrigger value="memory">{t("tabs.memory")}</TabsTrigger>
            <TabsTrigger value="knowledge">{t("tabs.knowledge")}</TabsTrigger>
            <TabsTrigger value="orchestration">
              {t("tabs.orchestration")}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="pipeline">
            <PipelineTab agent={agent} agentId={agentId} />
          </TabsContent>
          <TabsContent value="memory">
            <MemoryTab agentId={agentId} />
          </TabsContent>
          <TabsContent value="knowledge">
            <KnowledgeTab agent={agent} agentId={agentId} />
          </TabsContent>
          <TabsContent value="orchestration">
            <OrchestrationTab
              agentId={agentId}
              onOpenEvolution={
                onOpenEvolution
                  ? () => {
                      onOpenEvolution();
                      onOpenChange(false);
                    }
                  : undefined
              }
            />
          </TabsContent>
        </Tabs>

        <p className="text-xs text-muted-foreground">{t("note")}</p>
      </DialogContent>
    </Dialog>
  );
}
