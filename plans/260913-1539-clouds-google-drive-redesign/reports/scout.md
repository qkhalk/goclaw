# GoClaw Clouds Page — Scout Report

Repo: `C:\Users\DORA\Downloads\goclaw-mod` (Go backend, React 19/Vite/Tailwind/Radix UI at `ui/web`, pnpm). All paths absolute, cited as `path:line`.

---

## 1. Current Cloud web UI structure

Directory `ui/web/src/pages/cloud/` contains exactly 4 files: `cloud-page.tsx`, `account-detail.tsx`, `scope-bindings-panel.tsx`, `hooks/use-cloud.ts`. There is NO separate provider-setup-guide component file — the setup guide (`ProviderClientSetup`) is inline in cloud-page.tsx.

### 1.1 `ui/web/src/pages/cloud/cloud-page.tsx` (535 lines)

- `useCloudResult()` (lines 42–57): one-shot `?connected=`/`&error=` query-param banner, strips params via `setParams({}, {replace:true})`.
- `statusBadge()` helper (59–69): maps account status → StatusBadge (active=success, expired/revoked=warning, else error).
- `LIVE_PROVIDERS` (73–76): `[{id:"google", name:"Google Drive", icon:Cloud}, {id:"onedrive", name:"Microsoft OneDrive", icon:HardDrive}]`; `COMING_SOON_PROVIDERS = ["Dropbox", "Amazon S3 / compatible"]` (line 77).
- `ProviderClientSetup` (82–181): admin BYO OAuth client card. Uses `useAuthStore` role check (84–85, admin/owner only), `useCloudSettings(provider, isAdmin)` (86), local `clientID`/`secret`/`saving`/`error`/`editing` state (87–91), copy-redirect-URI via `useClipboard` (92). Collapses to a pencil "update" button after `settings.secret_set` (98–107). `handleSave` (109–121) calls `saveSettings(clientID, secret)`.
- `CloudPage` (183–535) — state (191–200):
  - `selectedProvider: CloudProvider | null` (191) — drives the two-level "picker → provider view" navigation.
  - `connecting` (192), `deleteTarget: CloudAccount | null` (193), `deleteLoading` (194), `detailTab: {id: string, tab: DetailTab}` (195) — which account's detail panel is open, `pasteProvider`/`pasteURL`/`completing`/`pasteError` (196–199), `showByoSetup` (200).
- Layout: `max-w-4xl mx-auto flex-col gap-6` (271), `PageHeader` with Refresh action (272–281).
- Dashboard stats: 3-card grid `grid-cols-1 sm:grid-cols-3` (298–328) — accounts count, active count, providers-ready count with per-provider Badge.
- Provider picker: rendered when `!selectedProvider` (332–388) — clickable cards set `setSelectedProvider(p.id)` (347), plus coming-soon cards (373–385).
- Back button (393–396): `setSelectedProvider(null)`.
- Paste-back panel (400–419): amber-bordered panel with Input + "Finish" button; only when `pasteProvider === selectedProvider`.
- `ScopeBindingsPanel` mount (422): `{isAdmin && <ScopeBindingsPanel provider={selectedProvider} />}`.
- Connect flow (212–232): `handleConnect` calls `startConnect(provider)`; if `res.mode === "paste"` opens `auth_url` in new tab and shows the paste panel (221–224); else `window.location.href = res.auth_url` (225). `handleComplete` (234–247) calls `completeConnect(pasteProvider, pasteURL)`.
- `accountCanMail` (249–257): google-only; parses `a.scopes` JSON, true if any scope includes `/auth/gmail`.
- Account cards (446–500): `grid grid-cols-1 gap-3 sm:grid-cols-2`; each card renders `AccountDetail` (461–467) with controlled `tab`/`onTabChange` from `detailTab` state; admin-only share `Switch` (468–476) calls `setShared(account.id, v)`; Disconnect button (479–487) opens `ConfirmDialog` (524–532) → `disconnect(deleteTarget.id)`.
- BYO advanced collapsible (504–518): chevron toggle `showByoSetup` → `ProviderClientSetup`.
- Mobile compliance on this page: inputs use `text-base md:text-sm` (156, 164, 409, 411), buttons use `min-h-11 sm:min-h-9` (175, 412, 432, 493), responsive grids (298, 339, 446). GOOD.

### 1.2 `ui/web/src/pages/cloud/account-detail.tsx` (309 lines)

