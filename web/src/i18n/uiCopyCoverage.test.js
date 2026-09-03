import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { basename, extname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { parse } from '@babel/parser'

const SOURCE_ROOT = fileURLToPath(new URL('../', import.meta.url))
const VISIBLE_ATTRIBUTES = new Set(['aria-label', 'alt', 'placeholder', 'title'])
const COPY_PROPERTIES = new Set(['description', 'label', 'title'])
const STATIC_ALLOWLIST = new Set([
  'AI',
  'API',
  'API Key',
  'ChatArea',
  'JSON',
  'KB',
  'L',
  'LLM',
  'Lumi',
  'Lumi Agent',
  'MIME',
  'Markdown',
  'SHA-256',
  'SQLite',
  'STORY.md',
  'Task UUID',
  'Thread UUID',
  'Run UUID',
  'target_uuid:',
  'UUID',
  'URL',
  'ZIP',
  'md',
  'r',
  'txt',
  'v',
])

test('user interface source has no unregistered hard-coded copy', () => {
  const files = [join(SOURCE_ROOT, 'App.jsx'), ...sourceFiles(join(SOURCE_ROOT, 'components')), ...sourceFiles(join(SOURCE_ROOT, 'pages'))]
  const violations = files.flatMap(scanFile)
  assert.deepEqual(violations, [], `Hard-coded user interface copy must use t(key):\n${violations.join('\n')}`)
})

test('user-visible copy does not expose the internal YOLO term', () => {
  const interfaceFiles = [...sourceFiles(join(SOURCE_ROOT, 'components')), ...sourceFiles(join(SOURCE_ROOT, 'pages'))]
  const messageFiles = sourceFiles(join(SOURCE_ROOT, 'i18n', 'messages'))
  const violations = [...interfaceFiles.flatMap(scanVisibleTerm), ...messageFiles.flatMap(scanMessageTerms)]
  assert.deepEqual(violations, [])
})

function sourceFiles(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = join(directory, entry.name)
    if (entry.isDirectory()) return sourceFiles(path)
    if (entry.name.includes('.test.')) return []
    return ['.js', '.jsx'].includes(extname(entry.name)) ? [path] : []
  })
}

function scanFile(path) {
  const source = readFileSync(path, 'utf8')
  const ast = parse(source, { sourceType: 'module', plugins: ['jsx'], errorRecovery: false })
  const violations = []
  walk(ast, [], (node, ancestors) => {
    if (isExempt(source, node)) return
    if (node.type === 'JSXOpeningElement' && isResponsiveIconButton(node) && !hasJSXAttribute(node, 'aria-label')) {
      violations.push(`${relativeName(path)}:${node.loc?.start.line || 1} responsive icon button is missing aria-label`)
    }
    if (node.type === 'JSXText') addIfCopy(node.value, node, 'JSX text')
    if (node.type === 'JSXAttribute' && VISIBLE_ATTRIBUTES.has(node.name?.name) && node.value?.type === 'StringLiteral') {
      addIfCopy(node.value.value, node.value, `${node.name.name} attribute`)
    }
    if ((node.type === 'StringLiteral' || node.type === 'TemplateLiteral') && isDirectVisibleExpression(node, ancestors)) {
      const value = node.type === 'StringLiteral' ? node.value : node.quasis.map((part) => part.value.cooked).join(' ')
      addIfCopy(value, node, 'JSX expression')
    }
    if (node.type === 'CallExpression' && isDialogCall(node.callee)) {
      const argument = node.arguments[0]
      if (argument?.type === 'StringLiteral') addIfCopy(argument.value, argument, 'browser dialog')
      if (argument?.type === 'TemplateLiteral') addIfCopy(argument.quasis.map((part) => part.value.cooked).join(' '), argument, 'browser dialog')
    }
    if (node.type === 'ObjectProperty' && COPY_PROPERTIES.has(propertyName(node.key)) && node.value?.type === 'StringLiteral') {
      addIfCopy(node.value.value, node.value, `${propertyName(node.key)} config`)
    }
  })
  return violations

  function addIfCopy(value, node, kind) {
    const normalized = String(value || '').replace(/\s+/g, ' ').trim()
    if (!isTranslatableCopy(normalized)) return
    violations.push(`${relativeName(path)}:${node.loc?.start.line || 1} ${kind}: ${JSON.stringify(normalized)}`)
  }
}

