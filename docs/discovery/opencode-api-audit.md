# OpenCode API Audit

Discovery date: 2026-06-01

Scope: documented `opencode serve` HTTP API surface only. This document is an audit artifact, not an implementation plan.

## Source policy

Every endpoint below is taken from the OpenCode Server documentation at `https://opencode.ai/docs/server/`. OpenCode states that `opencode serve` runs a headless HTTP server and exposes an OpenAPI 3.1 spec at `/doc`. The public documentation page also links generated SDK/OpenAPI-derived types at `https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts`.

If a request or response body is summarized in the public server page, the summary is recorded. If the public server page only links a type name without expanding all fields, the schema is recorded as `TYPE REFERENCE ONLY` and must be verified against `/doc` from a running OpenCode server before implementation. No field is invented.

## Server operation and authentication

| Item | Documented value | Source URL |
|---|---:|---|
| Command | `opencode serve [--port <number>] [--hostname <string>] [--cors <origin>]` | https://opencode.ai/docs/server/ |
| Default port | `4096` | https://opencode.ai/docs/server/ |
| Default hostname | `127.0.0.1` | https://opencode.ai/docs/server/ |
| OpenAPI spec endpoint | `GET /doc` | https://opencode.ai/docs/server/ |
| Authentication | HTTP Basic Auth when `OPENCODE_SERVER_PASSWORD` is set | https://opencode.ai/docs/server/ |
| Default username | `opencode` unless `OPENCODE_SERVER_USERNAME` is set | https://opencode.ai/docs/server/ |

## Endpoint inventory

### Global

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/global/health` | Get server health and version | None documented | `{ healthy: true, version: string }` | https://opencode.ai/docs/server/ |
| GET | `/global/event` | Get global events | None documented | Event stream | https://opencode.ai/docs/server/ |

Response fields for `/global/health`:

| Field | Type | Source URL |
|---|---|---|
| `healthy` | literal `true` | https://opencode.ai/docs/server/ |
| `version` | `string` | https://opencode.ai/docs/server/ |

### Project

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/project` | List all projects | None documented | `Project[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/project/current` | Get current project | None documented | `Project` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

Known `Project` fields from generated type reference:

