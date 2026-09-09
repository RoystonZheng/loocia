# AI Cool Tool Discovery Requirements

## Clarified Requirements

AI Cool needs a new tools discovery and sedimentation module for the current ai-cool repository. The module must use Cooper document 2209600482571 section 5 as the requirement baseline, overriding the older first-phase read-only and Git Markdown report import direction recorded in the historical technical plan.

The first release must provide a complete closed loop from discovery configuration to team tool listing. Operators can create, edit, enable, disable, and manually run discovery configurations. A configuration has a name, discovery method, keyword terms or GitHub Topics, trigger type manual or weekly, enabled state, and run metadata. GitHub discovery uses official GitHub APIs only: Repository Search for keyword and Topic discovery, and Repository API for manual repository URL add. GitHub Trending page parsing is out of scope. Public repositories are in scope, archived repositories are excluded, forks are excluded by default, and advanced GitHub search parameters are not exposed in the first-release UI.

Tools are deduplicated by GitHub node_id. Repeated matches update recent discovery time and merge discovery sources instead of creating duplicate tools. The only tool statuses are discovered, evaluating, included, and excluded. There is no observe status in the first release. Discovered tools can be moved to evaluating by entering evaluator, operator, and a Cooper evaluation document link. Discovered tools can also be marked excluded as not handled. Evaluating tools can be completed as included or excluded. Included tools appear in the team tools list; excluded tools stay stored for dedupe and audit but do not appear in primary lists.

The evaluator creates and edits the Cooper evaluation document outside AI Cool. AI Cool only stores the Cooper URL and does not create the document, read document content, or validate document permissions. The first release validates only that the Cooper URL belongs to the expected Cooper domain. Operator and evaluator identity is manually entered by the user in the first release; SSO and RBAC are out of scope, but write operations must persist actor fields and status events for audit.

The front end must add tools navigation while staying consistent with the existing React, Vite, TypeScript, hash-route, and self-managed CSS application. Planned views are discovered tools, evaluating tools, team tools, and discovery configs. Discovered tools show repository name, one-line description, source, stars, seven-day stars growth, recent commit, GitHub link, and actions to start evaluation or not handle. Evaluating tools show evaluator, start time, Cooper link, and actions to finish included or not included. Team tools show only included tools with the evaluator-confirmed one-line summary, GitHub link, Cooper link, and included time. Discovery configs are first-release user-facing pages.

The back end must add an independent tools domain instead of reusing raw_items or items. The design must preserve the existing Go plus PostgreSQL pattern, module-local schema.sql with EnsureSchema, publicapi handlers, shared pgxpool, and dual HTTP entrypoint registration. It must include storage for discovery configs, discovery runs, tools, discovery sources, evaluations, status events, and stars snapshots. It must include GitHub rate-limit and incomplete-result recording, bounded pagination, weekly discovery execution, manual config execution, manual add, and stars snapshot refresh. Deployment planning must include a discovertools command and cron integration, but no production code should be implemented during planning.

The work is accepted when the designed and planned implementation can support these cases end to end: create keyword config, create Topic config, manually run config, weekly run enabled configs, manually add GitHub repo, merge duplicate discoveries, start evaluation, exclude discovered item, finish evaluation as included, finish evaluation as excluded, view team tools, calculate seven-day stars growth, record GitHub limit or failure information, and reject or serialize invalid concurrent status operations.

## Sources

- External: `https://cooper.didichuxing.com/didocs/2209600482571` at `section 5 read 2026-09-07` — Baseline tool discovery and evaluation requirements
- External: `codex-thread:01a07b13-0116-7c53-8703-40cc9e6a6ebd` at `2026-09-09` — User confirmed fifth section baseline manual actors user owned Cooper documents and first release config page
- Repository: `server/cmd/webserver/main.go` at `0b39a0afa129799c74381282f5a63fcdeb318c14` — Current Go HTTP entrypoint and public route registration pattern
- Repository: `web/src/App.tsx` at `0b39a0afa129799c74381282f5a63fcdeb318c14` — Current frontend hash route and view switching pattern
- Repository: `web/package.json` at `0b39a0afa129799c74381282f5a63fcdeb318c14` — Current React Vite TypeScript test and lint stack
- Repository: `docs/design/ai-cool-2.0-product-technical-plan/README.md` at `0b39a0afa129799c74381282f5a63fcdeb318c14` — Historical first phase read only plan that the new requirement overrides
