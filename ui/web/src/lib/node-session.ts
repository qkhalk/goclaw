// Durable node-session persistence for the WS reconnect/lease flow.
// The browser acts as a lease-holding "node": nodeId is a stable per-browser
// identity, resumeToken is client-generated and rotated when the server echoes
// one back, and lastSeenSeq is the highest event seq observed so the gateway
// can replay missed events after a drop.
// Schema is versioned ({ v: 1, ... }) to allow future migration.

import { uniqueId } from "./utils";

const STORAGE_KEY = "goclaw.node.session";

interface NodeSessionData {
  /** Schema version for future migrations. */
  v: number;
  /** Stable per-browser node identity. */
  nodeId: string;
  /** Authenticated user the lease belongs to; rotates when a different user logs in. */
  clientId: string;
  /** Client-generated resume token, sent in node.hello; server may echo its own. */
  resumeToken: string;
  /** Highest event frame seq observed on any connection. */
  lastSeenSeq: number;
}

/** Server response payload of `node.hello` / `node.heartbeat` (lease object). */
export interface NodeLease {
  nodeId?: string;
  expiresAt?: number | string | null;
  ttlSeconds?: number | null;
}

/**
 * Extract a resume token from a node.hello response if the gateway echoes one.
 * Older gateways return only the lease; both shapes are accepted defensively.
 */
export function extractResumeToken(payload: unknown): string | undefined {
  if (!payload || typeof payload !== "object") return undefined;
  const rec = payload as Record<string, unknown>;
  const direct = typeof rec.resumeToken === "string" && rec.resumeToken.length > 0 ? rec.resumeToken : undefined;
  if (direct) return direct;
  const nested = rec.lease as Record<string, unknown> | null | undefined;
  if (nested && typeof nested === "object" && typeof nested.resumeToken === "string" && nested.resumeToken.length > 0) {
    return nested.resumeToken;
  }
  return undefined;
}

/** Fully validated session shape returned by loadNodeSession. */
export interface LoadedNodeSession {
  v: number;
  nodeId: string;
  clientId: string;
  resumeToken: string;
  lastSeenSeq: number;
}

/**
 * Load the persisted session, creating it if absent or unreadable.
 * Corrupted or wrong-version entries are discarded and regenerated fresh
 * (lastSeenSeq=0 makes the gateway replay from scratch / snapshot — safe).
 * When the stored clientId differs from the current user, identity is rotated
 * so one user never inherits another user's lease or replay cursor.
 */
export function loadNodeSession(clientId: string): LoadedNodeSession {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as Partial<NodeSessionData> & { v?: number };
      if (
        parsed.v === 1 &&
        typeof parsed.nodeId === "string" &&
        parsed.nodeId.length > 0 &&
        typeof parsed.resumeToken === "string" &&
        parsed.resumeToken.length > 0 &&
        typeof parsed.lastSeenSeq === "number" &&
        Number.isFinite(parsed.lastSeenSeq) &&
        parsed.lastSeenSeq >= 0
      ) {
        if (parsed.clientId === clientId) {
          return { v: 1, nodeId: parsed.nodeId, clientId, resumeToken: parsed.resumeToken, lastSeenSeq: parsed.lastSeenSeq };
        }
        // Same browser, different user: keep the stable nodeId, rotate secrets.
        const rotated: LoadedNodeSession = { v: 1, nodeId: parsed.nodeId, clientId, resumeToken: uniqueId(), lastSeenSeq: 0 };
        saveNodeSession(rotated);
        return rotated;
      }
    }
  } catch {
    // fall through to fresh generation below
  }
  const fresh: LoadedNodeSession = { v: 1, nodeId: uniqueId(), clientId, resumeToken: uniqueId(), lastSeenSeq: 0 };
  saveNodeSession(fresh);
  return fresh;
}

/** Persist the session. Storage failures are swallowed (private mode etc.). */
export function saveNodeSession(session: NodeSessionData): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(session));
  } catch {
    // ignore quota/privacy errors — reconnect still works without persistence
  }
}
