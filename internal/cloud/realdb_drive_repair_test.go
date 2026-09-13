package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/store/pg"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// TestRealDBDriveRepair is a manually-gated maintenance probe: with
// DRIVE_REPAIR_ACCOUNT set it re-resolves the default drive for one account
// (personal preferred — guest business drives are unreachable for a consumer
// token) and rewrites the account's settings in place.
func TestRealDBDriveRepair(t *testing.T) {
	dsn := os.Getenv("RESOLVER_PROBE_DSN")
	accountID := os.Getenv("DRIVE_REPAIR_ACCOUNT")
	if dsn == "" || accountID == "" {
		t.Skip("RESOLVER_PROBE_DSN + DRIVE_REPAIR_ACCOUNT not set — manual repair only")
	}
	encKey := os.Getenv("GOCLAW_ENCRYPTION_KEY")

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()
	accounts := pg.NewPGCloudAccountStore(db, encKey)

	// The account row is owner-scoped; load via the shared listing from the
	// owner's scope (user id stored on the row).
	var tenantID, ownerID string
	if err := db.QueryRow(`SELECT tenant_id, user_id FROM cloud_accounts WHERE id=$1`, accountID).
		Scan(&tenantID, &ownerID); err != nil {
		t.Fatalf("load account row: %v", err)
	}
	tid, err := uuid.Parse(tenantID)
	if err != nil {
		t.Fatalf("parse tenant: %v", err)
	}
	ctx := store.WithUserID(store.WithTenantID(context.Background(), tid), ownerID)

	acct, err := accounts.Get(ctx, accountID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	m := NewManager(CloudProviderConfig{}, accounts, encKey)
	m.SetBindingStore(accounts)
	ts, err := m.TokenSource(ctx, acct.ID)
	if err != nil {
		t.Fatalf("TokenSource: %v", err)
	}
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	// List every drive first so the repair log shows the full picture.
	all, err := fetchMicrosoftGraph(ctx, MicrosoftGraphDrivesURL, tok.AccessToken)
	if err != nil {
		t.Fatalf("drives list: %v", err)
	}
	var enumerated struct {
		Value []MicrosoftDrive `json:"value"`
	}
	_ = json.Unmarshal(all, &enumerated)
	for i, d := range enumerated.Value {
		prefix := d.ID
		if len(prefix) > 12 {
			prefix = prefix[:12] + "…"
		}
		t.Logf("drive[%d]: id=%s type=%s", i, prefix, d.DriveType)
	}

	drive, err := fetchMicrosoftDefaultDrive(ctx, tok.AccessToken)
	if err != nil {
		t.Fatalf("drives: %v", err)
	}
	if override := os.Getenv("DRIVE_ID_OVERRIDE"); override != "" {
		drive = &MicrosoftDrive{ID: override, DriveType: "personal"}
		t.Log("drive overridden via DRIVE_ID_OVERRIDE")
	}
	t.Logf("default drive: id=%s type=%s", drive.ID[:8]+"…", drive.DriveType)

	var settings map[string]string
	_ = json.Unmarshal([]byte(acct.Settings), &settings)
	if settings == nil {
		settings = map[string]string{}
	}
	settings["drive_id"] = drive.ID
	settings["drive_type"] = drive.DriveType
	out, _ := json.Marshal(settings)
	if _, err := db.Exec(`UPDATE cloud_accounts SET settings=$1, updated_at=NOW() WHERE id=$2`, string(out), accountID); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	t.Log("settings updated")
	if os.Getenv("DRIVE_LIST_FULL") != "" {
		for i, d := range enumerated.Value {
			t.Logf("FULL[%d]: %s", i, d.ID)
		}
	}
}
