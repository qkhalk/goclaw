import { useCallback, useMemo, useState } from "react";
import { uniqueId } from "@/lib/utils";
import { useChatMessages } from "@/pages/chat/hooks/use-chat-messages";
import { useChatSend } from "@/pages/chat/hooks/use-chat-send";
import type { AttachedFile, ComposerOverrides } from "@/components/chat/chat-input";
import type { ChatMessage } from "@/types/chat";

/**
 * Chat brain of the PPTX designer column. It is a thin re-wiring of the
 * main chat hooks onto a dedicated session of the design-only
 * "pptx-designer" agent, so streaming, run activity and history replay all
 * behave exactly like /chat.
 *
 * The conversation id persists in localStorage so the designer keeps its
 * thread across reloads; "New chat" rotates it and the old session stays in
 * the session list.
 */

export const DESIGNER_AGENT_KEY = "pptx-designer";
const CONV_STORAGE_KEY = "goclaw.pptx-designer-conv";

function loadConvId(): string {
  try {
    const existing = localStorage.getItem(CONV_STORAGE_KEY);
    if (existing) return existing;
    const fresh = uniqueId();
    localStorage.setItem(CONV_STORAGE_KEY, fresh);
    return fresh;
  } catch {
    // storage unavailable — ephemeral conversation for this tab only
    return uniqueId();
  }
}

export function useDesignerChat() {
  const [convId, setConvId] = useState(loadConvId);

  const sessionKey = useMemo(
    () => `agent:${DESIGNER_AGENT_KEY}:ws:direct:${convId}`,
    [convId],
  );

  const {
    messages,
    streamText,
    thinkingText,
    toolStream,
    isRunning,
    isBusy,
    loading,
    activity,
    blockReplies,
    expectRun,
    addLocalMessage,
  } = useChatMessages(sessionKey, DESIGNER_AGENT_KEY);

  const handleMessageAdded = useCallback(
    (msg: ChatMessage, key?: string) => {
      addLocalMessage(msg, key);
    },
    [addLocalMessage],
  );

  const { send, abort, error: sendError } = useChatSend({
    agentId: DESIGNER_AGENT_KEY,
    onMessageAdded: handleMessageAdded,
    onExpectRun: expectRun,
  });

  const handleSend = useCallback(
    (message: string, files?: AttachedFile[], overrides?: ComposerOverrides) => {
      send(message, sessionKey, files, overrides);
    },
    [send, sessionKey],
  );

  const handleAbort = useCallback(() => {
    abort(sessionKey);
  }, [abort, sessionKey]);

  const newChat = useCallback(() => {
    const fresh = uniqueId();
    try {
      localStorage.setItem(CONV_STORAGE_KEY, fresh);
    } catch {
      // storage unavailable — rotation still applies for this tab
    }
    setConvId(fresh);
  }, []);

  // Resume a past designer conversation from the session list. Only accepts
  // keys of this agent's direct-WS shape; the trailing segment is the convId.
  const openSession = useCallback(
    (key: string) => {
      const prefix = `agent:${DESIGNER_AGENT_KEY}:ws:direct:`;
      if (!key.startsWith(prefix)) return;
      const fresh = key.slice(prefix.length);
      if (!fresh || fresh === convId) return;
      try {
        localStorage.setItem(CONV_STORAGE_KEY, fresh);
      } catch {
        // storage unavailable — switch still applies for this tab
      }
      setConvId(fresh);
    },
    [convId],
  );

  return {
    sessionKey,
    openSession,
    messages,
    streamText,
    thinkingText,
    toolStream,
    isRunning,
    isBusy,
    loading,
    activity,
    blockReplies,
    sendError,
    send: handleSend,
    abort: handleAbort,
    newChat,
  };
}
