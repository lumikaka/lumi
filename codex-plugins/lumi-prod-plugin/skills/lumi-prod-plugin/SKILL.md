---
name: lumi-prod-plugin
description: Use the local Lumi MCP server to inspect or edit an OAuth-authorized project, including story, chapter, premise, picture-book, image, snapshot, and task operations. Use when the user asks to work with Lumi project content through this plugin.
---

# Lumi Desktop plugin

## Connection and project selection

Use the `lumi-prod-plugin` MCP server with the configured local endpoints:

- MCP URL: `http://127.0.0.1:32323/mcp`
- OAuth resource: `http://127.0.0.1:32323/mcp`
- OAuth authorization endpoint: `http://127.0.0.1:32323/oauth/authorize`
- OAuth token endpoint: `http://127.0.0.1:32323/oauth/token`

OAuth resource is the complete MCP URL, including `/mcp`. Let the MCP client discover metadata and perform login; these addresses describe the connection, not a manual token-exchange workflow. The plugin configures only the MCP URL. Codex discovers the resource and authorization server metadata, then dynamically registers its exact loopback callback, including any server-specific suffix. Do not add a fixed client ID, callback URL or redundant OAuth resource to `.mcp.json`. Do not request, invent or copy a desktop launch token, Cookie or project path.

Keep Lumi Desktop running. MCP shares the business listener. Desktop prefers port 32323 but may fall back when occupied; read the actual endpoint in Lumi MCP settings, update the plugin URL and skill dependency URL and use the matching OAuth endpoint base when the port changes, then reauthorize if needed. stdio discovery remains available when a stable HTTP URL cannot be used. If MCP authentication is required, use the client's MCP login flow. The browser opens Lumi, where the user chooses one project and read/edit permission. The plugin cannot select or expand this authorization itself. The user can revoke access in **Settings → MCP settings**. For another project, use a separate connection or reconnect and authorize it explicitly.

## OAuth login troubleshooting

If CLI login reports `Authorization server response missing required issuer`, check the executable and `codex --version` before diagnosing the server. Codex CLI 0.144.1 drops the returned `iss` parameter; the desktop app can bundle a newer CLI while PATH still resolves to the old standalone installation. On macOS, if `/Applications/ChatGPT.app/Contents/Resources/codex` exists, check its version and use that newer executable for `mcp login` instead. Use the existing server name, let the user complete project authorization, and wait for the command result. Do not manually reconstruct callback URLs, omit `iss`, or disable issuer validation to bypass this client bug. CLI login normally opens the authorization page itself; avoid opening the same authorization URL again and creating duplicate requests.

## Start with live documentation

1. Call `read_agent_doc` with `{"path":"/api/v1/agent-docs/overview.md"}`.
2. Read `data.project_uuid`, `data.permission`, `data.external_routes` and `data.external_instructions` from that tool's business envelope. This is the authorized project; never guess its UUID or use an unrelated project from the workspace.
3. Read the relevant route document linked by `data.content` before an unfamiliar operation or a write. Shared docs also describe the built-in Agent; only `external_routes` grants MCP access.
4. Use `request_api` with the documented `/api/v1/projects/<authorized project_uuid>/...` route and a narrow `response_filter`. Do not copy VACS `/api/mcp/current-project/...` paths or `read_api_doc` calls into Lumi.

## Tools and operations

- `read_agent_doc`: accepts only `path`. Use the bundled [API overview](references/overview.md) for terminology, route selection and examples, then obtain the live contract before calling a route.
- `request_api`: requires `method`, `url`, `response_filter`; accepts optional `query` and `request_body`. Writes also require a stable, top-level `idempotency_key`.
- `get_call`: accepts `call_uuid` and returns the persisted operation state/result belonging to this authorization.
- `read_media`: accepts a project `file_uuid`; returns supported image bytes up to 4 MiB. Do not fetch desktop Cookie-protected image URLs or local files directly.

Read the project with `GET /api/v1/projects/<project_uuid>` and `.data | {uuid,name,description,revision}`. Read list identifiers and status first, then retrieve only the details needed. All resource identifiers are public UUIDv7, never database IDs. Do not use raw HTTP, the local filesystem or direct SQLite access to bypass the MCP capability boundary.

Before editing, read the current revision and supply the route's `expected_revision`. A version conflict requires a fresh read and a newly reasoned operation; do not force an overwrite. Reuse the same idempotency key and identical parameters after an uncertain network failure. Different arguments require a new operation after checking the prior result.

`request_api` results use `data.call` for durable state and `data.result` for the business envelope. A call marked `succeeded` for generation means the job was accepted, not that generation finished. Read the returned task UUID through its documented task endpoint when needed. Models and providers come from Lumi configuration; do not inject provider credentials or fabricated chat/run metadata. Client disconnection does not cancel accepted jobs.

A `pending_confirmation` result must be reviewed in Lumi's MCP settings. Show the call UUID and requested operation to the user, then use `get_call` when they have acted; do not keep a blocking loop waiting for approval. Never submit `confirmed=true`, replace the saved request, or use replay as confirmation. Handle rejected/expired as terminal results. For `interrupted`, inspect current business state before considering another write.

If `project_not_open` is returned, ask the user to open the authorized project in Lumi. Do not attempt to open arbitrary paths. If credentials are expired or revoked, use MCP login again; do not create grants through business tools.

## API reference

Read [references/overview.md](references/overview.md) when planning a multi-step operation or identifying a domain. It includes Lumi terminology, the external API route index, response filters and examples for reading, editing, generation, confirmation recovery and media. The bundled index is a source snapshot; live `external_routes` and route contracts take precedence. Export, direct upload and other unlisted VACS capabilities are not exposed by this plugin.