- Types: `AccountAbout {total, used, free}` (19–23), `FileEntry {name, is_dir, size, mod_time}` (26–31), `MailSummary {id, from, subject, date, snippet}` (34–40) — all **snake_case**, matching HTTP JSON.
- **Local `formatBytes()` defined at lines 42–48** (duplicates `formatFileSize` in `lib/format.ts:97` and `formatSize` in `lib/file-helpers.ts:151` — 3 copies exist).
- `export type DetailTab = "files" | "mail" | null` (line 50).
- `AccountDetail` component (54–139): props `{accountId, provider, canMail, tab, onTabChange}` (60–66). Query: `useQuery({queryKey:["cloud","about",accountId], staleTime:60_000, queryFn: http.get<AccountAbout>(`/v1/cloud/accounts/${accountId}/about`)})` (70–74). Quota bar (110–127): custom div-based progress (`h-2 rounded-full`, color red>90%/amber>75%/emerald) — note there IS a `ui/progress.tsx` component it does not use. Tab buttons (85–104): toggle-style buttons (not Radix Tabs) calling `onTabChange`.
- `FilesBrowser` (142–257): `const [path, setPath] = useState("/")` (145). Query `["cloud","files",accountId,path]` → `GET /v1/cloud/accounts/${accountId}/files?path=...&limit=200` (147–154). Breadcrumb (156–163): `path.split("/").filter(Boolean).map(...)` producing `{name: decodeURIComponent(seg), path: "/" + arr.slice(0,i+1).join("/")}`; navigating into a folder: `setPath((path === "/" ? "" : path) + "/" + encodeURIComponent(e.name))` (221–223). Client-side sort dirs-first (165–169). Renders a plain `<table>` wrapped in `overflow-x-auto` with `min-w-[540px]` (196, 207) — note AGENTS.md wants `min-w-[600px]` (AGENTS.md:270), 540 is slightly under. Rows: folder rows clickable, file rows static. Read-only footer note (253–255).
- `MailboxPreview` (261–309): query `["cloud","mail",accountId]` → `GET /v1/cloud/accounts/${accountId}/mail?max=10` (265–272); renders `<ul>` of subject/from/snippet.

### 1.3 `ui/web/src/pages/cloud/scope-bindings-panel.tsx` (241 lines)

- `const NONE = "__none__"` sentinel (line 22) — comment: "Radix Select forbids empty values". Used for the tenant-default AccountSelect: value `tenantBinding?.account_id || NONE` (182), option list prepends `{id: NONE, label: t("scope.none")}` (184); `handleSetTenant` (116–127) translates `NONE` → `deleteBinding(tenantBinding.id)`, anything else → `upsertBinding({scope_type:"tenant", scope_key:"", provider, account_id})`.
- `useGroupCandidates()` (26–41): derives group-chat candidates by parsing session keys `agent:<agent>:<channel>:group:<chatId>[:topic:<n>]` via `useSessions({limit:200})`; returns `[{id: chatId, label: "channel · chatId"}]`.
- `useUserCandidates()` (44–54): distinct `s.userID` values from the same 200 sessions.
- `AccountSelect` (56–83): thin Radix `Select` wrapper; `<SelectContent className="max-h-64">`.
- `ScopeBindingsPanel({provider})` (88–241):
  - Data: `useCloudAccounts()` (90), `useCloudBindings(true)` (91).
  - Filters bindings per provider; splits into `tenantBinding` / `groupBindings` / `userBindings` (107–110). Account labels use `🏢 ` prefix for shared accounts (102–105).
  - Add-row state (95–99): `addScope` (default `"group"`), `addKey`, `addAccount`, `error`, `saving`.
  - Scope select options: **only `group` and `user`** in the add-row Select (204–207); `tenant` is handled exclusively by the dedicated default row (176–187). Backend supports all three (`internal/http/cloud.go:293-302`).
  - Add key Input uses a native `<datalist id="cloud-scope-key-suggestions">` (212–221) for suggestions — not a combobox (note `ui/combobox.tsx` exists).
  - `handleAdd` validation (129–145): requires `addKey.trim()` and `addAccount` non-empty, else `t("scope.missing_fields")`; then `upsertBinding({scope_type: addScope, scope_key: addKey.trim(), provider, account_id: addAccount})`.
  - Binding rows (`renderBindingRows`, 149–168): icon + `scope_key || t("scope.tenant_default")` + `→ emailOf(account_id)` + Trash2 delete button calling `deleteBinding(b.id)` (163).
  - Mobile: grid `grid-cols-1 sm:grid-cols-[130px_1fr_1fr_auto]` (199), input `text-base md:text-sm` (214), button `min-h-11 sm:min-h-9` (228). GOOD.

### 1.4 `ui/web/src/pages/cloud/hooks/use-cloud.ts` (167 lines)

**Transport is pure HTTP** via `useHttp()` (from `@/hooks/use-ws`, which returns an `HttpClient` — `ui/web/src/hooks/use-ws.ts:14-19`). No WS RPC anywhere in cloud. All param/response shapes are **snake_case**.

Exports:
- `interface CloudAccount` (6–18): `{id, provider, email, display_name, scopes (string), token_expires_at?, status: "active"|"expired"|"revoked"|"error", status_message, shared: boolean, created_at}`.
- `type CloudBindingScopeType = "tenant" | "user" | "group"` (20).
- `interface CloudBinding` (23–30): `{id, scope_type, scope_key, provider, account_id, created_by}`.
- `type CloudProvider = "google" | "onedrive"` (32).
- `interface CloudStatus` (34–38): `{enabled, edition, providers: {google?: {configured}, onedrive?: {configured}}}`.
- `interface CloudStartResponse` (40–46): `{auth_url, redirect_uri, mode: "callback" | "paste"}`.
- `useCloudStatus()` (48–55): `useQuery ["cloud","status"]` → `GET /v1/cloud/status`.
- `interface CloudSettings {client_id, secret_set, redirect_uri}` (58–62); `useCloudSettings(provider, enabled)` (64–82): `GET /v1/cloud/settings?provider=${provider}`; `saveSettings(client_id, client_secret)` → `PUT /v1/cloud/settings?provider=...` body `{client_id, client_secret}`, then `invalidateQueries({queryKey:["cloud"]})`.
- `useCloudAccounts()` (84–133): returns `{accounts, loading, refresh, disconnect, startConnect, completeConnect, setShared}`:
  - list: `GET /v1/cloud/accounts` → `{accounts: CloudAccount[]}` (92–96)
  - `disconnect(id)`: `DELETE /v1/cloud/accounts/${id}` (98–104)
  - `startConnect(provider)`: `POST /v1/cloud/oauth/${provider}/start` (106–110)
  - `completeConnect(provider, url)`: `POST /v1/cloud/oauth/${provider}/complete` body `{url}` → `{email}` (114–121)
  - `setShared(id, shared)`: `PUT /v1/cloud/accounts/${id}/shared` body `{shared}` (124–130)
  - invalidation: `invalidateQueries({queryKey:["cloud"]})` (87–90).
