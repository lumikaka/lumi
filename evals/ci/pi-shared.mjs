// One fresh process and real pi Agent per task. Only tools/fixtures are simulated.
import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { createInterface } from 'node:readline';
import { pathToFileURL } from 'node:url';

// The pinned packages export ESM only; CommonJS require.resolve cannot select
// their import export condition. Load the verified checkout's built ESM files.
const { Agent } = await import(pathToFileURL(path.join(process.env.PI_DIR, 'packages/agent/dist/index.js')));
const { stream } = await import(pathToFileURL(path.join(process.env.PI_DIR, 'packages/ai/dist/api/openai-completions.js')));
if (process.argv[2] === '--check-runtime') {
  if (typeof Agent !== 'function' || typeof stream !== 'function') throw new Error('Invalid pi runtime exports');
  console.log('Pinned pi Agent and model transport imported successfully; no model request made.');
  process.exit(0);
}
const [protocolPath, slug, repetition, directory] = process.argv.slice(2);
const rawProtocol = readFileSync(protocolPath);
const protocol = JSON.parse(rawProtocol);
const cfg = protocol.config;
const secret = process.env.LUMI_EVAL_API_KEY;
if (!secret) throw new Error('Missing evaluation credential');
const redact = value => JSON.stringify(value).replaceAll(secret, '[REDACTED]');
const tracePath = path.join(directory, 'trace.jsonl');
const record = (type, data) => appendFileSync(tracePath, redact({time: new Date().toISOString(), type, data}) + '\n');
const started = Date.now();
const bridgeEnv = {...process.env};
delete bridgeEnv.LUMI_EVAL_API_KEY;
const bridge = spawn(process.env.LUMI_EVAL_BINARY, ['--bridge-case', slug], {env: bridgeEnv, stdio: ['pipe', 'pipe', 'pipe']});
bridge.stderr.on('data', data => record('bridge_stderr', data.toString()));
const replies = createInterface({input: bridge.stdout})[Symbol.asyncIterator]();
// Pi's tools run sequentially, so the JSONL request/reply channel has one caller.
const rpc = async request => {
  bridge.stdin.write(JSON.stringify(request) + '\n');
  const line = await replies.next();
  if (line.done) throw new Error('Evaluation tool bridge ended unexpectedly');
  return JSON.parse(line.value);
};
const definition = await rpc({op: 'describe'});
const expected = protocol.spec.cases.find(c => c.slug === slug);
// JSON is emitted by the same compiled Go binary; compare parsed values without key-order dependence.
const canonical = value => JSON.stringify(value, (_, v) => v && !Array.isArray(v) && typeof v === 'object'
  ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a.localeCompare(b))) : v);
if (!expected || canonical(definition.case) !== canonical(expected) ||
    canonical(definition.tools) !== canonical(protocol.spec.tools[slug]) || definition.system_prompt !== protocol.spec.system_prompt) {
  bridge.kill();
  throw new Error('Shared task, tools or system prompt drift');
}
const model = {
  id: cfg.model, name: cfg.model, provider: 'lumi-eval', api: 'openai-completions', baseUrl: cfg.base_url,
  reasoning: true, input: ['text'], contextWindow: 262144, maxTokens: cfg.max_tokens,
  cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0},
  compat: {maxTokensField: 'max_tokens', supportsDeveloperRole: false, supportsStore: false,
    supportsReasoningEffort: false, thinkingFormat: 'openai', supportsUsageInStreaming: true},
};
let requests = 0, budget = false, timedOut = false, bridgeFailed = false;
const requestUsage = [];
const tools = (definition.tools || []).map(t => ({...t, label: t.name, execute: async (_id, args) => {
  try {
    const result = await rpc({op: 'call', name: t.name, arguments: args});
    return {content: [{type: 'text', text: result.content}], details: {}, isError: result.is_error};
  } catch (error) { bridgeFailed = true; throw error; }
}}));
const agent = new Agent({
  initialState: {systemPrompt: definition.system_prompt, model, tools, thinkingLevel: 'off'},
  toolExecution: 'sequential', getApiKey: () => secret,
  streamFn: (selected, context, options) => {
    if (requests >= cfg.max_requests_per_turn) { budget = true; throw new Error('Evaluation request budget exhausted'); }
    const index = requests++;
    requestUsage.push(null);
    return stream(selected, context, {...options, maxTokens: cfg.max_tokens,
      onPayload: payload => { record('model_request', {request: index + 1, payload}); },
      // Keep HTTP metadata without headers, which can contain provider-side credentials/cookies.
      onResponse: response => record('http_response', {request: index + 1, status: response.status}),
      onProviderStreamEvent: chunk => {
        if (chunk?.usage) requestUsage[index] = chunk.usage;
        record('provider_chunk', {request: index + 1, chunk});
      },
    });
  },
});
agent.subscribe(event => {
  // Raw stream chunks plus final messages preserve all content without repeated growing partial messages.
  if (event.type !== 'message_update') record('agent_event', event);
});
record('task_started', {task: {case_slug: slug, repetition: Number(repetition)}, model,
  system_prompt: definition.system_prompt, tools: definition.tools});
