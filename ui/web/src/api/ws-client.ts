import { generateId } from "@/lib/utils";
import { extractResumeToken, loadNodeSession, saveNodeSession } from "@/lib/node-session";
import type { ErrorShape, EventFrame, ResponseFrame } from "./protocol";
import { PROTOCOL_VERSION } from "./protocol";
import { ApiError } from "./errors";

/** Response payload of the node.hello lease handshake. */
interface NodeHelloResult {
  lease?: {
    nodeId?: string;
    expiresAt?: number | string | null;
    ttlSeconds?: number | null;
  } | null;
  replayed?: boolean;
  snapshotRequired?: boolean;
}

type EventListener = (payload: unknown, seq?: number) => void;

interface PendingRequest {
  resolve: (payload: unknown) => void;
  reject: (error: ApiError) => void;
  timeout: ReturnType<typeof setTimeout>;
}

export type ConnectionState =
  | "disconnected"
  | "connecting"
  | "connected"
  /** Socket dropped but a reconnect cycle is scheduled/underway; node lease keeps server state alive. */
  | "reconnecting";

export class WsClient {
  private ws: WebSocket | null = null;
  private pending = new Map<string, PendingRequest>();
  private eventListeners = new Map<string, Set<EventListener>>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;
  private authenticated = false;
  private intentionalClose = false;
  private pairingInProgress = false;
  private connectGeneration = 0;

  /** Server-assigned role from connect response. */
  role: "owner" | "admin" | "operator" | "viewer" | "" = "";

  /** Tenant fields from connect response. */
  tenantId = "";
  tenantName = "";
  tenantSlug = "";
  isOwner = false;
  /** Server-derived: caller qualifies for master-only actions (owner OR on master tenant). Advisory UI hint only — backend still enforces. */
  isMasterScope = false;
  /** Server edition, drives UI feature gating. */
  edition: "standard" | "lite" = "standard";
  serverVersion = "";

  private readonly maxReconnectDelay = 30_000;
  private readonly baseReconnectDelay = 1_000;
  private readonly defaultTimeout = 30_000;
  private readonly heartbeatIntervalMs = 15_000;

  /** Node-lease session (persisted across reloads via lib/node-session). */
  private nodeId = "";
  private clientId = "";
  private resumeToken = "";
  private lastSeenSeq = 0;
  /** Set when the gateway answers node.* with "unknown method" (legacy build): lease upkeep skipped. */
  private leaseUnsupported = false;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private consecutiveHeartbeatFailures = 0;

  onAuthFailure: (() => void) | null = null;

  onPairingRequired: ((code: string, senderID: string) => void) | null = null;

  /**
   * Fired once per successful (re)connect AFTER the node-lease handshake
   * settles. Lets the chat layer resync in-flight runs via runs.events.
   */
  private reconnectListeners = new Set<() => void>();

  /** Subscribe to post-handshake (re)connect notifications; returns unsubscribe. */
  onReconnected(listener: () => void): () => void {
    this.reconnectListeners.add(listener);
    return () => { this.reconnectListeners.delete(listener); };
  }

  /** Fan out the (re)connect notification; one failing listener never blocks others. */
  private notifyReconnected(): void {
    for (const fn of this.reconnectListeners) {
      try { fn(); } catch { /* ignore */ }
    }
  }

  constructor(
    private url: string,
    private getToken: () => string,
    private getUserId: () => string,
    private getSenderID: () => string,
    private onStateChange: (state: ConnectionState) => void,
  ) {}

  connect(): void {
    if (this.ws) return;

    // Distinguishing "connecting" (first ever attempt) from "reconnecting"
    // (a previous connection dropped) lets the UI show a Reconnecting banner
    // instead of a full offline state.
    const state = this.reconnectAttempts > 0 ? "reconnecting" : "connecting";
    this.onStateChange(state);

    const wsUrl = this.buildWsUrl();
    const socket = new WebSocket(wsUrl);
    const generation = ++this.connectGeneration;
    this.ws = socket;

    socket.onopen = () => {
      if (this.ws !== socket) return;
      this.reconnectAttempts = 0;
      this.authenticate(generation);
    };

    socket.onmessage = (event) => {
      this.handleMessage(event.data as string);
    };

    socket.onclose = () => {
      if (this.ws !== socket) return;

      this.stopHeartbeat();
      this.ws = null;
      this.authenticated = false;
      this.rejectAllPending("Connection closed");
      this.onStateChange(this.intentionalClose ? "disconnected" : "reconnecting");

      if (!this.intentionalClose) {
        this.scheduleReconnect();
      }
    };

    socket.onerror = () => {
      // onclose will fire after onerror
    };
  }