- `useCloudBindings(enabled)` (135–167): returns `{bindings, loading, upsertBinding, deleteBinding}`:
  - list: `GET /v1/cloud/bindings` → `{bindings: CloudBinding[]}` (143–148)
  - `upsertBinding(input: {scope_type, scope_key, provider, account_id})`: `PUT /v1/cloud/bindings` (150–156)
  - `deleteBinding(id)`: `DELETE /v1/cloud/bindings/${id}` (158–164)
  - invalidation key: `["cloud","bindings"]`.

State management: **TanStack react-query (v5) + local useState only**. No zustand cloud store (zustand stores live in `ui/web/src/stores/`: use-auth-store, use-chat-messages-store, use-kg-detail-store, use-logs-store, use-team-event-store, use-toast-store, use-ui-store). `HttpClient` (`ui/web/src/api/http-client.ts:6-11`) is a fetch wrapper with auth headers; has `get/post/put/patch/delete`, `upload(path, formData)` (line 78), `fetchBlob` (line 71). Single shared `QueryClient` created in `ui/web/src/components/providers/app-providers.tsx:6`.

---

## 2. Backend cloud surface

### 2.1 `internal/http/cloud.go` (715 lines) — every route

`RegisterRoutes` (57–76):

| Method+Path | Guard | Handler |
|---|---|---|
| `GET /v1/cloud/status` | `requireAuth("")` (any authed user) | handleStatus (80–95): `{enabled, edition, providers:{google:{configured},onedrive:{configured}}}` |
| `GET /v1/cloud/settings` | `requireAuth(RoleAdmin)` + `requireMasterScope` (62, 106) | handleGetSettings (105–129): `{client_id, secret_set, redirect_uri}` — secret never returned |
| `PUT /v1/cloud/settings` | RoleAdmin + master scope (63, 137) | handlePutSettings (136–182): body `{client_id, client_secret}`; empty secret keeps saved; first save requires secret |
| `GET /v1/cloud/accounts` | requireAuth (64) | handleList (214–228): `h.accounts.List(ctx)` — ctx tenant+user scoped → `{accounts}` |
| `DELETE /v1/cloud/accounts/{id}` | requireAuth (65) | handleDelete (232–251): uuid-validated; `ErrCloudAccountNotFound` → 404 |
| `PUT /v1/cloud/accounts/{id}/shared` | requireAuth + **tenantAdmin** (66, 264) | handleSetShared (263–289): body `{shared: *bool}` |
| `GET /v1/cloud/accounts/{id}/about` | requireAuth (67) | handleAccountAbout (432–453): → `{total, used, free}` via `storage.AboutAccount` |
| `GET /v1/cloud/accounts/{id}/files` | requireAuth (68) | handleAccountFiles (456–495): `?path=` (default `/`), `?limit=` 1–1000 default 200 → `{path, entries:[{name,is_dir,size,mod_time}]}` |
| `GET /v1/cloud/accounts/{id}/mail` | requireAuth (69) | handleAccountMail (499–533): `?max=` 1–50 default 10 → `{email, messages}`; requires Gmail-capable (BYO client) else 400 |
| `GET /v1/cloud/bindings` | requireAuth + **requireTenantAdmin** (70, 307) — members cannot even list | handleListBindings (306–324) |
| `PUT /v1/cloud/bindings` | requireAuth + requireTenantAdmin (71, 336) | handleUpsertBinding (335–384): body `{scope_type, scope_key, provider, account_id}`; validates scope (`validBindingScope` 293–302: tenant⇒empty key, user/group⇒non-empty), provider (`IsSupportedProvider`), uuid account_id, account visible via `manager.AccountByID` and provider match |
| `DELETE /v1/cloud/bindings/{id}` | requireAuth + requireTenantAdmin (72, 388) | handleDeleteBinding (387–410) |
| `POST /v1/cloud/oauth/{provider}/start` | requireAuth (73) | handleStart (561–590): → `{auth_url, redirect_uri, mode}` |
| `POST /v1/cloud/oauth/{provider}/complete` | requireAuth (74) | handleComplete (602–634): body `{url}`; `cloudmgr.ParseRedirectedURL` then `HandleCallback` → `{email}` |
| `GET /v1/cloud/oauth/callback` | **unauthenticated** — signed state is the auth primitive (58–60, 638) | handleCallback (638–667): 302 redirect to `/cloud?connected=<email>` or `?error=<code>` (672–678, fixed fragments only — no open redirect) |

Surface gate: `h.available()` (682–688) → 403 when `cloud.enabled=false` kill-switch. Error mapping `storageError` (536–543): `ErrRCloneMissing` → 503; other storage errors → 502.

**No upload / mkdir / rename / delete / move / copy / download endpoint exists for cloud accounts.** Files/about/mail are GET-only, read-only.

