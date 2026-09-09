package cmd

import (
	"log/slog"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/edition"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/gateway/methods"
	"github.com/nextlevelbuilder/goclaw/internal/nodes"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// wireNodeRuntime wires the Phase-2 compute-node registry surface:
//   - the nodes.* WS methods (register/list/revoke/result) backed by the
//     node store and the shared in-memory connection registry;
//   - the node_exec agent tool, full (PG) edition only — same gating rule as
//     workstation_exec (Lite has no multi-tenant node registry).
//
// Kept out of registerAllMethods so gateway.go carries a single wiring line
// (this phase rebases over Phase 1/3 in that file).
func wireNodeRuntime(pgStores *store.Stores, toolsReg *tools.Registry, server *gateway.Server, msgBus *bus.MessageBus) {
	if pgStores == nil || pgStores.Nodes == nil {
		slog.Warn("node runtime skipped: node store not initialised")
		return
	}

	// Shared live-connection registry: methods track daemons here, the tool
	// routes invokes through it.
	nodeRegistry := nodes.NewRegistry()
	methods.NewNodesMethods(pgStores.Nodes, nodeRegistry, msgBus).Register(server.Router())
	slog.Info("nodes.* RPC methods registered", "methods", []string{"nodes.register", "nodes.list", "nodes.revoke", "nodes.result"})

	if edition.Current().Name != "standard" {
		return
	}
	toolsReg.Register(tools.NewNodeExecTool(pgStores.Nodes, nodeRegistry, msgBus))
	slog.Info("node_exec tool registered (Standard edition)")
}
