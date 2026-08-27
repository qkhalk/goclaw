package skills

import (
	"testing"
)

func TestSkillStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status SkillStatus
		want   bool
	}{
		{SkillStatusPending, false},
		{SkillStatusRunning, false},
		{SkillStatusCompleted, true},
		{SkillStatusFailed, true},
		{SkillStatusSkipped, true},
		{SkillStatusBlocked, false},
		{SkillStatusCancelled, true},
	}
	for _, tt := range tests {
		if got := tt.status.IsTerminal(); got != tt.want {
			t.Errorf("SkillStatus(%q).IsTerminal() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestToolPolicy_IsToolAllowed(t *testing.T) {
	tests := []struct {
		name   string
		policy ToolPolicy
		tool   string
		want   bool
	}{
		{
			name:   "empty policy allows all",
			policy: ToolPolicy{},
			tool:   "write_file",
			want:   true,
		},
		{
			name:   "denylist blocks tool",
			policy: ToolPolicy{Denied: []string{"shell"}},
			tool:   "shell",
			want:   false,
		},
		{
			name:   "denylist allows other tools",
			policy: ToolPolicy{Denied: []string{"shell"}},
			tool:   "write_file",
			want:   true,
		},
		{
			name:   "allowlist permits listed tool",
			policy: ToolPolicy{Allowed: []string{"write_file", "read_file"}},
			tool:   "write_file",
			want:   true,
		},
		{
			name:   "allowlist blocks unlisted tool",
			policy: ToolPolicy{Allowed: []string{"write_file"}},
			tool:   "shell",
			want:   false,
		},
		{
			name:   "denylist takes precedence over allowlist",
			policy: ToolPolicy{Allowed: []string{"shell", "write_file"}, Denied: []string{"shell"}},
			tool:   "shell",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.policy.IsToolAllowed(tt.tool); got != tt.want {
				t.Errorf("ToolPolicy.IsToolAllowed(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestToolPolicy_NeedsApproval(t *testing.T) {
	policy := ToolPolicy{
		RequireApproval: []string{"shell", "write_file"},
	}
	if !policy.NeedsApproval("shell") {
		t.Error("shell should need approval")
	}
	if !policy.NeedsApproval("write_file") {
		t.Error("write_file should need approval")
	}
	if policy.NeedsApproval("read_file") {
		t.Error("read_file should not need approval")
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    *SkillSpec
		wantErr bool
	}{
		{
			name:    "valid spec",
			spec:    &SkillSpec{Name: "test", Slug: "test"},
			wantErr: false,
		},
		{
			name:    "missing name",
			spec:    &SkillSpec{Slug: "test"},
			wantErr: true,
		},
		{
			name:    "missing slug",
			spec:    &SkillSpec{Name: "test"},
			wantErr: true,
		},
		{
			name:    "negative retries",
			spec:    &SkillSpec{Name: "test", Slug: "test", MaxRetries: -1},
			wantErr: true,
		},
		{
			name:    "negative cost",
			spec:    &SkillSpec{Name: "test", Slug: "test", MaxCost: -1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSpec() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSkillSpec_FromInfo(t *testing.T) {
	info := &Info{
		Name:         "test-skill",
		Slug:         "test-skill",
		Description:  "A test skill",
		Version:      "1.0.0",
		BaseDir:      "/tmp/test",
		Inputs:       []string{"issue"},
		Outputs:      []string{"patch"},
		AllowedTools: []string{"write_file"},
		QualityGates: []string{"tests_pass"},
	}
	raw := "---\nname: test-skill\ndescription: A test skill\nversion: 1.0.0\n---\n# Test Skill\nDo stuff."

	spec := specFromInfo(info, raw)

	if spec.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", spec.Name, "test-skill")
	}
	if spec.Slug != "test-skill" {
		t.Errorf("Slug = %q, want %q", spec.Slug, "test-skill")
	}
	if spec.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", spec.Version, "1.0.0")
	}
	if len(spec.Inputs) != 1 || spec.Inputs[0] != "issue" {
		t.Errorf("Inputs = %v, want [issue]", spec.Inputs)
	}
	if len(spec.Outputs) != 1 || spec.Outputs[0] != "patch" {
		t.Errorf("Outputs = %v, want [patch]", spec.Outputs)
	}
	if spec.Content != "# Test Skill\nDo stuff." {
		t.Errorf("Content = %q, want %q", spec.Content, "# Test Skill\nDo stuff.")
	}
}
