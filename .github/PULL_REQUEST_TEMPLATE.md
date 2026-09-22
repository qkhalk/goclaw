## Summary

<!-- What changed and why. Link the plan/issue if one exists. -->

## Surface parity

- [ ] Gateway server (handlers, stores, migrations, background jobs)
- [ ] API contract (`pkg/protocol`, request/response shapes)
- [ ] Web UI (`ui/web` screens, hooks, i18n ×5, loading/error states)
- [ ] CLI/runtime package (`cmd`, installers, docs)

Mark N/A with a one-line reason.

## Checks

- [ ] `go build ./...` and `go build -tags sqliteonly ./...` pass
- [ ] `go vet ./...` clean
- [ ] `pnpm build` (ui/web) green
- [ ] Migrations updated for BOTH PostgreSQL (`migrations/` + `RequiredSchemaVersion`) and SQLite (`schema.sql` + patch + `SchemaVersion`) — or N/A
- [ ] i18n keys added to all backend catalogs / all UI locale files — or N/A

## Docs

- [ ] Docs updated? (open a goclaw-docs PR if user-facing)
