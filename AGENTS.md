# Regrow Terminal

All new work belongs in this directory. Do not modify the legacy repositories in `../src`.

## Architecture
- `apps/web`: Next.js App Router, TypeScript strict mode, Spanish UI.
- `apps/engine`: Go modular backend. Trading arithmetic uses decimal, never binary floats.
- `contracts`: API specification. Keep it consistent with Go DTOs and web schemas.
- `database/migrations`: PostgreSQL migration source of truth (not automatically applied).

## Safety
- Simulation only until exchange reconciliation, credential storage and execution recovery are implemented and tested.
- Never claim that a preview is an executed order or that a stop guarantees a maximum loss.
- Authenticate on the server, validate resource ownership, reject unknown request fields, and never expose engine tokens to the browser.
- No default passwords or secrets. No secrets in logs, tests, commits or fixtures.
- Tests must not call exchanges. Financial examples are synthetic.

## Verification
- Root: `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`.
- Engine directory: `go test ./...`, `go vet ./...`.
- Write unit tests for strategy rules and security changes. Keep docs honest about unimplemented features.
