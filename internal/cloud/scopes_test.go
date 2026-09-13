package cloud

import (
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

func TestHasGoogleWriteScopes(t *testing.T) {
	// Full scope, JSON array shape → write.
	if !HasGoogleWriteScopes(`["openid","https://www.googleapis.com/auth/drive"]`) {
		t.Fatal("array with full drive scope must qualify as write")
	}
	// Full scope, space-separated JSON string shape (Extra("scope") marshal).
	if !HasGoogleWriteScopes(`"openid https://www.googleapis.com/auth/drive"`) {
		t.Fatal("string shape with full drive scope must qualify as write")
	}
	// Read-only legacy grant must NOT qualify (exact match, never substring).
	if HasGoogleWriteScopes(`["https://www.googleapis.com/auth/drive.readonly"]`) {
		t.Fatal("drive.readonly must not qualify as write (array shape)")
	}
	if HasGoogleWriteScopes(`"https://www.googleapis.com/auth/drive.readonly"`) {
		t.Fatal("drive.readonly must not qualify as write (string shape)")
	}
	// drive.file (per-file scope) is not the full write scope either.
	if HasGoogleWriteScopes(`["https://www.googleapis.com/auth/drive.file"]`) {
		t.Fatal("drive.file must not qualify as write")
	}
	// Empty / garbage.
	if HasGoogleWriteScopes("") || HasGoogleWriteScopes("[]") || HasGoogleWriteScopes("not json") {
		t.Fatal("empty or unparseable scopes must not qualify as write")
	}
}

func TestHasMicrosoftWriteScopes(t *testing.T) {
	if !HasMicrosoftWriteScopes(`["offline_access","User.Read","Files.ReadWrite.All"]`) {
		t.Fatal("Files.ReadWrite.All must qualify as write")
	}
	if !HasMicrosoftWriteScopes(`"offline_access User.Read Files.ReadWrite.All"`) {
		t.Fatal("string shape Files.ReadWrite.All must qualify as write")
	}
	if HasMicrosoftWriteScopes(`["offline_access","User.Read","Files.Read.All"]`) {
		t.Fatal("legacy Files.Read.All must not qualify as write")
	}
	if HasMicrosoftWriteScopes("") || HasMicrosoftWriteScopes("not json") {
		t.Fatal("empty or unparseable scopes must not qualify as write")
	}
}

func TestAccountCanWrite(t *testing.T) {
	writeGoogle := &store.CloudAccount{Provider: GoogleProvider, Scopes: `["` + GoogleDriveWriteScope + `"]`}
	readonlyGoogle := &store.CloudAccount{Provider: GoogleProvider, Scopes: `["https://www.googleapis.com/auth/drive.readonly"]`}
	writeOneDrive := &store.CloudAccount{Provider: MicrosoftProvider, Scopes: `["Files.ReadWrite.All"]`}
	readonlyOneDrive := &store.CloudAccount{Provider: MicrosoftProvider, Scopes: `["Files.Read.All"]`}

	if !AccountCanWrite(writeGoogle) || AccountCanWrite(readonlyGoogle) {
		t.Fatal("google can_write mismatch")
	}
	if !AccountCanWrite(writeOneDrive) || AccountCanWrite(readonlyOneDrive) {
		t.Fatal("onedrive can_write mismatch")
	}
	if AccountCanWrite(nil) {
		t.Fatal("nil account must not be writable")
	}
	if AccountCanWrite(&store.CloudAccount{Provider: "dropbox", Scopes: `["everything"]`}) {
		t.Fatal("unsupported provider must not be writable")
	}
}

func TestAccountMicrosoftScopesPrefersStoredGrant(t *testing.T) {
	acct := &store.CloudAccount{Scopes: `"offline_access User.Read Files.Read.All"`}
	got := strings.Join(accountMicrosoftScopes(acct), ",")
	if got != "offline_access,User.Read,Files.Read.All" {
		t.Fatalf("accountMicrosoftScopes = %q, want the original grant", got)
	}
	// Legacy row with no stored scopes falls back to the current constants.
	empty := &store.CloudAccount{Scopes: ""}
	if got := accountMicrosoftScopes(empty); len(got) != len(MicrosoftScopes) {
		t.Fatalf("fallback = %v, want MicrosoftScopes", got)
	}
}