Wiring: `cmd/gateway_cloud.go:71` — `server.SetCloudHandler(httpapi.NewCloudHandler(manager, stores.CloudAccounts, stores.Tenants, mailSvc, enabled, cfg.Cloud.RedirectBaseURL))`; not wired at all when `edition.Current().CloudAccountsEnabled` is false (`cmd/gateway_cloud.go:63-65`).

### 2.2 `internal/cloud/storage_service.go` (285 lines) — every exported method

`StorageService{manager, supervisor *storage.Supervisor}` (25–28). `NewStorageService(manager, configDir)` (34–39). Methods:
- `Shutdown()` (42) — stops rcd child.
- `RemoveRemote(ctx, accountID)` (47–57) — best-effort `ConfigDelete` on disconnect.
- `resolveAccount` (62–66) — private; delegates to `manager.ResolveAccount(ctx, name, [google, onedrive], isStorageProvider)`.
- `IsStorageProvider(provider)` exported (80).
- `ensureRemote(ctx, acct) (string, error)` (84–163) — **private, core primitive**: gets/starts rcd, remote name `storage.RemoteName(acct.ID)`, if absent injects token bootstrap (`ConfigCreate` with `drive` or `onedrive` type + `drive_id`/`drive_type` from `acct.Settings` for OneDrive + pinned `client_id`/`client_secret` from `credentialsForAccount`); as of commit 6c081728 also pins onedrive `access_scopes` (comma-separated) and a real token expiry, plus self-heal: `runWithRemote` detects `InvalidAuthentication`/`JWT is not well formed` errors and rebuilds the remote from the DB once.
- `FS(ctx, account) (string, error)` (166–172) — returns remote spec e.g. `goclaw-abcd1234:`.
- `List(ctx, account, path string, max int) ([]storage.ListEntry, error)` (175–185).
- `Stat(ctx, account, path string) (*storage.StatInfo, error)` (188–198).
- `About(ctx, account string) (*storage.AboutInfo, error)` (201–207); `aboutFor` (209–219).
- `AboutAccount(ctx, acct *store.CloudAccount) (*storage.AboutInfo, error)` (223–225) — no re-resolution.
- `ListAccount(ctx, acct, path, max)` (228–238).
- `Fetch(ctx, account, remotePath, workspaceDir string, sizeCapMB int64) (string, error)` (243–278) — stats, enforces size cap, copies into `<workspace>/cloud/<name>` via `rc.OperationsCopyFile`, returns workspace-relative path.

### 2.3 `internal/cloud/storage/rc_client.go` (188 lines) — rc API wrappers

`RCClient{base,user,pass,http}` (17–22), `NewRCClient(base,user,pass)` with 60s timeout (25–32). **The `do()` helper: `func (c *RCClient) do(ctx, path string, params map[string]any, out any) error` (35–65)** — POST JSON to `base/<path>` with basic auth, 16MB response cap, status != 200 → error with truncated body. **Adding a new wrapper is a 3–6 line function** — e.g.:

```go
func (c *RCClient) OperationsMkdir(ctx, fs, remote string) error {
    return c.do(ctx, "operations/mkdir", map[string]any{"fs": fs, "remote": remote}, nil)
}
```

Already wrapped (endpoint → wrapper):
- `core/version` → `CoreVersion` (68–72)
- `config/listremotes` → `ConfigListRemotes` (75–81)
- `config/create` → `ConfigCreate(ctx, name, remoteType, parameters)` (87–94) — note parameters must be **nested** under `"parameters"` key with `opt:{obscure:true}`
- `config/delete` → `ConfigDelete(ctx, name)` (97–99)
- `operations/list` → `OperationsList(ctx, fs, remote, maxEntries)` (120–133) — `opt:{recurse:false, maxDepth:1, limit}`; `ListEntry{Name,IsDir,Size,ModTime}` (102–107) (rclone capital field names)
- `operations/stat` → `OperationsStat(ctx, fs, remote)` (145–156); `StatInfo{Name,Size,MimeType,ModTime,IsDir}` (136–142)
- `operations/about` → `OperationsAbout(ctx, fs)` (166–172); `AboutInfo{Total,Used,Free}` (159–163)
- `operations/copyfile` → `OperationsCopyFile(ctx, srcFS, srcRemote, dstFS, dstRemote)` (176–181)

**NOT wrapped** (header comment at lines 15–16 says only needed operations are wrapped; `core/command` and `config/dump` deliberately absent): `operations/mkdir`, `operations/rmdir`, `operations/deletefile`, `operations/move`/`operations/movefile`, `operations/copy` (dir), `operations/copyurl`, `operations/upload` (multipart) / `operations/uploadfile` (rc multipart), `operations/publiclink` (share links), `sync/*`. The `do()` helper makes any of these trivial to add. Path-joining helper `remoteSpec(fs, remote)` (111–117): `"" → "fs:"`, else `"fs:/remote"`.

`internal/cloud/storage/rcd.go`: `Supervisor` (29), `NewSupervisor(binPath, configDir)` (44), `RC(ctx)` (52) — lazy start with lock, `Shutdown()` (152), and **`RemoteName(accountID) = "goclaw-" + strings.ToLower(accountID)`** (161–165, full UUID, no truncation).

### 2.4 `internal/cloud/manager.go` (552 lines) + access.go + account_by_id.go