function scanVisibleTerm(path) {
  const source = readFileSync(path, 'utf8')
  const ast = parse(source, { sourceType: 'module', plugins: ['jsx'], errorRecovery: false })
  const violations = []
  walk(ast, [], (node, ancestors) => {
    if (node.type === 'JSXText') add(node.value, node)
    if (node.type === 'JSXAttribute' && VISIBLE_ATTRIBUTES.has(node.name?.name) && node.value?.type === 'StringLiteral') add(node.value.value, node.value)
    if ((node.type === 'StringLiteral' || node.type === 'TemplateLiteral') && isDirectVisibleExpression(node, ancestors)) {
      add(node.type === 'StringLiteral' ? node.value : node.quasis.map((part) => part.value.cooked).join(' '), node)
    }
    if (node.type === 'CallExpression' && isDialogCall(node.callee)) {
      const argument = node.arguments[0]
      if (argument?.type === 'StringLiteral') add(argument.value, argument)
      if (argument?.type === 'TemplateLiteral') add(argument.quasis.map((part) => part.value.cooked).join(' '), argument)
    }
    if (node.type === 'ObjectProperty' && COPY_PROPERTIES.has(propertyName(node.key)) && node.value?.type === 'StringLiteral') add(node.value.value, node.value)
  })
  return violations

  function add(value, node) {
    if (/\bYOLO\b/i.test(String(value || ''))) violations.push(`${relativeName(path)}:${node.loc?.start.line || 1}`)
  }
}

function scanMessageTerms(path) {
  const source = readFileSync(path, 'utf8')
  const ast = parse(source, { sourceType: 'module', errorRecovery: false })
  const violations = []
  walk(ast, [], (node) => {
    if (node.type !== 'ObjectProperty' || node.value?.type !== 'ArrayExpression') return
    for (const item of node.value.elements || []) {
      if (item?.type === 'StringLiteral' && /\bYOLO\b/i.test(item.value)) violations.push(`${relativeName(path)}:${item.loc?.start.line || 1}`)
    }
  })
  return violations
}

function isResponsiveIconButton(node) {
  if (node.name?.type !== 'JSXIdentifier' || node.name.name !== 'button') return false
  const className = node.attributes.find((attribute) => attribute.type === 'JSXAttribute' && attribute.name?.name === 'className')
  return className?.value?.type === 'StringLiteral' && className.value.value.split(/\s+/).includes('project-topbar__action')
}

function hasJSXAttribute(node, name) {
  return node.attributes.some((attribute) => attribute.type === 'JSXAttribute' && attribute.name?.name === name)
}

function walk(node, ancestors, visit) {
  if (!node || typeof node !== 'object') return
  if (typeof node.type === 'string') visit(node, ancestors)
  const nextAncestors = typeof node.type === 'string' ? [...ancestors, node] : ancestors
  for (const [key, value] of Object.entries(node)) {
    if (['comments', 'errors', 'loc', 'tokens'].includes(key)) continue
    if (Array.isArray(value)) value.forEach((child) => walk(child, nextAncestors, visit))
    else if (value && typeof value === 'object') walk(value, nextAncestors, visit)
  }
}

function isDirectVisibleExpression(node, ancestors) {
  const containerIndex = ancestors.findLastIndex((ancestor) => ancestor.type === 'JSXExpressionContainer')
  if (containerIndex < 0) return false
  const attribute = ancestors.findLast((ancestor) => ancestor.type === 'JSXAttribute')
  if (attribute && !VISIBLE_ATTRIBUTES.has(attribute.name?.name)) return false
  return ['ConditionalExpression', 'JSXExpressionContainer'].includes(ancestors.at(-1)?.type)
}

function isDialogCall(callee) {
  if (callee?.type === 'Identifier') return ['alert', 'confirm', 'prompt'].includes(callee.name)
  return callee?.type === 'MemberExpression'
    && callee.object?.type === 'Identifier'
    && ['window', 'globalThis'].includes(callee.object.name)
    && ['alert', 'confirm', 'prompt'].includes(propertyName(callee.property))
}

function propertyName(node) {
  if (node?.type === 'Identifier') return node.name
  if (node?.type === 'StringLiteral') return node.value
  return ''
}

function isTranslatableCopy(value) {
  if (!value || !/[\p{L}]/u.test(value)) return false
  if (STATIC_ALLOWLIST.has(value)) return false
  if (/^(?:https?:\/\/|data:|\/api\/|\/(?:[\w .-]+\/)*[\w .-]+|[\w.-]+\.(?:md|json|sqlite|png|jpe?g|webp|gif|svg))\S*$/i.test(value)) return false
  if (/^(?:[a-z0-9]+[-_.:/])+[a-z0-9*]+$/i.test(value)) return false
  return true
}

function isExempt(source, node) {
  const line = node.loc?.start.line || 1
  const lines = source.split('\n')
  return [lines[line - 1], lines[line - 2]].some((value) => value?.includes('i18n-exempt'))
}

function relativeName(path) {
  const marker = `${basename(SOURCE_ROOT)}/`
  const index = path.lastIndexOf(marker)
  return index >= 0 ? path.slice(index) : path
}
