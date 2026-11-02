'use strict';
const channel = new BroadcastChannel('lumi-host-acceptance-20261101');
const route = new URLSearchParams(location.search).get('session');
const observe = (event, extra = {}) => channel.postMessage({ kind: 'observation', event, route,
  theme: getComputedStyle(document.documentElement).colorScheme, ...extra });
async function record() {
  const wake = await (await fetch('/wake')).json();
  const context = await (await fetch('/app-context')).json();
  const publicKeys = Object.keys(context.context || {}).sort();
  document.querySelector('#result').textContent = JSON.stringify({ route, wake, public_context_keys: publicKeys,
    has_preload: typeof window.cindy !== 'undefined', page_identity_verified: false }, null, 2);
  observe('loaded', { public_context_keys: publicKeys, has_preload: typeof window.cindy !== 'undefined', wake });
}
channel.onmessage = ({ data }) => {
  if (data?.kind !== 'probe-fixtures') return;
  // Only diagnostic claims, never a business read. A different routing label
  // can be emitted from the same WebView without any Host identity assertion.
  for (const claimed of data.task_ids || []) observe('alternate-session-claim', { claimed_session: claimed,
    source_route: route, project_content_read: false, host_attestation_present: false });
  document.querySelector('#state').textContent = '已记录通信边界。页面身份尚未验证，项目读取保持关闭。';
};
document.querySelector('#retry').onclick = () => record().catch(error => observe('error', { message: String(error.message) }));
window.addEventListener('focus', () => observe('focus'));
window.addEventListener('pagehide', () => { observe('closed'); channel.close(); });
record().catch(error => observe('error', { message: String(error.message) }));
