/** Agent keys of seeded system workers that power dedicated tool pages
 * (video studio, pptx studio). They are managed by the gateway and hidden
 * from the main chat + agents surfaces so they don't pollute lists that
 * are meant for user-facing conversational agents. */
export const SYSTEM_AGENT_KEYS = new Set(["video-designer", "pptx-designer"]);

export function isSystemAgent(agentKey?: string | null): boolean {
  return !!agentKey && SYSTEM_AGENT_KEYS.has(agentKey);
}
