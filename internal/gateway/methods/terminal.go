package methods

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// TerminalMethods implements the terminal.* WS surface (Paseo plan Phase 4 /
// §25): one PTY per terminal tab, streamed to clients via WS events with a
// bounded in-memory ring buffer for replay. Only session metadata is
// persisted; raw output is never written to disk.
type TerminalMethods struct {
	tstore  store.TerminalStore
	wsStore store.WorkspaceStore
	pub     bus.EventPublisher
	mgr     *ptyManager
}

// NewTerminalMethods wires the terminal surface. Any nil dependency yields a
// stub whose handlers reply unavailable, keeping registration nil-safe.
func NewTerminalMethods(tstore store.TerminalStore, wsStore store.WorkspaceStore, pub bus.EventPublisher) *TerminalMethods {
	return &TerminalMethods{
		tstore:  tstore,
		wsStore: wsStore,
		pub:     pub,
		mgr:     newPtyManager(),
	}
}

// Register wires the terminal.* methods into the method router.
func (m *TerminalMethods) Register(router *gateway.MethodRouter) {
	router.Register(protocol.MethodTerminalCreate, m.handleCreate)
	router.Register(protocol.MethodTerminalList, m.handleList)
	router.Register(protocol.MethodTerminalAttach, m.handleAttach)
	router.Register(protocol.MethodTerminalInput, m.handleInput)
	router.Register(protocol.MethodTerminalResize, m.handleResize)
	router.Register(protocol.MethodTerminalClose, m.handleClose)
}

// emit broadcasts one bus event scoped to the tenant. Every terminal payload
// carries "userId" so the gateway event filter can scope delivery to the
// owning user (fail-closed when absent).
func (m *TerminalMethods) emit(event string, payload map[string]any, tenantID uuid.UUID) {
	if m.pub == nil {
		return
	}
	m.pub.Broadcast(bus.Event{Name: event, Payload: payload, TenantID: tenantID})
}

// wired reports whether all dependencies are present; unwired surfaces reply
// unavailable instead of panicking.
func (m *TerminalMethods) wired() bool {
	return m.tstore != nil && m.wsStore != nil && m.pub != nil
}

// guard performs the shared preflight for every handler: authentication,
// wiring, and the operator role floor (viewers get no terminal access).
// ok=false means a response was already sent.
func (m *TerminalMethods) guard(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) bool {
	if client.Role() == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, "not authenticated"))
		return false
	}
	locale := store.LocaleFromContext(ctx)
	if !m.wired() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnavailable, "terminal not wired"))
		return false
	}
	if !permissions.HasMinRole(client.Role(), permissions.RoleOperator) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "terminal requires operator role")))
		return false
	}
	return true
}

// terminalJSON is the camelCase wire form of store.TerminalSession.
type terminalJSON struct {
	ID          string  `json:"id"`
	TenantID    *string `json:"tenantId,omitempty"`
	UserID      string  `json:"userId,omitempty"`
	WorkspaceID string  `json:"workspaceId"`
	CWD         string  `json:"cwd"`
	Shell       string  `json:"shell"`
	Status      string  `json:"status"`
	ExitCode    *int    `json:"exitCode,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

func toTerminalJSON(s *store.TerminalSession) terminalJSON {
	return terminalJSON{
		ID:          s.ID,
		TenantID:    s.TenantID,
		UserID:      s.UserID,
		WorkspaceID: s.WorkspaceID,
		CWD:         s.CWD,
		Shell:       s.Shell,
		Status:      s.Status,
		ExitCode:    s.ExitCode,
		CreatedAt:   s.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		UpdatedAt:   s.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
	}
}

// terminalCreateParams are the terminal.create inputs.
type terminalCreateParams struct {
	WorkspaceID string `json:"workspaceId"`
	CWD         string `json:"cwd"`
	Shell       string `json:"shell"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
}

