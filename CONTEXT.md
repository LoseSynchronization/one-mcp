# One MCP — Domain Glossary

## MCP Service Group

| Term | Definition |
|------|-----------|
| **Group** | A named collection of MCP services that are exposed through a single MCP endpoint (`/group/:name/mcp`). |
| **Group Mode** | The strategy by which a group exposes its constituent services' tools to the MCP client. |
| **Native Mode** | Group mode where all tools from all services are exposed directly via `tools/list`, with tool names prefixed by the service name for disambiguation. |
| **Wrapped Mode** | Group mode where tools are accessed via a two-step process: `search_tools` to discover, then `execute_tool` to invoke. The current default. |
| **Tool Delimiter** | The separator string between service name and tool name in native mode. Configurable system-wide via `GroupNativeToolDelimiter` option, defaults to `"."`. Service names are restricted from containing the delimiter character. Splitting is done at the **first** delimiter occurrence. Service names are restricted from containing the delimiter character. |