- `Manager{cfg, store, bindings, secrets, encKey, storage}` (23–30). `SupportedProviders = []string{GoogleProvider, MicrosoftProvider}` (53); `IsSupportedProvider` (56–58).
- Credential resolution: web-UI-saved `config_secrets` keys `cloud.google.client_id` / `cloud.google.client_secret` / `cloud.microsoft.client_id` / `cloud.microsoft.client_secret` (44–48) win over env (`googleCredentials` 71–82, `microsoftCredentials` 97–108). `GoogleConfigured`/`MicrosoftConfigured` **always return true** (embedded shared rclone client fallback, lines 87–93, 112–118).
- `BuildAuthURL(ctx, provider, baseURL, tenantID, userID) (authURL, redirectURI, mode string, err)` (196–205): BYO client → mode `"callback"` with `RedirectURI(base)` = `base + "/v1/cloud/oauth/callback"` (502–507); embedded client → mode `"paste"` with loopback `http://127.0.0.1:53682/` (google) / `http://localhost:53682/` (microsoft) (`internal/cloud/credentials.go:32-33`). PKCE S256 + `access_type=offline` + `prompt=consent` (276–281).
- `ParseRedirectedURL(rawURL) (code, state, err)` (211–239) — accepts query and fragment forms.
- `HandleCallback(ctx, code, state) (*store.CloudAccount, error)` (325–359) — verifies HMAC state, exchanges, fetches profile, upserts encrypted row.
- `Delete(ctx, id)` (456–464) — DB delete + `storage.RemoveRemote`.
- `StorageService()` / `SetStorageService` (468–471).
- `TokenSource(ctx, accountID) (oauth2.TokenSource, error)` (476–498) — singleflight-refreshing source; errors when no refresh token.
- **`AccountByID(ctx, id) (*store.CloudAccount, error)`** (`internal/cloud/account_by_id.go:12-26`) — resolves from accessible set (own + tenant-shared), NOT owner-scoped Get.
- **`accessibleAccounts(ctx) ([]store.CloudAccount, error)`** (`internal/cloud/access.go:23-41`) — private; `store.List(ctx)` (own) + `store.ListShared(ctx)` deduped by ID.
- **`ResolveAccount(ctx, name string, providers []string, usable func(*store.CloudAccount) bool) (*store.CloudAccount, error)`** (`internal/cloud/access.go:54-138`): resolution order explicit name (email or ID, must be own/shared) → group binding → user binding → tenant default → own accounts preferring active. Revoked targets fall through. `ErrNoAccessibleAccount` (14).

### 2.5 WS gateway: no cloud namespace

`pkg/protocol/methods.go` defines method constants (e.g. `MethodCronList = "cron.list"` at line 177); **there is no `cloud.*` constant anywhere in `pkg/protocol/methods.go`** (grep for "cloud" returns nothing). Gateway method registrations in `internal/gateway/methods/` (files cron.go, agents.go, sessions.go, etc.) contain no cloud file. Cloud is 100% HTTP REST on the ServeMux.

### 2.6 Cron / background jobs

Two layers:
1. In-memory service `internal/cron/`: `Schedule{Kind:"at"|"every"|"cron", AtMS, EveryMS, Expr, TZ}` (types.go:17–23), `Payload{Kind: "agent_turn", Message, Command}` (26–30) — **only payload kind is `agent_turn`**; `Job` struct (41–57); `Service.AddJob(name, schedule, message, deliver bool, channel, to, agentID)` (service.go:123).
2. The DB-backed store the WS methods actually use: `internal/store/cron_store.go` — `CronSchedule{Kind, AtMS, EveryMS, Expr, TZ}` (54–60), `CronCommandSpec{Argv, Cwd,...}` (66+) — **a deterministic shell-command payload ("command" kind) exists**, gated by `cron.command_enabled`, running inside the gateway WITHOUT an LLM (comment at cron_store.go:64-69). Store interface `AddJob(ctx, name, schedule, message, deliver, channel, to, agentID, userID) (*CronJob, error)` (line 201).
   WS surface: `internal/gateway/methods/cron.go:31-39` registers `protocol.MethodCronList/Create/Update/Delete/Toggle/Status/Run/Runs`; create params `{name, schedule, message, command?, deliver, deliverChannel, deliverTo, wakeHeartbeat, stateless?, agentId}` (methods/cron.go:54-67).
   **There is no cloud-sync job type.** Nothing in `internal/cron/` or `internal/store/cron_store.go` mentions cloud. A recurring sync could either be a new payload kind, an agent_turn with a prompt, or a `CronCommandSpec` argv — but a first-class sync engine would be new work. No existing background worker touches clouds (StorageService only reacts to requests/disconnect).

### 2.7 Store: `internal/store/cloud_account_store.go` (115 lines)

