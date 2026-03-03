import { useState, useEffect, useCallback } from "react";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import { Methods, Events } from "@/api/protocol";

export interface PendingPairing {
  code: string;
  sender_id: string;
  channel: string;
  chat_id: string;
  account_id: string;
  created_at: number;
  expires_at: number;
}

export interface PairedDevice {
  sender_id: string;
  channel: string;
  chat_id: string;
  paired_at: number;
  paired_by: string;
}

export function useNodes() {
  const ws = useWs();
  const [pendingPairings, setPendingPairings] = useState<PendingPairing[]>([]);
  const [pairedDevices, setPairedDevices] = useState<PairedDevice[]>([]);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    if (!ws.isConnected) return;
    setLoading(true);
    try {
      const res = await ws.call<{
        pending: PendingPairing[];
        paired: PairedDevice[];
      }>(Methods.PAIRING_LIST);
      setPendingPairings(res.pending ?? []);
      setPairedDevices(res.paired ?? []);
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [ws]);

  useEffect(() => {
    load();
  }, [load]);

  useWsEvent(Events.DEVICE_PAIR_REQUESTED, () => {
    load();
  });

  useWsEvent(Events.DEVICE_PAIR_RESOLVED, () => {
    load();
  });

  const approvePairing = useCallback(
    async (code: string) => {
      await ws.call(Methods.PAIRING_APPROVE, { code });
      load();
    },
    [ws, load],
  );

  const denyPairing = useCallback(
    async (code: string) => {
      await ws.call(Methods.PAIRING_DENY, { code });
      load();
    },
    [ws, load],
  );

  const revokePairing = useCallback(
    async (senderId: string, channel: string) => {
      await ws.call(Methods.PAIRING_REVOKE, { senderId, channel });
      load();
    },
    [ws, load],
  );

  return { pendingPairings, pairedDevices, loading, refresh: load, approvePairing, denyPairing, revokePairing };
}