// handleCreate spawns a shell in the workspace (terminal.create).
// Operator-only. Params: { workspaceId required, cwd?, shell?, cols?, rows? }.
// The store row is created first so a stable id exists for streaming; on PTY
// start failure it transitions to closed and the caller sees an internal
// error.
func (m *TerminalMethods) handleCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params terminalCreateParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.WorkspaceID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "workspaceId")))
		return
	}

	ws, err := m.wsStore.GetWorkspace(ctx, params.WorkspaceID)
	if err != nil || ws == nil || !canManageWorkspace(client, ws) {
		// Existence hiding: unauthorized callers see not-found.
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "workspace", params.WorkspaceID)))
		return
	}
	root := ws.RootPath
	if ws.WorktreePath != nil && *ws.WorktreePath != "" {
		root = *ws.WorktreePath
	}
	cwd, err := safeJoin(root, params.CWD)
	if err != nil {
		slog.Warn("security.terminal_cwd_escape", "workspace_id", params.WorkspaceID, "cwd", params.CWD, "user", client.UserID())
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidPath)))
		return
	}

	sess := &store.TerminalSession{
		UserID:      client.UserID(),
		WorkspaceID: ws.ID,
		CWD:         cwd,
		Shell:       params.Shell,
		Status:      store.TerminalStatusRunning,
	}
	if tid := tenantString(client); tid != "" {
		sess.TenantID = &tid
	}
	if err := m.tstore.CreateSession(ctx, sess); err != nil {
		slog.Warn("terminal.create_store_failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "create terminal")))
		return
	}

	if m.countRunning(client) >= maxSessionsPerUser {
		_ = m.tstore.UpdateStatus(ctx, sess.ID, store.TerminalStatusClosed, nil)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "too many open terminals"))
		return
	}

	if _, err := m.mgr.start(sess.ID, client.UserID(), ws.ID, root, params.CWD, params.Shell, params.Cols, params.Rows, func(event string, payload map[string]any) {
		m.emit(event, payload, client.TenantID())
	}); err != nil {
		_ = m.tstore.UpdateStatus(ctx, sess.ID, store.TerminalStatusClosed, nil)
		slog.Warn("terminal.start_failed", "session_id", sess.ID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "start terminal")))
		return
	}
	resp := map[string]any{
		"terminal": map[string]any{
			"id":          sess.ID,
			"workspaceId": sess.WorkspaceID,
			"cwd":         cwd,
			"shell":       sess.Shell,
			"status":      sess.Status,
		},
		"replay": "",
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, resp))
}

// countRunning returns live PTYs owned by this user.
func (m *TerminalMethods) countRunning(client *gateway.Client) int {
	return m.mgr.countRunning(client.UserID())
}

// handleList lists terminal sessions (terminal.list). Params:
// { workspaceId? }. Sessions belong to the calling user within their tenant.
func (m *TerminalMethods) handleList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		WorkspaceID string `json:"workspaceId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	var tid *string
	if s := tenantString(client); s != "" {
		tid = &s
	}
	sessions, err := m.tstore.ListSessions(ctx, tid, client.UserID(), params.WorkspaceID)
	if err != nil {
		slog.Warn("terminal.list_failed", "user_id", client.UserID(), "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "list terminals")))
		return
	}
	out := make([]terminalJSON, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, toTerminalJSON(s))
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"sessions": out}))
}

// fetchOwnedTerminal loads one session row and enforces ownership (owner or
// admin); mismatches answer not-found to avoid leaking existence. A response
// was sent unless ok=true.
func (m *TerminalMethods) fetchOwnedTerminal(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, id string) (*store.TerminalSession, bool) {
	locale := store.LocaleFromContext(ctx)
	notFound := func() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "terminal session", id)))
	}
	row, err := m.tstore.GetSession(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || row == nil {
		notFound()
		return nil, false
	}
	if err != nil {
		slog.Warn("terminal.get_failed", "session_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "load terminal")))
		return nil, false
	}
	if row.UserID != client.UserID() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		notFound()
		return nil, false
	}
	return row, true
}

// handleAttach reconnects to a terminal (terminal.attach). Live PTY → ring
// replay {live:true, data:<b64>, cols, rows}; dead/gone → {live:false} so
// the client shows the ended state.
func (m *TerminalMethods) handleAttach(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	id, ok := m.parseTerminalID(ctx, client, req)
	if !ok {
		return
	}
	row, ok := m.fetchOwnedTerminal(ctx, client, req, id)
	if !ok {
		return
	}
	if sess, live := m.mgr.get(id); live && !sess.closed.Load() {
		data, cols, rows := sess.snapshot()
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
			"live": true,
			"data": base64.StdEncoding.EncodeToString(data),
			"cols": cols,
			"rows": rows,
		}))
		return
	}
	_ = row
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"live": false}))
}

// parseTerminalID decodes and validates { terminalId }; ok=false means the
// error response was already sent.
func (m *TerminalMethods) parseTerminalID(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) (string, bool) {
	locale := store.LocaleFromContext(ctx)
	var params struct {
		TerminalID string `json:"terminalId"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return "", false
		}
	}
	if params.TerminalID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "terminalId")))
		return "", false
	}
	return params.TerminalID, true
}