- `CloudAccount` struct (18–37): `ID, TenantID, UserID, Provider ("google"|"onedrive"), Email, DisplayName, Scopes (JSON string), AccessToken/RefreshToken (json:"-", AES-256-GCM at rest), TokenExpiresAt *time.Time, Status (active|expired|revoked|error), StatusMessage, Settings (JSON string), Shared bool, CreatedAt, UpdatedAt`.
- Scope constants (40–48): `CloudBindingScopeTenant="tenant"` (scope_key=""), `CloudBindingScopeUser="user"`, `CloudBindingScopeGroup="group"` (scope_key = channel chat id).
- `CloudBinding` struct (54–64): `ID, TenantID, ScopeType, ScopeKey, Provider, AccountID, CreatedBy, CreatedAt, UpdatedAt`. **No ordering/priority/weight column exists.** One binding per (tenant, scope_type, scope_key, provider) (50–53).
- `CloudAccountStore` interface (80–103): `Upsert(ctx, *CloudAccount) error` (by tenant,user,provider,email); `Get(ctx, id)`; `GetByEmail(ctx, provider, email)`; `List(ctx)` (newest first); `ListShared(ctx)`; `SetShared(ctx, id, shared)`; `UpdateTokens(ctx, id, CloudAccountUpdate)`; `Delete(ctx, id)`. All scoped by ctx tenant+user.
- `CloudBindingStore` interface (107–115): **`ListBindings(ctx) ([]CloudBinding, error)`**, **`UpsertBinding(ctx, b *CloudBinding) error`**, **`DeleteBinding(ctx, id) error`** — tenant-scoped.
- PG implementation (`internal/store/pg/cloud_accounts.go`): binding upsert conflict target **`ON CONFLICT (tenant_id, scope_type, scope_key, provider) DO UPDATE SET account_id, created_by, updated_at`** (293–301); `ListBindings` ordered `created_at DESC` (265); account upsert conflicts on `(tenant_id, user_id, provider, email)` (67). SQLite twin: `internal/store/sqlitestore/cloud_accounts.go`.

### 2.8 Agent tools + config

- Agent tools (`internal/tools/cloud_storage.go`, `cloud_mail.go`): `cloud_ls` (52), `cloud_read` (86), `cloud_fetch` (120), `cloud_about` (152), `cloud_accounts` (cloud_mail.go:57), `mail_search` (98), `mail_read` (138), `mail_archive` (178), `mail_unsubscribe` (301). Wired in `cmd/gateway_cloud.go:87-95`.
- Config (`internal/config/config_cloud.go`): `CloudConfig{Enabled *bool (kill-switch), RedirectBaseURL, Google/Microsoft client cfg, MailRatePerMinute, MailReadMaxBytes, FetchSizeCapMB (default 100), RClonePath (default "rclone")}`.
- **OAuth scopes are read-only**: `GoogleScopes` = openid, userinfo.email/profile, gmail.readonly, gmail.labels, gmail.modify, **`drive.readonly`** (`internal/cloud/google.go:28-36`); embedded client uses `EmbeddedGoogleScopes` (Drive-only, no Gmail — manager.go:270). `MicrosoftScopes` = offline_access, User.Read, **`Files.Read.All`** — "there is no write scope — cloud_fetch copies out only" (`internal/cloud/onedrive.go:31-36`). **Any upload/move/delete feature requires new OAuth scopes and re-consent.**

---

## 3. Reusable UI primitives (ui/web)

### 3.1 `ui/web/src/components/ui/` (complete ls)

`alert.tsx, badge.tsx, button.tsx, card.tsx, combobox.tsx, dialog.tsx, inline-edit-text.tsx, input.tsx, label.tsx, radio-group.tsx, scroll-area.tsx, select.tsx, separator.tsx, skeleton.tsx, slider.tsx, switch.tsx, tabs.tsx, textarea.tsx, toaster.tsx, tooltip.tsx`

**Missing**: sheet, popover, dropdown-menu, command, checkbox, context-menu, progress. Shared components worth noting in `ui/web/src/components/shared/`: `page-header`, `empty-state`, `loading-skeleton` (TableSkeleton), `status-badge`, `confirm-dialog`, `file-browser`, `file-tree`, `file-tree-dnd-wrappers`, `file-tree-file-icon` (FileIcon), `file-upload-dialog`, `drag-preview`, `file-viewers`.

### 3.2 Left sub-sidebar layout patterns

- Global shell: `ui/web/src/components/layout/app-layout.tsx` — `flex h-dvh overflow-hidden safe-top` (line 43), global `Sidebar` + `<main className="min-w-0 flex-1 overflow-y-auto">` wrapping `<Outlet/>` (75–77).
- Page-level sub-sidebar: **Chat page** (`ui/web/src/pages/chat/chat-page.tsx`) composes `<ChatSidebar>` + `<ChatThread>` with `useIsMobile()` (hook at `shared/file-browser.tsx:14-23` also exists) and a `PanelLeftOpen` reopen button; session key from `useParams` as source of truth (line 37, per AGENTS.md:279 rule).
- **Storage page** (`ui/web/src/pages/storage/storage-page.tsx`) is the closest analog to a file manager: `FileBrowser` (`shared/file-browser.tsx`) = left `FileTreePanel` + right `FileContentPanel` (file-viewers), mobile switches via `useIsMobile(640)`. This two-panel composition is directly reusable for a Drive-style layout.

### 3.3 Drag and drop — rich existing support

- **`@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities` are dependencies** (package.json lines 17–19).
- `ui/web/src/components/chat/drop-zone.tsx` (49 lines): `DropZone({onDrop: (files: File[]) => void})` — full-area drop overlay with drag-counter; **reusable as-is for cloud upload**.
- `ui/web/src/components/shared/file-upload-dialog.tsx`: multi-file dialog with per-file status machine (`checking|ready|uploading|success|error`), blocked-extension list (11–13), 50MB cap (15), drag-into-dialog (101, 130–133).
- File-tree DnD (move files between folders): `shared/file-tree.tsx:4` (`DndContext, DragOverlay`), `shared/file-tree-dnd-wrappers.tsx` (`DraggableItem`, `DroppableFolder`, `RootDropZone` via `useDraggable/useDroppable`), hook `hooks/use-tree-dnd.ts` (sensors + auto-expand-on-hover, line 84), `shared/drag-preview.tsx`. Sortable list examples: `pages/agents/agent-detail/config-sections/model-fallback-row.tsx`, `pages/builtin-tools/media-sortable-provider-card.tsx`.
- Simple HTML5 drops: `pages/backup-restore/tenant-restore-section.tsx:179-181`, `system-restore-panel.tsx:159-161`.

