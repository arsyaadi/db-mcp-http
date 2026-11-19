// Thin stdio <-> Streamable HTTP bridge: Claude Desktop talks stdio, the real server (tools, DB, instructions) is remote.
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js';

const token = process.env.MCP_TOKEN ?? '';
// Forgiving for hand-typed values: "host:3333" -> "http://host:3333/mcp".
let url = (process.env.MCP_URL ?? '').trim();
if (url && !/^https?:\/\//i.test(url)) url = `http://${url}`;
let parsed = null;
try {
  parsed = new URL(url);
  if (parsed.pathname === '/') parsed.pathname = '/mcp';
} catch {
  console.error(`Invalid Server URL: "${process.env.MCP_URL}"`);
}

const local = new StdioServerTransport();
const remote = parsed && new StreamableHTTPClientTransport(parsed, {
  requestInit: { headers: { Authorization: `Bearer ${token}` } },
});

// Answer a failed request with a readable error instead of leaving Claude waiting.
const reply = (msg, text) =>
  msg.id !== undefined && local.send({ jsonrpc: '2.0', id: msg.id, error: { code: -32000, message: text } });

local.onmessage = (msg) =>
  !remote
    ? reply(msg, `Server URL "${process.env.MCP_URL}" is invalid. Use the full address, e.g. http://127.0.0.1:3333/mcp`)
    : remote.send(msg).catch((e) => {
        reply(msg, e.code === 401 ? 'DB server rejected the token. Check the token in the extension settings.' : `Cannot reach DB server at ${parsed.href}: ${e.message}`);
      });
local.onclose = () => (remote ? remote.close() : Promise.resolve()).finally(() => process.exit(0));

if (remote) {
  remote.onmessage = (msg) => local.send(msg);
  remote.onerror = (e) => console.error('remote:', e.message);
  await remote.start();
}
await local.start();
