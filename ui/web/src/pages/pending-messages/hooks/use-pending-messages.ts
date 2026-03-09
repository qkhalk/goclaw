import { useState, useCallback } from "react";
import { useHttp } from "@/hooks/use-ws";
import type { PendingMessageGroup, PendingMessage } from "../types";

export function usePendingMessages() {
  const http = useHttp();
  const [groups, setGroups] = useState<PendingMessageGroup[]>([]);
  const [messages, setMessages] = useState<PendingMessage[]>([]);
  const [loading, setLoading] = useState(false);
  const [messagesLoading, setMessagesLoading] = useState(false);

  const loadGroups = useCallback(async () => {
    setLoading(true);
    try {
      const res = await http.get<{ groups: PendingMessageGroup[] }>("/v1/pending-messages");
      setGroups(res?.groups ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [http]);

  const loadMessages = useCallback(
    async (channel: string, key: string) => {
      setMessagesLoading(true);
      try {
        const res = await http.get<{ messages: PendingMessage[] }>("/v1/pending-messages/messages", {
          channel,
          key,
        });
        setMessages(res?.messages ?? []);
      } catch {
        // ignore
      } finally {
        setMessagesLoading(false);
      }
    },
    [http],
  );

  const compactGroup = useCallback(
    async (channel: string, key: string) => {
      try {
        await http.post("/v1/pending-messages/compact", {
          channel_name: channel,
          history_key: key,
        });
        return true;
      } catch {
        return false;
      }
    },
    [http],
  );

  const clearGroup = useCallback(
    async (channel: string, key: string) => {
      try {
        await http.delete(`/v1/pending-messages?channel=${encodeURIComponent(channel)}&key=${encodeURIComponent(key)}`);
        return true;
      } catch {
        return false;
      }
    },
    [http],
  );

  return {
    groups,
    messages,
    loading,
    messagesLoading,
    loadGroups,
    loadMessages,
    compactGroup,
    clearGroup,
  };
}