### 3.4 Grid/list toggle, virtualization, icons, formatting

- No virtualized lists: **`@tanstack/react-virtual` is not a dependency** and `useVirtualizer` has zero hits (only `@tanstack/react-query` and `@tanstack/react-table` are present — package.json:23-24).
- No existing grid/list view-toggle pattern found in pages (storage page is tree+content only; a toggle would be new).
- File icon component: `ui/web/src/components/shared/file-tree-file-icon.tsx` (`FileIcon`) — used by file-tree; folder icons in cloud page are raw lucide `Folder`/`File` (account-detail.tsx:227,238). No thumbnail/image-preview pipeline for cloud files (`shared/file-viewers.tsx` handles storage-page text/media previews).
- Icon library: **`lucide-react` ^1.7.0** (package.json:40).
- Byte formatting: `formatFileSize(bytes)` at `ui/web/src/lib/format.ts:97`; `formatRelativeTime` at `format.ts:14`; `formatSize` + `TreeNode` + `buildTree` at `ui/web/src/lib/file-helpers.ts:151/1/69`. Cloud page has its own duplicate `formatBytes` (account-detail.tsx:42-48).
- Upload transport: `HttpClient.upload(path, formData)` — `ui/web/src/api/http-client.ts:78-94` (POST with auth headers, no explicit Content-Type so browser sets multipart boundary). Example usage: storage page `http.upload("/v1/storage/files?" + params, fd)` (`pages/storage/storage-page.tsx:145-147`).

### 3.5 Backend upload/move endpoints (server-side precedent, for local storage only)

`internal/http/storage.go` routes (53–59): `GET /v1/storage/files`, `GET/DELETE /v1/storage/files/{path...}`, `GET /v1/storage/size`, **`POST /v1/storage/files` (multipart upload, admin+tenantAdmin, temp-file then rename, path-traversal and protected-dir guards, lines 534–653)**, **`PUT /v1/storage/move`** (655+). Similar: `POST /v1/teams/{teamId}/workspace/upload` + `PUT .../workspace/move` (`internal/http/workspace_upload.go:38-39`), `POST /v1/media/upload` (`internal/http/media_upload.go:31`). These are the patterns to mirror for cloud accounts.

### 3.6 i18n

- Namespace `cloud`, files at `ui/web/src/i18n/locales/{en,vi,zh,ko,ru}/cloud.json` — **5 locales, all have cloud.json** (registered in `ui/web/src/i18n/index.ts`; `ns` array includes `"cloud"`; fallback `en`, missing-key console warn in dev).
- en/cloud.json key structure (119 lines): top-level `title, description, refresh, disconnect`; `connect.{google,onedrive,another}`; `status.{active,expired,revoked,error}`; `empty.{title,description}`; `dashboard.{title,accounts,active,providers_ready,needs_setup}`; `picker.{...}`; `provider.{back,accounts_title}`; `setup.{title_google,title_onedrive,body,google_step1..4,onedrive_step1..4,client_id(s),client_secret,client_secret_keep,save,saving,cancel,update,advanced,embedded_note}`; `disconnect_confirm.{title,description}`; `result.{connected,error}`; `paste.{title,description,placeholder,complete,completing}`; `share.toggle`; `scope.{title,description,tenant_default,none,group,user,pick_account,add,missing_fields,key_placeholder_group,key_placeholder_user,key_hint_group,key_hint_user}`; `detail.{files_btn,mail_btn,quota_unavailable,loading_quota,used_of,root,loading_files,files_unavailable,no_files,col_name,col_size,col_modified,readonly_note,mailbox,loading_mail,mail_unavailable,no_mail,no_subject,mail_readonly_note}`.
- Usage pattern: `const { t } = useTranslation("cloud")` then `t("scope.title")`, interpolation `t("result.connected", { email })`.

---

## 4. Constraints

### AGENTS.md mobile rules (lines 262–279) vs current cloud pages

- `h-dvh` never `h-screen` (266) — cloud page is a normal scroll page, N/A but fine.
- Inputs `text-base md:text-sm` (267) — cloud pages comply (cloud-page.tsx:156,164,409; scope-bindings-panel.tsx:214). ui/select.tsx trigger also has `text-base md:text-sm` baked in.
- Touch targets ≥44px via `@media (pointer:coarse)` CSS (269) — cloud uses `min-h-11 sm:min-h-9` on primary buttons; some icon buttons (`h-6 w-6` in file-browser.tsx:45) rely on the global coarse-pointer expansion.
- Tables wrapped in `overflow-x-auto` (270) — FilesBrowser does this with `min-w-[540px]` (account-detail.tsx:196,207); AGENTS.md prescribes `min-w-[600px]`, so the cloud table is 60px short.
- Portal dropdowns in dialogs need `pointer-events-auto` (276) — Radix-native Select/Popover handles automatically; only relevant if custom portals are added.
- ErrorBoundary stable key / URL params as source of truth (278–279) — **cloud page violates the spirit**: `selectedProvider` and `detailTab` are local state, not URL params; a redesign with deep-linkable folders/accounts should move them to route params (`lib/routes.ts:25` has `CLOUD: "/cloud"`, no sub-path params yet).