// handleInput forwards keystrokes (terminal.input). Params: { terminalId,
// data(b64) ≤64KiB decoded }. Ownership enforced against the LIVE session —
// a dead or foreign id answers not-found.
func (m *TerminalMethods) handleInput(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		TerminalID string `json:"terminalId"`
		Data       string `json:"data"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.TerminalID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "terminalId")))
		return
	}
	raw, err := base64.StdEncoding.DecodeString(params.Data)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "data must be base64")))
		return
	}
	if len(raw) > ringCapacity {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidRequest, "input too large")))
		return
	}
	sess, live := m.mgr.get(params.TerminalID)
	if !live || sess.closed.Load() {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "terminal session", params.TerminalID)))
		return
	}
	if sess.userID != client.UserID() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "terminal session", params.TerminalID)))
		return
	}
	if err := sess.write(raw); err != nil {
		slog.Debug("terminal.input_write_failed", "session_id", params.TerminalID, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "terminal session", params.TerminalID)))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{}))
}

// handleResize resizes the PTY window (terminal.resize). Params:
// { terminalId, cols, rows }, clamped by the manager.
func (m *TerminalMethods) handleResize(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	locale := store.LocaleFromContext(ctx)
	var params struct {
		TerminalID string `json:"terminalId"`
		Cols       int    `json:"cols"`
		Rows       int    `json:"rows"`
	}
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgInvalidJSON)))
			return
		}
	}
	if params.TerminalID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, i18n.T(locale, i18n.MsgRequired, "terminalId")))
		return
	}
	cols := clampInt(params.Cols, minTermCols, maxTermCols)
	rows := clampInt(params.Rows, minTermRows, maxTermRows)
	sess, live := m.mgr.get(params.TerminalID)
	if !live || sess.closed.Load() {
		// Resizing an ended tab is harmless — acknowledge without error so
		// debounced resize calls after exit do not spam failures.
		client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{}))
		return
	}
	// Same owner-or-admin rule as input: a live PTY belongs to its creator.
	if sess.userID != client.UserID() && !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrNotFound, i18n.T(locale, i18n.MsgNotFound, "terminal session", params.TerminalID)))
		return
	}
	if err := sess.resize(uint16(cols), uint16(rows)); err != nil {
		slog.Debug("terminal.resize_failed", "session_id", params.TerminalID, "error", err)
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{}))
}

// handleClose kills the PTY and marks the row closed (terminal.close). The
// status update runs regardless of whether the PTY was still live so a
// stale tab can always be cleaned up.
func (m *TerminalMethods) handleClose(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	if !m.guard(ctx, client, req) {
		return
	}
	locale := store.LocaleFromContext(ctx)
	id, ok := m.parseTerminalID(ctx, client, req)
	if !ok {
		return
	}
	row, ok := m.fetchOwnedTerminal(ctx, client, req, id)
	if !ok {
		return
	}
	if sess, live := m.mgr.get(id); live {
		sess.close()
		m.mgr.remove(id)
	}
	if err := m.tstore.UpdateStatus(ctx, id, store.TerminalStatusClosed, nil); err != nil {
		slog.Warn("terminal.close_update_failed", "session_id", id, "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, i18n.T(locale, i18n.MsgInternalError, "close terminal")))
		return
	}
	_ = row
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{"closed": true}))
}
