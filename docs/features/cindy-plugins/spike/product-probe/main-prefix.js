/* Test harness only. Uses Host-created empty tasks; never starts another Agent.
 * The product adapter below is embedded byte-for-byte by build.mjs. Only its
 * context source is replaced with the Host's own tasks.create/get receipts.
 * This tests the media adapter, not ordinary product context injection.
 */
(() => {
  'use strict';
  const native = cindy;
  const fixtures = new Map(), observations = [];
  const channel = new BroadcastChannel('lumi-host-acceptance-20261101');
  let product;
  const sdk = { node: native.node, send: message => native.send(message), onHostMessage: handler => { product = handler; } };
  const reply = (msg, result) => native.send({ type: 'tool-result', callId: msg.callId, ok: true, result });
  async function fixture(which) {
    if (!['a', 'b'].includes(which) || !fixtures.has(which)) throw new Error('Create the named Host-owned fixture first');
    const saved = fixtures.get(which), task = await native.tasks.get({ taskId: saved.taskId });
    if (task.taskId !== saved.taskId || task.workingDir !== saved.workingDir || !task.workingDir?.startsWith('/')
      || ['plan', 'readOnly', 'read-only'].includes(task.permissionMode)) throw new Error('Fixture context changed or is read-only');
    return { session_id: task.taskId, workdir: task.workingDir, workdir_is_local: true, workdir_is_read_only: false };
  }
  channel.onmessage = ({ data }) => {
    if (!data || data.kind !== 'observation' || JSON.stringify(data).length > 4096) return;
    observations.push(data); if (observations.length > 32) observations.shift();
    if (data.event === 'loaded') channel.postMessage({ kind: 'probe-fixtures',
      task_ids: [...fixtures.values()].map(f => f.taskId),
      note: 'Session is a routing claim. No project content is released without a verified page identity.' });
  };
  native.onHostMessage(async msg => {
    if (msg.type !== 'tool-call') return;
    try {
      const args = msg.args || {}, ctx = args.session_context;
      if (!msg.callId || !ctx?.workdir_is_local || ctx.workdir_is_read_only) throw new Error('Trusted writable local caller required');
      if (msg.tool === 'create_fixture') {
        if (!['a', 'b'].includes(args.fixture)) throw new Error('fixture must be a or b');
        const title = 'Lumi acceptance · ' + args.fixture.toUpperCase();
        let created;
        if (args.task_id) {
          // Recovery uses an earlier Host receipt. tasks.get enforces this
          // plugin's ownership; no model-supplied filesystem path is accepted.
          created = await native.tasks.get({ taskId: args.task_id });
          if (created.taskId !== args.task_id || created.title !== title) throw new Error('Fixture receipt identity does not match');
        } else {
          const caps = await native.tasks.capabilities();
          created = await native.tasks.create({ requestKey: 'lumi-host-acceptance-20261101-' + args.fixture,
            title, isolatedWorkspace: true,
            ...(caps.sourceCallContext ? { callId: msg.callId } : {}) });
        }
        if (!created.taskId || !created.workingDir?.startsWith('/')) throw new Error('Host did not allocate an isolated local task');
        fixtures.set(args.fixture, created);
        return reply(msg, { fixture: args.fixture, task_id: created.taskId, permission_mode: created.permissionMode,
          empty_task: true, agent_started: false, context_source: args.task_id ? 'host_tasks_get_previous_receipt' : 'host_tasks_create' });
      }
      if (msg.tool === 'report') return reply(msg, { fixture_ids: [...fixtures].map(([key, v]) => ({ fixture: key, task_id: v.taskId })), observations,
        page_read_policy: 'denied_without_host_verified_page_identity', agent_started: false });
      if (msg.tool === 'open_probe') {
        const sessionId = args.fixture ? (await fixture(args.fixture)).session_id : ctx.session_id;
        return reply(msg, await native.taskView({ sessionId }));
      }
      const context = await fixture(args.fixture);
      if (msg.tool === 'release_fixture') {
        const result = await native.node.request({ method: 'runtime_release', params: { context }, callId: msg.callId, cancelWithCall: true, timeoutMs: 60000 });
        return reply(msg, result);
      }
      if (msg.tool === 'receive_media') {
        const input = Object.fromEntries(['target_uuid', 'slot', 'expected_revision', 'fit_mode', 'description', 'prompt']
          .filter(key => Object.hasOwn(args, key)).map(key => [key, args[key]]));
        return product({ ...msg, tool: 'receive_media', args: { ...input, attachments: args.attachments, session_context: context } });
      }
      if (msg.tool !== 'run') throw new Error('Unknown fixture tool');
      return product({ ...msg, tool: 'call_tool', args: { session_context: context, name: args.name, args: args.args || {} } });
    } catch (error) {
      await native.send({ type: 'tool-result', callId: msg.callId, ok: false, errorCode: 'PROBE_FAILED',
        message: 'Fixture operation failed: ' + String(error?.message || error).slice(0, 500) });
    }
  });
  function installProduct(cindy) {
