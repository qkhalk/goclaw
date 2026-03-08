import { useState } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { ArrowLeft, Bot, Star, Trash2, Sparkles } from "lucide-react";
import { useAgentDetail } from "../hooks/use-agent-detail";
import { useAgents } from "../hooks/use-agents";
import { AgentGeneralTab } from "./agent-general-tab";
import { AgentConfigTab } from "./agent-config-tab";
import { AgentFilesTab } from "./agent-files-tab";
import { AgentSharesTab } from "./agent-shares-tab";
import { AgentLinksTab } from "./agent-links-tab";
import { AgentSkillsTab } from "./agent-skills-tab";
import { AgentInstancesTab } from "./agent-instances-tab";
import { SummoningModal } from "../summoning-modal";
import { ConfirmDeleteDialog } from "@/components/shared/confirm-delete-dialog";
import { DetailPageSkeleton } from "@/components/shared/loading-skeleton";

interface AgentDetailPageProps {
  agentId: string;
  onBack: () => void;
}

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function agentDisplayName(agent: { display_name?: string; agent_key: string }) {
  if (agent.display_name) return agent.display_name;
  if (UUID_RE.test(agent.agent_key)) return "Unnamed Agent";
  return agent.agent_key;
}

function agentSubtitle(agent: { display_name?: string; agent_key: string; id: string }) {
  // Don't show agent_key if it equals the id (both are UUID) and there's no display_name
  if (!agent.display_name && agent.agent_key === agent.id) return null;
  // Show agent_key as subtitle (truncate if UUID)
  if (UUID_RE.test(agent.agent_key)) return agent.agent_key.slice(0, 8) + "…";
  return agent.agent_key;
}

