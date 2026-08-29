import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

test('project setup card renders REST-backed draft values, provenance, missing fields and lifecycle', () => {
  const source = readFileSync(new URL('./ProjectSetupCard.jsx', import.meta.url), 'utf8')
  const chat = readFileSync(new URL('./ChatArea.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')
  assert.match(source, /queryKey: projectQueryKeys\.setup\(projectUuid\)/)
  assert.match(source, /queryFn: \(\) => getProjectSetup\(projectUuid\)/)
  assert.doesNotMatch(source, /refetchInterval|setInterval|setTimeout/)
  assert.match(source, /setup\.draft_values/)
  assert.doesNotMatch(source, /setup\.candidate/)
  for (const field of ['project_name', 'generation_language', 'overall_style', 'format', 'aspect_ratio', 'large_image_minimal_text', 'interaction_mode', 'comic_layout']) assert.match(source, new RegExp(field))
  for (const provenance of ['system_default', 'agent_proposed', 'user_confirmed']) assert.match(source, new RegExp(provenance))
  assert.match(source, /missing_information/)
  assert.match(source, /aria-pressed=\{activeField === field\.key\}/)
  assert.match(styles, /\&\[aria-pressed="true"\]:hover/)
  assert.match(chat, /<ProjectSetupCard projectUuid=\{projectUuid\} enabled=\{expanded && Boolean\(selectedThreadUuid\)\} \/>/)
})
