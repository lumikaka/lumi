import assert from 'node:assert/strict'
import test from 'node:test'

import { jsonSyntaxSegments, safeMachineJSON } from './trajectoryMachineValue.js'

test('safeMachineJSON parses JSON strings and formats structured values', () => {
  assert.equal(safeMachineJSON('{"name":"Lumi","count":2}'), '{\n  "name": "Lumi",\n  "count": 2\n}')
  assert.equal(safeMachineJSON({ ok: true }), '{\n  "ok": true\n}')
  assert.equal(safeMachineJSON(null), '—')
})

test('jsonSyntaxSegments classifies JSON keys and primitive values', () => {
  const segments = jsonSyntaxSegments({ name: 'Lumi', count: 2, ok: true, missing: null })
  assert.ok(segments.some((segment) => segment.kind === 'key' && segment.text === '"name"'))
  assert.ok(segments.some((segment) => segment.kind === 'string' && segment.text === '"Lumi"'))
  assert.ok(segments.some((segment) => segment.kind === 'number' && segment.text === '2'))
  assert.ok(segments.some((segment) => segment.kind === 'boolean' && segment.text === 'true'))
  assert.ok(segments.some((segment) => segment.kind === 'null' && segment.text === 'null'))
})

test('non-JSON user content remains inert plain text', () => {
  const source = '<img src=x onerror=alert(1)>'
  assert.equal(jsonSyntaxSegments(source).map((segment) => segment.text).join(''), source)
})
