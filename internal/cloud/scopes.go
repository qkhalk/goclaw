package cloud

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// ErrCloudWriteScopeRequired is returned by write endpoints (file ops) when
// the target account was connected with read-only scopes. The UI surfaces it
// with the "re-grant access" affordance on the Clouds page.
var ErrCloudWriteScopeRequired = errors.New("cloud: account has read-only scopes — re-grant access on the Clouds page to enable file operations")

// GoogleDriveWriteScope is the full Google Drive OAuth scope (superset of
// drive.readonly). Matched as an EXACT string — a substring check on "drive"
// would wrongly credit the read-only scope too.
const GoogleDriveWriteScope = "https://www.googleapis.com/auth/drive"

// MicrosoftFilesWriteScope is the OneDrive read/write Graph scope (superset
// of Files.Read.All).
const MicrosoftFilesWriteScope = "Files.ReadWrite.All"

// scopesFromJSON parses the stored account scopes into individual scope
// strings. Rows persist two shapes depending on what the token endpoint
// returned (see Manager.handleGoogleCallback): a JSON array of strings, or a
// JSON-quoted string of space-separated scopes. Plain space/comma-separated
// input is accepted as a last resort. Returns nil when nothing parses.
func scopesFromJSON(scopesJSON string) []string {
	scopesJSON = strings.TrimSpace(scopesJSON)
	if scopesJSON == "" {
		return nil
	}
	switch scopesJSON[0] {
	case '[':
		var arr []string
		if err := json.Unmarshal([]byte(scopesJSON), &arr); err == nil {
			return arr
		}
	case '"':
		var s string
		if err := json.Unmarshal([]byte(scopesJSON), &s); err == nil {
			return splitScopes(s)
		}
	}
	return splitScopes(scopesJSON)
}

func splitScopes(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// HasGoogleWriteScopes reports whether the stored scope set contains the full
// Drive scope. Exact-string match only: drive.readonly must NOT qualify.
func HasGoogleWriteScopes(scopesJSON string) bool {
	return slices.Contains(scopesFromJSON(scopesJSON), GoogleDriveWriteScope)
}

// HasMicrosoftWriteScopes reports whether the stored scope set contains
// Files.ReadWrite.All.
func HasMicrosoftWriteScopes(scopesJSON string) bool {
	return slices.Contains(scopesFromJSON(scopesJSON), MicrosoftFilesWriteScope)
}

// AccountCanWrite reports whether the account's stored OAuth grant includes
// the provider's write scope. Accounts connected before the write upgrade
// report false until their owner re-grants; every read path stays usable.
func AccountCanWrite(acct *store.CloudAccount) bool {
	if acct == nil {
		return false
	}
	switch acct.Provider {
	case GoogleProvider:
		return HasGoogleWriteScopes(acct.Scopes)
	case MicrosoftProvider:
		return HasMicrosoftWriteScopes(acct.Scopes)
	default:
		return false
	}
}

// accountMicrosoftScopes returns the OneDrive scopes to pin on the rclone
// remote: the account's ORIGINAL grant when known (MSA refresh must repeat
// the granted scopes or Microsoft returns a compact token Graph rejects),
// falling back to the current MicrosoftScopes for legacy rows with no usable
// stored grant.
func accountMicrosoftScopes(acct *store.CloudAccount) []string {
	if scopes := scopesFromJSON(acct.Scopes); len(scopes) > 0 {
		return scopes
	}
	return MicrosoftScopes
}
