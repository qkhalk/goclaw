package cloud

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestNormalizeAgentAccess(t *testing.T) {
	cases := map[string]AgentAccess{
		"none": AgentAccessNone, "read": AgentAccessRead,
		"write": AgentAccessWrite, "full": AgentAccessFull,
		" full ": AgentAccessFull, "": AgentAccessRead, // empty → legacy default
	}
	for in, want := range cases {
		got, err := NormalizeAgentAccess(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeAgentAccess(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := NormalizeAgentAccess("admin"); err == nil {
		t.Fatal("unknown level must be rejected")
	}
}

func TestAgentAccessLadder(t *testing.T) {
	if !AgentAccessFull.allows(AgentAccessWrite) || !AgentAccessWrite.allows(AgentAccessRead) {
		t.Fatal("higher levels must allow lower floors")
	}
	if AgentAccessRead.allows(AgentAccessWrite) || AgentAccessNone.allows(AgentAccessRead) {
		t.Fatal("lower levels must not allow higher floors")
	}
}

func TestAgentAccessOfDefaultsLegacyRows(t *testing.T) {
	if got := AgentAccessOf(&store.CloudAccount{}); got != AgentAccessRead {
		t.Fatalf("legacy row = %v, want read", got)
	}
	if got := AgentAccessOf(&store.CloudAccount{AgentAccess: "none"}); got != AgentAccessNone {
		t.Fatalf("none row = %v", got)
	}
	if got := AgentAccessOf(nil); got != AgentAccessNone {
		t.Fatalf("nil account = %v, want none", got)
	}
}
