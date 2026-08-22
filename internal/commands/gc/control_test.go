package gc

import (
	"context"
	"strings"
	"testing"
)

func TestControlPlaneKindsParse(t *testing.T) {
	for _, kind := range []CommandKind{KindStatus, KindRuns, KindDoctor, KindApprove} {
		cmd, ok := Parse("/gc:" + string(kind))
		if !ok {
			t.Errorf("Parse(/gc:%s) not recognized", kind)
			continue
		}
		if !cmd.Kind.IsControlPlane() {
			t.Errorf("kind %s should be control-plane", kind)
		}
		if cmd.Kind.Valid() == false {
			t.Errorf("kind %s should be valid", kind)
		}
	}
	// Skill kinds are NOT control-plane.
	cmd, ok := Parse("/gc:plan --deep")
	if !ok || cmd.Kind != KindPlan {
		t.Fatalf("skill kind regression")
	}
	if cmd.Kind.IsControlPlane() {
		t.Error("plan must not be control-plane")
	}
}

func TestResolveControlAnswersCanned(t *testing.T) {
	e := NewExecutor(nil, nil)
	cases := map[CommandKind]string{
		KindStatus:  "/gc:status",
		KindRuns:    "/gc:runs",
		KindDoctor:  "/gc:doctor",
		KindApprove: "/gc:approve",
	}
	for want, msg := range cases {
		reply, handled := e.ResolveControl(context.Background(), msg)
		if !handled {
			t.Errorf("%s: ResolveControl did not handle", want)
			continue
		}
		if reply.Text == "" || !strings.Contains(reply.Text, "## /gc:"+string(want)) {
			t.Errorf("%s: canned text missing header, got %.60q", want, reply.Text)
		}
	}
	// Skill command + non-command passthrough.
	if _, handled := e.ResolveControl(context.Background(), "/gc:plan x"); handled {
		t.Error("skill command must not be control-handled")
	}
	if _, handled := e.ResolveControl(context.Background(), "hello world"); handled {
		t.Error("plain message must not be control-handled")
	}
}

func TestStatusSnapshotInjected(t *testing.T) {
	e := NewExecutor(nil, nil)
	e.SetStatusSnapshot(func() string { return "lanes: main=2/8 cron=0/2" })
	reply, _ := e.ResolveControl(context.Background(), "/gc:status")
	if !strings.Contains(reply.Text, "lanes: main=2/8") {
		t.Errorf("snapshot not rendered: %.80q", reply.Text)
	}
}