| Field | Type | Source URL |
|---|---|---|
| `id` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `worktree` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `vcsDir` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `vcs` | optional literal `git` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `time.created` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `time.initialized` | optional `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

### Path and VCS

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/path` | Get current path | None documented | `Path` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/vcs` | Get VCS info for current project | None documented | `VcsInfo` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

Field-level schemas for `Path` and `VcsInfo`: UNKNOWN from the public server page. Verify against `GET /doc` on a running server.

### Instance

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| POST | `/instance/dispose` | Dispose current instance | None documented | `boolean` | https://opencode.ai/docs/server/ |

### Config

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/config` | Get config info | None documented | `Config` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| PATCH | `/config` | Update config | `Config` TYPE REFERENCE ONLY | `Config` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/config/providers` | List providers and default models | None documented | `{ providers: Provider[], default: { [key: string]: string } }` | https://opencode.ai/docs/server/ |

Response fields for `/config/providers`:

| Field | Type | Source URL |
|---|---|---|
| `providers` | `Provider[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| `default` | map `{ [key: string]: string }` | https://opencode.ai/docs/server/ |

### Provider

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/provider` | List all providers | None documented | `{ all: Provider[], default: {...}, connected: string[] }` | https://opencode.ai/docs/server/ |
| GET | `/provider/auth` | Get provider authentication methods | None documented | `{ [providerID: string]: ProviderAuthMethod[] }` | https://opencode.ai/docs/server/ |
| POST | `/provider/{id}/oauth/authorize` | Authorize provider using OAuth | UNKNOWN | `ProviderAuthAuthorization` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| POST | `/provider/{id}/oauth/callback` | Handle OAuth callback | UNKNOWN | `boolean` | https://opencode.ai/docs/server/ |

Response fields for `/provider`:

| Field | Type | Source URL |
|---|---|---|
| `all` | `Provider[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| `default` | object, fields UNKNOWN from public page | https://opencode.ai/docs/server/ |
| `connected` | `string[]` | https://opencode.ai/docs/server/ |

### Sessions

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/session` | List all sessions | None documented | `Session[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| POST | `/session` | Create new session | `{ parentID?, title? }` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/session/status` | Get status for all sessions | None documented | `{ [sessionID: string]: SessionStatus }` | https://opencode.ai/docs/server/ |
| GET | `/session/:id` | Get session details | Path param `id` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| DELETE | `/session/:id` | Delete session and all data | Path param `id` | `boolean` | https://opencode.ai/docs/server/ |
| PATCH | `/session/:id` | Update session properties | `{ title? }` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/session/:id/children` | Get child sessions | Path param `id` | `Session[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/session/:id/todo` | Get todo list | Path param `id` | `Todo[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| POST | `/session/:id/init` | Analyze app and create `AGENTS.md` | `{ messageID, providerID, modelID }` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/fork` | Fork existing session at a message | `{ messageID? }` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| POST | `/session/:id/abort` | Abort a running session | Path param `id` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/share` | Share a session | Path param `id` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| DELETE | `/session/:id/share` | Unshare a session | Path param `id` | `Session` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/session/:id/diff` | Get session diff | Query `messageID?` | `FileDiff[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| POST | `/session/:id/summarize` | Summarize session | `{ providerID, modelID }` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/revert` | Revert a message | `{ messageID, partID? }` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/unrevert` | Restore all reverted messages | Path param `id` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/permissions/:permissionID` | Respond to permission request | `{ response, remember? }` | `boolean` | https://opencode.ai/docs/server/ |

Request body fields:

| Endpoint | Field | Type | Required | Source URL |
|---|---|---|---:|---|
| `POST /session` | `parentID` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session` | `title` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `PATCH /session/:id` | `title` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/init` | `messageID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/init` | `providerID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/init` | `modelID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/fork` | `messageID` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/summarize` | `providerID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/summarize` | `modelID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/revert` | `messageID` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/revert` | `partID` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/permissions/:permissionID` | `response` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/permissions/:permissionID` | `remember` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |

Known `Session` fields from generated type reference:

| Field | Type | Source URL |
|---|---|---|
| `id` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `projectID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `directory` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `parentID` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `summary.additions` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `summary.deletions` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `summary.files` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `summary.diffs` | optional `FileDiff[]` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `share.url` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `title` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `version` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `time.created` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `time.updated` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `time.compacting` | optional `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `revert.messageID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `revert.partID` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `revert.snapshot` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `revert.diff` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

Known `SessionStatus` variants from generated type reference:

| Variant | Fields | Source URL |
|---|---|---|
| `idle` | `type: "idle"` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `busy` | `type: "busy"` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `retry` | `type: "retry"`, `attempt: number`, `message: string`, `next: number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

Known `Todo` fields from generated type reference:

| Field | Type | Source URL |
|---|---|---|
| `content` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `status` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `priority` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `id` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

### Messages

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/session/:id/message` | List messages in a session | Query `limit?` | `{ info: Message, parts: Part[] }[]` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/message` | Send message and wait for response | `{ messageID?, model?, agent?, noReply?, system?, tools?, parts }` | `{ info: Message, parts: Part[] }` | https://opencode.ai/docs/server/ |
| GET | `/session/:id/message/:messageID` | Get message details | Path params `id`, `messageID` | `{ info: Message, parts: Part[] }` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/prompt_async` | Send message asynchronously without waiting | Same as `/session/:id/message` | `204 No Content` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/command` | Execute slash command | `{ messageID?, agent?, model?, command, arguments }` | `{ info: Message, parts: Part[] }` | https://opencode.ai/docs/server/ |
| POST | `/session/:id/shell` | Run shell command | `{ agent, model?, command }` | `{ info: Message, parts: Part[] }` | https://opencode.ai/docs/server/ |

Request body fields:

| Endpoint | Field | Type | Required | Source URL |
|---|---|---|---:|---|
| `POST /session/:id/message` and `POST /session/:id/prompt_async` | `messageID` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| Same | `model` | UNKNOWN object/string from public page | No | https://opencode.ai/docs/server/ |
| Same | `agent` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| Same | `noReply` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| Same | `system` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| Same | `tools` | UNKNOWN object/map from public page | No | https://opencode.ai/docs/server/ |
| Same | `parts` | UNKNOWN array element schema from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/command` | `messageID` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/command` | `agent` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/command` | `model` | UNKNOWN object/string from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/command` | `command` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/command` | `arguments` | UNKNOWN scalar/object from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/shell` | `agent` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /session/:id/shell` | `model` | UNKNOWN object/string from public page | No | https://opencode.ai/docs/server/ |
| `POST /session/:id/shell` | `command` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |

Known `Message` / `Part` field-level details from generated type reference:

| Type | Field | Type | Source URL |
|---|---|---|---|
| `UserMessage` | `id` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `sessionID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `role` | literal `user` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `time.created` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `agent` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `model.providerID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `model.modelID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `system` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `UserMessage` | `tools` | optional `{ [key: string]: boolean }` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `id` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `sessionID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `role` | literal `assistant` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `time.created` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `time.completed` | optional `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `parentID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `modelID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `providerID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `mode` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `path.cwd` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `path.root` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `cost` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `tokens.input` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `tokens.output` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `tokens.reasoning` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `tokens.cache.read` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `tokens.cache.write` | `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `AssistantMessage` | `finish` | optional `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `TextPart` | `id`, `sessionID`, `messageID` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `TextPart` | `type` | literal `text` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `TextPart` | `text` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `ReasoningPart` | `type`, `text`, `time.start`, `time.end` | literal `reasoning`, `string`, `number`, optional `number` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `FilePart` | `type`, `mime`, `filename`, `url` | literal `file`, `string`, optional `string`, `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `ToolPart` | `type`, `callID`, `tool`, `state` | literal `tool`, `string`, `string`, `ToolState` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `PatchPart` | `type`, `hash`, `files` | literal `patch`, `string`, `string[]` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

### Commands

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/command` | List all commands | None documented | `Command[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

### Files

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/find?pattern=<pat>` | Search text in files | Query `pattern` | Array of match objects with `path`, `lines`, `line_number`, `absolute_offset`, `submatches` | https://opencode.ai/docs/server/ |
| GET | `/find/file?query=<q>` | Find files and directories by name | Query `query`; optional `type`, `directory`, `limit`, `dirs` | `string[]` | https://opencode.ai/docs/server/ |
| GET | `/find/symbol?query=<q>` | Find workspace symbols | Query `query` | `Symbol[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/file?path=<path>` | List files and directories | Query `path` | `FileNode[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/file/content?path=<p>` | Read file | Query `path` | `FileContent` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/file/status` | Get tracked file status | None documented | `File[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

Documented `/find/file` query parameters:

| Field | Type | Required | Source URL |
|---|---|---:|---|
| `query` | search string | Yes | https://opencode.ai/docs/server/ |
| `type` | `"file"` or `"directory"` | No | https://opencode.ai/docs/server/ |
| `directory` | project-root override | No | https://opencode.ai/docs/server/ |
| `limit` | max results, `1–200` | No | https://opencode.ai/docs/server/ |
| `dirs` | legacy flag; `"false"` returns only files | No | https://opencode.ai/docs/server/ |

Documented `/find?pattern=<pat>` response fields:

| Field | Type | Source URL |
|---|---|---|
| `path` | UNKNOWN scalar type from public page | https://opencode.ai/docs/server/ |
| `lines` | UNKNOWN type from public page | https://opencode.ai/docs/server/ |
| `line_number` | UNKNOWN scalar type from public page | https://opencode.ai/docs/server/ |
| `absolute_offset` | UNKNOWN scalar type from public page | https://opencode.ai/docs/server/ |
| `submatches` | UNKNOWN array element schema from public page | https://opencode.ai/docs/server/ |

### Tools / Experimental

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/experimental/tool/ids` | List tool IDs | None documented | `ToolIDs` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/experimental/tool?provider=<p>&model=<m>` | List tools with JSON schemas for a model | Query `provider`, `model` | `ToolList` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

### LSP, Formatters, MCP

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/lsp` | Get LSP server status | None documented | `LSPStatus[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/formatter` | Get formatter status | None documented | `FormatterStatus[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |
| GET | `/mcp` | Get MCP server status | None documented | `{ [name: string]: MCPStatus }` | https://opencode.ai/docs/server/ |
| POST | `/mcp` | Add MCP server dynamically | `{ name, config }` | MCP status object | https://opencode.ai/docs/server/ |

Request body fields for `POST /mcp`:

| Field | Type | Required | Source URL |
|---|---|---:|---|
| `name` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `config` | UNKNOWN object schema from public page | Yes | https://opencode.ai/docs/server/ |

### Agents

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/agent` | List all available agents | None documented | `Agent[]` TYPE REFERENCE ONLY | https://opencode.ai/docs/server/ |

Field-level `Agent` schema: UNKNOWN from public server page and not captured in the available generated-type excerpt. Verify against `GET /doc`.

### Logging

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| POST | `/log` | Write log entry | `{ service, level, message, extra? }` | `boolean` | https://opencode.ai/docs/server/ |

Request body fields for `POST /log`:

| Field | Type | Required | Source URL |
|---|---|---:|---|
| `service` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `level` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `message` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `extra` | UNKNOWN schema from public page | No | https://opencode.ai/docs/server/ |

### TUI

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| POST | `/tui/append-prompt` | Append text to prompt | UNKNOWN | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/open-help` | Open help dialog | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/open-sessions` | Open session selector | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/open-themes` | Open theme selector | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/open-models` | Open model selector | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/submit-prompt` | Submit current prompt | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/clear-prompt` | Clear prompt | None documented | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/execute-command` | Execute command | `{ command }` | `boolean` | https://opencode.ai/docs/server/ |
| POST | `/tui/show-toast` | Show toast | `{ title?, message, variant }` | `boolean` | https://opencode.ai/docs/server/ |
| GET | `/tui/control/next` | Wait for next control request | None documented | Control request object; fields UNKNOWN | https://opencode.ai/docs/server/ |
| POST | `/tui/control/response` | Respond to control request | `{ body }` | `boolean` | https://opencode.ai/docs/server/ |

Request body fields:

| Endpoint | Field | Type | Required | Source URL |
|---|---|---|---:|---|
| `POST /tui/execute-command` | `command` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /tui/show-toast` | `title` | UNKNOWN scalar type from public page | No | https://opencode.ai/docs/server/ |
| `POST /tui/show-toast` | `message` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /tui/show-toast` | `variant` | UNKNOWN scalar type from public page | Yes | https://opencode.ai/docs/server/ |
| `POST /tui/control/response` | `body` | UNKNOWN schema from public page | Yes | https://opencode.ai/docs/server/ |

### Auth

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| PUT | `/auth/:id` | Set authentication credentials | Body must match provider schema | `boolean` | https://opencode.ai/docs/server/ |

Request body fields for `PUT /auth/:id`: UNKNOWN because the body is provider-specific and must match provider schema.

### Events

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/event` | Server-sent events stream | None documented | First event `server.connected`, then bus events | https://opencode.ai/docs/server/ |

Known event stream shape from generated type reference:

| Field | Type | Source URL |
|---|---|---|
| `EventServerConnected.type` | literal `server.connected` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `EventServerConnected.properties` | `{ [key: string]: unknown }` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `GlobalEvent.directory` | `string` | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |
| `GlobalEvent.payload` | `Event` union | https://github.com/anomalyco/opencode/blob/dev/packages/sdk/js/src/gen/types.gen.ts |

### Docs

| Method | Path | Purpose | Request schema | Response schema | Source URL |
|---|---|---|---|---|---|
| GET | `/doc` | OpenAPI 3.1 specification HTML page with OpenAPI spec | None documented | HTML page with OpenAPI spec | https://opencode.ai/docs/server/ |

## Endpoint mapping for gateway-required capabilities

| Capability | Required OpenCode endpoint(s) | Status | Evidence URL |
|---|---|---|---|
| Session creation | `POST /session` | DOCUMENTED | https://opencode.ai/docs/server/ |
| Sending messages and receiving synchronous response | `POST /session/:id/message` | DOCUMENTED | https://opencode.ai/docs/server/ |
| Sending messages asynchronously | `POST /session/:id/prompt_async` | DOCUMENTED | https://opencode.ai/docs/server/ |
| Receiving responses by polling/listing | `GET /session/:id/message`, `GET /session/:id/message/:messageID` | DOCUMENTED | https://opencode.ai/docs/server/ |
| Receiving responses by stream | `GET /event`, possibly `GET /global/event` | DOCUMENTED as SSE streams; exact correlation contract UNKNOWN from public page | https://opencode.ai/docs/server/ |
| Listing agents | `GET /agent` | DOCUMENTED | https://opencode.ai/docs/server/ |
| Cancelling requests | `POST /session/:id/abort` | DOCUMENTED | https://opencode.ai/docs/server/ |

## Discovery gaps requiring live `/doc` verification

These are hard blockers for implementation, not optional polish:

1. Exact field-level request schema for `POST /session/:id/message`, especially `model`, `agent`, `parts`, `tools`, and `noReply`.
2. Exact field-level request schema for `POST /session/:id/prompt_async`.
3. Exact `Agent` response fields from `GET /agent`.
4. Exact SSE event framing and event names needed to correlate a streamed response with a specific session/message.
5. Exact error schemas and HTTP status codes for the required endpoints.
6. Exact auth behavior for Basic Auth challenge/failure responses.

