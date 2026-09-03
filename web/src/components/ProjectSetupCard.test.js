import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

test('project setup card renders REST-backed draft values and disappears after finalization', () => {
  const source = readFileSync(new URL('./ProjectSetupCard.jsx', import.meta.url), 'utf8')
  const chat = readFileSync(new URL('./ChatArea.jsx', import.meta.url), 'utf8')
  const styles = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')
  assert.match(source, /queryKey: projectQueryKeys\.setup\(projectUuid\)/)
  assert.match(source, /queryFn: \(\) => getProjectSetup\(projectUuid\)/)
  assert.doesNotMatch(source, /refetchInterval|setInterval|setTimeout/)
  assert.match(source, /setup\.draft_values/)
  assert.doesNotMatch(source, /setup\.candidate/)
  for (const field of ['project_name', 'generation_language', 'overall_style', 'generation_brief', 'format', 'aspect_ratio', 'large_image_minimal_text', 'interaction_mode', 'comic_layout']) assert.match(source, new RegExp(field))
  for (const provenance of ['system_default', 'agent_proposed', 'user_confirmed']) assert.match(source, new RegExp(provenance))
  assert.match(source, /missing_information/)
  assert.match(source, /if \(!setup \|\| setup\.setup_status !== 'draft'\) return null/)
  assert.doesNotMatch(source, /finalized/)
  assert.match(source, /aria-pressed=\{activeField === field\.key\}/)
  assert.match(source, /setup\.reference_plan\?\.items\?\.length/)
  assert.match(source, /chat\.setup\.reference\.auto_hint/)
  assert.match(source, /setup\.reference_plan\.items\.length/)
  assert.doesNotMatch(source, /updateProjectSetupReference|useMutation|<select|<textarea|include_in_yolo|reference_role|plan_source/)
  assert.doesNotMatch(styles, /project-setup-reference__(?:include|save|instruction|readonly|error)/)
  assert.match(chat, /<ProjectSetupCard projectUuid=\{projectUuid\} enabled=\{expanded && Boolean\(selectedThreadUuid\)\} \/>/)
})
