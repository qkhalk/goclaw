import { useState, useEffect, useRef, useLayoutEffect } from "react";
import { createPortal } from "react-dom";
import { useTranslation } from "react-i18next";
import { Bot, ChevronDown } from "lucide-react";
import { usePortalDropdownClose } from "@/hooks/use-portal-dropdown-close";
import { useAgents } from "@/hooks/use-agents";
import { stripLeadingEmoji } from "@/lib/agent-emoji";
import type { AgentData } from "@/types/agent";

interface AgentSelectorProps {
  value: string;
  onChange: (agentId: string) => void;
  /** Increment to open the dropdown programmatically (empty-state CTA). */
  openSignal?: number;
}

/** Extract emoji from agent top-level field */
function agentEmoji(agent: AgentData): string | undefined {
  return agent.emoji || undefined;
}

export function AgentSelector({ value, onChange, openSignal }: AgentSelectorProps) {
  const { t } = useTranslation("common");
  const { data: allAgents = [] } = useAgents();
  const agents = allAgents.filter((a) => a.status === "active");
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const [dropdownStyle, setDropdownStyle] = useState<React.CSSProperties>({});

  // Open the dropdown when a parent asks (empty-state CTA increments the signal).
  useEffect(() => {
    if (openSignal === undefined || openSignal === 0) return;
    setOpen(true);
  }, [openSignal]);

  useLayoutEffect(() => {
    if (!open || !containerRef.current) return;
    const rect = containerRef.current.getBoundingClientRect();
    setDropdownStyle({
      position: "fixed",
      top: rect.bottom + 4,
      left: rect.left,
      width: rect.width,
      zIndex: 9999,
    });
  }, [open]);

  usePortalDropdownClose({
    open,
    onClose: () => setOpen(false),
    ignore: [containerRef, dropdownRef],
  });

  const selected = agents.find((a) => a.agent_key === value);
  const selectedEmoji = selected ? agentEmoji(selected) : undefined;

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-lg border bg-background px-3 py-2 text-sm hover:bg-accent"
      >
        {selectedEmoji ? (
          <span className="text-base shrink-0">{selectedEmoji}</span>
        ) : (
          <Bot className="h-4 w-4 shrink-0 text-muted-foreground" />
        )}
        <span className="flex-1 truncate text-left font-medium">
          {stripLeadingEmoji(selectedEmoji, selected?.display_name ?? selected?.agent_key ?? (value || t("selectAgent")))}
        </span>
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
      </button>

      {open && createPortal(
        <div
          ref={dropdownRef}
          style={dropdownStyle}
          className="pointer-events-auto max-h-60 sm:max-h-80 overflow-y-auto rounded-lg border bg-popover p-1 shadow-md"
        >
          {agents.length === 0 && (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              {t("noAgentsAvailable")}
            </div>
          )}
          {agents.map((agent) => {
            const emoji = agentEmoji(agent);
            return (
              <button
                key={agent.agent_key}
                type="button"
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => { onChange(agent.agent_key); setOpen(false); }}
                className={`flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-accent ${
                  agent.agent_key === value ? "bg-accent" : ""
                }`}
              >
                {emoji ? (
                  <span className="text-base shrink-0">{emoji}</span>
                ) : (
                  <Bot className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <span className="flex-1 truncate text-left">
                  {stripLeadingEmoji(emoji, agent.display_name || agent.agent_key)}
                </span>
                {agent.is_default && (
                  <span className="text-xs text-muted-foreground">{t("default")}</span>
                )}
              </button>
            );
          })}
        </div>,
        document.body,
      )}
    </div>
  );
}