  disconnect(): void {
    this.intentionalClose = true;
    this.pairingInProgress = false;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopHeartbeat();
    if (this.ws) {
      // Best-effort goodbye so the gateway can release the lease immediately
      // instead of waiting for TTL expiry. Fire-and-forget with a hard timeout;
      // call() throws synchronously when the socket is already gone.
      try {
        if (this.nodeId && !this.leaseUnsupported) {
          void this.call("node.bye", { nodeId: this.nodeId }, 500).catch(() => {});
        }
      } catch {
        // socket already closing — nothing to release server-side
      }
      const socket = this.ws;
      this.ws = null;
      socket.close();
    }
    this.authenticated = false;
    this.rejectAllPending("Disconnected");
    this.onStateChange("disconnected");
  }

  get isConnected(): boolean {
    return this.authenticated && this.ws?.readyState === WebSocket.OPEN;
  }

  /**
   * Reset the timeout for a pending RPC call (e.g. when stream events arrive).
   */
  resetTimeout(requestId: string, timeoutMs: number): void {
    const pending = this.pending.get(requestId);
    if (!pending) return;
    clearTimeout(pending.timeout);
    pending.timeout = setTimeout(() => {
      this.pending.delete(requestId);
      pending.reject(new ApiError("AGENT_TIMEOUT", `timed out after ${timeoutMs}ms of inactivity`));
    }, timeoutMs);
  }

  /**
   * Send an RPC call and wait for the response.
   * Returns { promise, requestId } so callers can reset the timeout on activity.
   */
  callWithId<T = unknown>(
    method: string,
    params?: Record<string, unknown>,
    timeoutMs?: number,
  ): { promise: Promise<T>; requestId: string } {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new ApiError("UNAVAILABLE", "WebSocket not connected");
    }

    const id = generateId();
    const timeout = timeoutMs ?? this.defaultTimeout;

