package config

import "encoding/json"

// FilePolicy holds per-agent file/cloud capability toggles stored in
// agents.other_config under the "file_policy" key:
//
//	other_config = { "file_policy": { "read": true, "write": true, "create": true } }
//
// Every capability defaults to true (backwards compatible): agents created
// before this field existed keep full file access. Missing or malformed
// config also means "allow all". Cloud read tools (cloud_ls, cloud_read,
// cloud_fetch, cloud_about) are gated under Read by the tool-policy layer.
type FilePolicy struct {
	Read   *bool `json:"read,omitempty"`
	Write  *bool `json:"write,omitempty"`
	Create *bool `json:"create,omitempty"`
}

// DefaultFilePolicy returns the allow-all policy used when an agent has no
// file_policy configured (or when the enforcement layer has no agent context).
func DefaultFilePolicy() FilePolicy {
	return FilePolicy{}
}

// ParseFilePolicy decodes a file_policy JSON object, returning the zero
// FilePolicy (all pointers nil = allow all) on missing/malformed input.
func ParseFilePolicy(raw json.RawMessage) FilePolicy {
	if len(raw) <= 2 {
		return FilePolicy{}
	}
	var p FilePolicy
	if json.Unmarshal(raw, &p) != nil {
		return FilePolicy{}
	}
	return p
}

// can reports whether the given capability pointer is enabled. A nil pointer
// defaults to true so unconfigured agents keep existing behavior.
func can(v *bool) bool { return v == nil || *v }

// CanRead reports whether the agent may read/list files and cloud drives.
func (p FilePolicy) CanRead() bool { return can(p.Read) }

// CanWrite reports whether the agent may modify existing file content.
func (p FilePolicy) CanWrite() bool { return can(p.Write) }

// CanCreate reports whether the agent may create new files or folders.
func (p FilePolicy) CanCreate() bool { return can(p.Create) }