### Radix Select empty-value pitfall

Fixed in scope-bindings-panel via the `NONE = "__none__"` sentinel (scope-bindings-panel.tsx:22,114-127). Nearby risks:
- The add-row `AccountSelect` for `addAccount` starts as `""` (line 97) and is passed `value={value || undefined}` in AccountSelect (line 70) — the `|| undefined` conversion is the workaround; it works but means "no selection" is indistinguishable from placeholder. Any new account/scope Select in the redesign should copy the sentinel pattern.
- `ui/combobox.tsx` exists as an alternative if searchable pickers are needed (accounts list can grow).

### State

TanStack Query v5 (single QueryClient, `app-providers.tsx:6`), local useState, zustand only for auth/ui/toast/logs. Query keys are ad-hoc arrays `["cloud", ...]` — note there is a centralized `ui/web/src/lib/query-keys.ts` (with `queryKeys.sessions.list(...)` used by use-sessions.ts:24) that cloud does NOT use; adopting it would be consistent.

---

## GAPS — what does NOT exist yet (drives the plan's phase breakdown)

**Backend — rc wrappers (each trivial via `do()`, rc_client.go:35):**
1. `operations/mkdir` wrapper (directory creation).
2. `operations/deletefile` (+ `operations/rmdir` for dirs) wrapper.
3. `operations/move`/`movefile` and dir `move` wrapper (rename/move).
4. `operations/copy` / dir copy + cross-remote copyfile for cross-account transfer (copyfile exists but only used local-bound in `Fetch`).
5. `operations/copyurl` wrapper (upload-by-URL / "paste URL to upload").
6. `operations/upload` / multipart upload wrapper — no upload path into remotes at all; `rc_client.go` HTTP client has no multipart support (`do()` is JSON-only).
7. `operations/publiclink` wrapper (share links).
8. No per-file thumbnail fetch; `operations/stat` returns `MimeType` (rc_client.go:139) — unused by UI today.

**Backend — HTTP surface:**
9. No `POST/PUT/DELETE` endpoints for cloud files (upload, mkdir, rename, move, copy, delete, download/stream, share). Only `GET about/files/mail` (cloud.go:67-69).
10. No download/`fetchBlob` endpoint for cloud files (storage.go `handleRead` analog missing).
11. No recursive listing/search endpoint (`operations/list` recurse, or Google Drive `search`).
12. No trash/restore, starred/favorites, recents, or "shared with me" concepts anywhere.
13. No thumbnails or preview proxy for cloud files.
14. No cross-account transfer endpoint (would pair two remotes via `operations/copyfile` with two `ensureRemote` calls).

**OAuth scopes (hard blocker for write features):**
15. Google granted scope is `drive.readonly` (google.go:35); embedded client is Drive-only (manager.go:270); Microsoft is `Files.Read.All` with explicit no-write comment (onedrive.go:31-36). Write operations require scope additions + provider console changes + re-consent of existing accounts (token refresh won't add scopes).

**Bindings/store:**
16. No priority/weight/order column on `cloud_account_bindings` (struct cloud_account_store.go:54-64; pg upsert target pg/cloud_accounts.go:297). Resolution order is hard-coded group→user→tenant (access.go:87-121).
17. No multi-account-per-scope (conflict target allows exactly one account per scope+provider).
18. No binding audit/history.

**Sync:**
19. No cloud sync job type: cron Payload kind is only `agent_turn` (cron/types.go:27); DB cron has `agent_turn` + `command` kinds (cron_store.go:64-69) — no `cloud_sync` kind; no background worker touches clouds; no sync-state table (cursor, last-run, conflict policy) or delta detection (rc `sync/*` unwrapped).
20. No transfer-queue/progress tracking (job/task model for long uploads/copies) — nothing like it in the cloud domain; `pending_messages`/`tasks` WS methods exist but are chat-scoped.

**Frontend:**
21. No settings-gear entry point on the Clouds page (gear exists nowhere in cloud/; `ProviderClientSetup` is buried in a per-provider collapsible, cloud-page.tsx:504-518).
22. No Google-Drive-style left account/folder rail, breadcrumb history, multi-select (checkbox selection), grid/list toggle, sort menus, search, context menus (no `ui/context-menu.tsx`, `ui/dropdown-menu.tsx`, `ui/checkbox.tsx`, `ui/sheet.tsx`, `ui/progress.tsx` — all must be added or substituted).
23. No drag-drop upload wired to cloud (DropZone exists at components/chat/drop-zone.tsx but only used by chat; file-upload-dialog is storage-scoped).
24. No virtualization (`@tanstack/react-virtual` absent) — large folders (limit 1000, cloud.go:475) will need it.
25. No URL-driven navigation state (`selectedProvider`, `path`, `detailTab` all local state; routes.ts has only `CLOUD: "/cloud"`, routes.ts:25).
26. No upload progress UI for cloud (XHR/fetch progress not implemented in HttpClient.upload; storage upload is fire-and-forget).
27. i18n: new keys must be added to all 5 locales (en, vi, zh, ko, ru) — cloud.json exists in each.
28. Duplicate formatBytes (account-detail.tsx:42) should be consolidated onto `lib/format.ts:97 formatFileSize`.