export function AgentDetailPage({ agentId, onBack }: AgentDetailPageProps) {
  const { agent, files, loading, updateAgent, getFile, setFile, regenerateAgent, resummonAgent, refresh } =
    useAgentDetail(agentId);
  const { deleteAgent: deleteAgentById } = useAgents();
  const [summoningOpen, setSummoningOpen] = useState(false);
  const [activeTab, setActiveTab] = useState("general");
  const [deleteOpen, setDeleteOpen] = useState(false);

  const handleRegenerate = async (prompt: string) => {
    await regenerateAgent(prompt);
    setSummoningOpen(true);
  };

  const handleResummon = async () => {
    await resummonAgent();
    setSummoningOpen(true);
  };

  // Refresh data after modal closes (not when completed fires)
  const handleSummoningClose = (open: boolean) => {
    setSummoningOpen(open);
    if (!open) refresh();
  };

  if (loading || !agent) {
    return <DetailPageSkeleton tabs={6} />;
  }

  const title = agentDisplayName(agent);
  const subtitle = agentSubtitle(agent);

  return (
    <TooltipProvider>
    <div className="p-4 sm:p-6">
      {/* Header */}
      <div className="mb-6 flex items-start gap-4">
        <Button variant="ghost" size="icon" onClick={onBack} className="mt-0.5 shrink-0">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
          <Bot className="h-6 w-6" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-xl font-semibold">{title}</h2>
            {agent.is_default && (
              <Star className="h-4 w-4 shrink-0 fill-amber-400 text-amber-400" />
            )}
            <Badge variant={agent.status === "active" ? "success" : agent.status === "summon_failed" ? "destructive" : "secondary"}>
              {agent.status === "summon_failed" ? "Summon Failed" : agent.status}
            </Badge>
          </div>
          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-sm text-muted-foreground">
            {subtitle && (
              <>
                <span className="font-mono text-xs">{subtitle}</span>
                <span className="text-border">|</span>
              </>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge variant="outline" className="text-[11px]">{agent.agent_type}</Badge>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-[260px] text-xs">
                {agent.agent_type === "predefined"
                  ? "Shared context files (SOUL.md, IDENTITY.md, AGENTS.md) with per-user USER.md instances."
                  : "Each user gets their own full set of context files."}
              </TooltipContent>
            </Tooltip>
            {agent.agent_type === "predefined" && (() => {
              const evolving = Boolean((agent.other_config as Record<string, unknown> | null)?.self_evolve);
              return (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Badge
                      variant={evolving ? "default" : "outline"}
                      className={`text-[11px] ${evolving ? "bg-violet-100 text-violet-700 hover:bg-violet-100 dark:bg-violet-900/30 dark:text-violet-300" : "text-muted-foreground"}`}
                    >
                      <Sparkles className="mr-0.5 h-3 w-3" />
                      {evolving ? "Evolving" : "Static"}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent side="bottom" className="max-w-[240px] text-xs">
                    {evolving
                      ? "Agent can self-evolve its communication style (SOUL.md). Identity and workflow stay fixed."
                      : "Agent style is static. Enable Self-Evolution in General tab to allow style adaptation."}
                  </TooltipContent>
                </Tooltip>
              );
            })()}
            {agent.provider && (
              <>
                <span className="text-border">|</span>
                <span>{agent.provider} / {agent.model}</span>
              </>
            )}
          </div>
        </div>
        <Button
          variant="ghost"
          size="icon"
          className="shrink-0 text-muted-foreground hover:text-destructive"
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>

      {/* Tabs */}
      <div className="max-w-4xl rounded-xl border bg-card p-3 shadow-sm sm:p-4">
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="w-full justify-start overflow-x-auto overflow-y-hidden">
            <TabsTrigger value="general">General</TabsTrigger>
            <TabsTrigger value="config">Config</TabsTrigger>
            <TabsTrigger value="files">Files</TabsTrigger>
            <TabsTrigger value="shares">Shares</TabsTrigger>
            <TabsTrigger value="links">Links</TabsTrigger>
            <TabsTrigger value="skills">Skills</TabsTrigger>
            {agent.agent_type === "predefined" && (
              <TabsTrigger value="instances">Instances</TabsTrigger>
            )}
          </TabsList>

          <TabsContent value="general" className="mt-4">
            <AgentGeneralTab agent={agent} onUpdate={updateAgent} />
          </TabsContent>

          <TabsContent value="config" className="mt-4">
            <AgentConfigTab agent={agent} onUpdate={updateAgent} />
          </TabsContent>

          <TabsContent value="files" className="mt-4">
            <AgentFilesTab
              agent={agent}
              files={files}
              onGetFile={getFile}
              onSetFile={setFile}
              onRegenerate={handleRegenerate}
              onResummon={handleResummon}
            />
          </TabsContent>

          <TabsContent value="shares" className="mt-4">
            <AgentSharesTab agentId={agentId} />
          </TabsContent>

          <TabsContent value="links" className="mt-4">
            <AgentLinksTab agentId={agentId} />
          </TabsContent>

          <TabsContent value="skills" className="mt-4">
            <AgentSkillsTab agentId={agentId} />
          </TabsContent>

          {agent.agent_type === "predefined" && (
            <TabsContent value="instances" className="mt-4">
              <AgentInstancesTab agentId={agentId} />
            </TabsContent>
          )}

        </Tabs>
      </div>

      <SummoningModal
        open={summoningOpen}
        onOpenChange={handleSummoningClose}
        agentId={agentId}
        agentName={title}
        onCompleted={() => {}}
        onResummon={async () => { await resummonAgent(); }}
      />

      <ConfirmDeleteDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Delete Agent"
        description={`Are you sure you want to delete "${title}"? All context files, sessions, and configuration will be permanently removed.`}
        confirmValue={agent.display_name || agent.agent_key}
        confirmLabel="Delete"
        onConfirm={async () => {
          await deleteAgentById(agentId);
          setDeleteOpen(false);
          onBack();
        }}
      />
    </div>
    </TooltipProvider>
  );
}