const timer = setTimeout(() => { timedOut = true; agent.abort(); }, cfg.timeout_seconds * 1000);
let final = '', status = 'scored', issues = [], graded = {scores: null, tools: [], files: {}};
try {
  for (const step of expected.steps) {
    await agent.prompt(step.content);
    const last = agent.state.messages.findLast(m => m.role === 'assistant');
    final = (last?.content || []).filter(c => c.type === 'text').map(c => c.text).join('');
    if (timedOut || bridgeFailed) {
      status = 'environment_error';
      issues.push({kind: 'environment', code: timedOut ? 'timeout' : 'tool_bridge', message: agent.state.errorMessage || 'Evaluation interrupted'});
    } else if (budget || last?.stopReason === 'length') {
      status = 'agent_error';
      issues.push({kind: 'agent', code: budget ? 'request_budget' : 'output_limit', message: agent.state.errorMessage || 'Output limit reached'});
    } else if (agent.state.errorMessage || !last || ['error', 'aborted'].includes(last.stopReason)) {
      status = 'environment_error';
      issues.push({kind: 'environment', code: 'provider_or_runtime_error', message: agent.state.errorMessage || last?.errorMessage || 'No completed assistant response'});
    }
    if (status !== 'scored') break;
  }
  graded = await rpc({op: 'grade', final});
} catch (error) {
  status = 'environment_error';
  issues.push({kind: 'environment', code: 'adapter_error', message: String(error)});
} finally {
  clearTimeout(timer);
  bridge.stdin.end();
  bridge.kill();
}
const metric = getter => requestUsage.length && requestUsage.every(u => {
  const v = u && getter(u); return typeof v === 'number' && Number.isFinite(v) && v >= 0;
}) ? requestUsage.reduce((sum, u) => sum + getter(u), 0) : null;
const usage = {requests, input_tokens: metric(u => u.prompt_tokens), output_tokens: metric(u => u.completion_tokens),
  cached_input_tokens: metric(u => u.prompt_tokens_details?.cached_tokens), cost_usd: null};
record('task_finished', {status, issues, usage, messages: agent.state.messages, ...graded});
const result = {task: {case_slug: slug, repetition: Number(repetition)},
  protocol_digest: createHash('sha256').update(rawProtocol).digest('hex'), status, final,
  scores: status === 'environment_error' ? null : graded.scores, tools: graded.tools, files: graded.files, issues, usage,
  started_at: new Date(started).toISOString(), duration_ms: Date.now() - started,
  trace_sha256: createHash('sha256').update(readFileSync(tracePath)).digest('hex')};
writeFileSync(path.join(directory, 'result.json'), redact(result) + '\n');
console.log(`${slug}/run-${repetition}: ${status}; ${requests} requests; ${(result.duration_ms / 1000).toFixed(1)}s`);
process.exitCode = status === 'environment_error' ? 1 : 0;