    const promise = new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new ApiError("AGENT_TIMEOUT", `${method} timed out after ${timeout}ms`));
      }, timeout);

      this.pending.set(id, {
        resolve: resolve as (p: unknown) => void,
        reject,
        timeout: timer,
      });

      this.ws!.send(
        JSON.stringify({ type: "req", id, method, params }),
      );
    });

    return { promise, requestId: id };
  }

  /**
   * Send an RPC call and wait for the response.
   */
  async call<T = unknown>(
    method: string,
    params?: Record<string, unknown>,
    timeoutMs?: number,
  ): Promise<T> {
    return this.callWithId<T>(method, params, timeoutMs).promise;
  }

  /**
   * Subscribe to a WebSocket event. Returns an unsubscribe function.
   */
  on(event: string, listener: EventListener): () => void {
    let listeners = this.eventListeners.get(event);
    if (!listeners) {
      listeners = new Set();
      this.eventListeners.set(event, listeners);
    }
    listeners.add(listener);

    return () => {
      listeners!.delete(listener);
      if (listeners!.size === 0) {
        this.eventListeners.delete(event);
      }
    };
  }

  private buildWsUrl(): string {
    if (this.url.startsWith("ws://") || this.url.startsWith("wss://")) {
      return this.url;
    }
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    return `${proto}//${host}${this.url}`;
  }

  private async authenticate(generation: number): Promise<void> {
    try {
      const res = await this.call<{
        role?: string;
        status?: string;
        pairing_code?: string;
        sender_id?: string;
        tenant_id?: string;
        tenant_name?: string;
        tenant_slug?: string;
        is_owner?: boolean;
        is_master_scope?: boolean;
        edition?: "standard" | "lite";
        server?: { name?: string; version?: string };
      }>("connect", {
        token: this.getToken(),
        user_id: this.getUserId(),
        sender_id: this.getSenderID(),
        locale: localStorage.getItem("goclaw:language") || "en",
        tenant_hint: localStorage.getItem("goclaw:tenant_hint") || "",
        tenant_id: localStorage.getItem("goclaw:tenant_id") || "",
        protocolVersion: PROTOCOL_VERSION,
      });
      if (this.connectGeneration !== generation) return;

      // Browser pairing: server requires approval
      if (res?.status === "pending_pairing" && res.pairing_code && res.sender_id) {
        if (!this.pairingInProgress) {
          this.pairingInProgress = true;
          this.onPairingRequired?.(res.pairing_code, res.sender_id);
        }
        // Keep connection alive for polling browser.pairing.status
        return;
      }
      this.pairingInProgress = false;

      // Server accepted connection but assigned viewer role → token is invalid
      if (this.getToken() && res?.role === "viewer") {
        this.intentionalClose = true;
        this.ws?.close();
        this.onAuthFailure?.();
        return;
      }

      this.authenticated = true;
      this.role = (res?.role as "owner" | "admin" | "operator" | "viewer") ?? "";
      this.tenantId = res?.tenant_id ?? "";
      this.tenantName = res?.tenant_name ?? "";
      this.tenantSlug = res?.tenant_slug ?? "";
      this.isOwner = res?.is_owner ?? false;
      this.isMasterScope = res?.is_master_scope ?? false;
      this.edition = res?.edition ?? "standard";
      this.serverVersion = res?.server?.version ?? "";
      this.onStateChange("connected");

      // Lease handshake runs AFTER the connection is usable: an older gateway
      // without node.* methods must not break login, so failures are swallowed.
      this.startNodeLease(generation);
    } catch (e) {
      if (this.connectGeneration === generation) {
        // Tenant access revoked → force logout instead of reconnect
        if (e instanceof ApiError && e.code === "TENANT_ACCESS_REVOKED") {
          this.intentionalClose = true;
          this.ws?.close();
          this.onAuthFailure?.();
          return;
        }
        this.intentionalClose = true;
        this.ws?.close();
      }
    }
  }

  /**
   * Node-lease handshake. Runs after authentication succeeds on every
   * connection (fresh or resumed). Sends the persisted nodeId/resumeToken and
   * lastSeenSeq so the gateway can resume the lease and replay missed events.
   * Best-effort: gateways without node.* support simply ignore it.
   */
  private async startNodeLease(generation: number): Promise<void> {
    if (this.leaseUnsupported) {
      // Legacy gateway (node.* unknown): no handshake, but the socket is live
      // and runs.* methods may still exist — resync must still fire.
      this.notifyReconnected();
      return;
    }
    const clientId = this.getUserId();
    if (!this.nodeId || this.clientId !== clientId) {
      const session = loadNodeSession(clientId);
      this.nodeId = session.nodeId;
      this.clientId = clientId;
      this.resumeToken = session.resumeToken;
      this.lastSeenSeq = session.lastSeenSeq;
    }

    try {
      const res = await this.call<NodeHelloResult>("node.hello", {
        nodeId: this.nodeId,
        clientId,
        resumeToken: this.resumeToken,
        lastSeenSeq: this.lastSeenSeq,
      });
      if (this.connectGeneration !== generation) return;

      // Accept a server-echoed token when present; keep ours otherwise.
      const echoed = extractResumeToken(res);
      if (echoed && echoed !== this.resumeToken) {
        this.resumeToken = echoed;
        this.persistNodeSession();
      }
    } catch (e) {
      if (this.connectGeneration !== generation) return;
      // Legacy gateway without node.* methods: mark unsupported so we don't
      // hammer it with hello/heartbeat and force reconnect cycles forever.
      if (e instanceof ApiError && e.code === "INVALID_REQUEST") {
        this.leaseUnsupported = true;
        return;
      }
      // Any other error is transient — the lease stays advisory; heartbeat
      // failures below will escalate to a reconnect if the socket is wedged.
    } finally {
      if (this.connectGeneration === generation) {
        this.startHeartbeat(generation);
        // Handshake settled (success, legacy-skip or transient failure) and
        // the socket is live: notify the chat layer to resync in-flight runs.
        this.notifyReconnected();
      }
    }
  }

  /** Periodic lease renewal; repeated failures force an immediate reconnect cycle. */
  private startHeartbeat(generation: number): void {
    this.stopHeartbeat();
    if (this.leaseUnsupported || !this.nodeId) return;
    this.consecutiveHeartbeatFailures = 0;
    this.heartbeatTimer = setInterval(() => {
      if (this.connectGeneration !== generation || !this.ws) {
        this.stopHeartbeat();
        return;
      }
      void this.call<{ expiresAt?: number | string | null; ttlSeconds?: number | null }>(
        "node.heartbeat",
        { nodeId: this.nodeId },
        10_000,
      )
        .then(() => {
          if (this.connectGeneration !== generation) return;
          this.consecutiveHeartbeatFailures = 0;
        })
        .catch(() => {
          if (this.connectGeneration !== generation) return;
          this.consecutiveHeartbeatFailures += 1;
          if (this.consecutiveHeartbeatFailures > 2) {
            this.consecutiveHeartbeatFailures = 0;
            // The socket is likely wedged: tear down and let scheduleReconnect
            // rebuild it with a fresh node.hello + event replay.
            this.ws?.close();
          }
        });
    }, this.heartbeatIntervalMs);
  }

  /** Clear the heartbeat timer (safe to call repeatedly). */
  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
  }

  /** Write current lease identity through to localStorage (best-effort). */
  private persistNodeSession(): void {
    saveNodeSession({
      v: 1,
      nodeId: this.nodeId,
      clientId: this.clientId,
      resumeToken: this.resumeToken,
      lastSeenSeq: this.lastSeenSeq,
    });
  }

  private handleMessage(data: string): void {
    let frame: { type: string };
    try {
      frame = JSON.parse(data);
    } catch {
      return;
    }

    if (frame.type === "res") {
      this.handleResponse(frame as ResponseFrame);
    } else if (frame.type === "event") {
      this.handleEvent(frame as EventFrame);
    }
  }

  private handleResponse(frame: ResponseFrame): void {
    const pending = this.pending.get(frame.id);
    if (!pending) return;

    this.pending.delete(frame.id);
    clearTimeout(pending.timeout);

    if (frame.ok) {
      pending.resolve(frame.payload);
    } else {
      const err = frame.error as ErrorShape;
      // Only force logout on tenant revocation (session-level invalidation).
      // UNAUTHORIZED from a method call means "insufficient permission for this action",
      // not "session expired" — let the caller handle it via the rejected promise.
      if (err.code === "TENANT_ACCESS_REVOKED") {
        this.onAuthFailure?.();
      }
      pending.reject(
        new ApiError(err.code, err.message, err.details, err.retryable),
      );
    }
  }

  private handleEvent(frame: EventFrame): void {
    const listeners = this.eventListeners.get(frame.event);
    if (listeners) {
      for (const fn of listeners) {
        try {
          fn(frame.payload, frame.seq);
        } catch {
          // Don't let one listener crash others
        }
      }
    }

    const wildcardListeners = this.eventListeners.get("*");
    if (wildcardListeners) {
      for (const fn of wildcardListeners) {
        try {
          fn({ event: frame.event, payload: frame.payload });
        } catch {
          // ignore
        }
      }
    }

    const seq = frame.seq;
    if (typeof seq === "number" && Number.isFinite(seq) && seq > this.lastSeenSeq) {
      this.lastSeenSeq = seq;
      // Persisted so a page reload can resume the lease with an accurate
      // lastSeenSeq; the blob is tiny, write-through is fine at event rates.
      this.persistNodeSession();
    }
  }

  private rejectAllPending(reason: string): void {
    for (const [, req] of this.pending) {
      clearTimeout(req.timeout);
      req.reject(new ApiError("UNAVAILABLE", reason));
    }
    this.pending.clear();
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;

    this.reconnectAttempts++;
    const delay = Math.min(
      this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts),
      this.maxReconnectDelay,
    );

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }
}
