import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const page = readFileSync(new URL('./ThreadTrajectoryPage.jsx', import.meta.url), 'utf8')
const ledger = readFileSync(new URL('./TrajectoryLedger.jsx', import.meta.url), 'utf8')
const inspector = readFileSync(new URL('./TrajectoryInspector.jsx', import.meta.url), 'utf8')
const stats = readFileSync(new URL('./TrajectoryStats.jsx', import.meta.url), 'utf8')
const timeline = readFileSync(new URL('./TrajectoryTimeline.jsx', import.meta.url), 'utf8')
const workspace = readFileSync(new URL('../StoryWorkspacePage.jsx', import.meta.url), 'utf8')
const simpleWorkspace = readFileSync(new URL('../SimpleProjectWorkspace.jsx', import.meta.url), 'utf8')
const chatArea = readFileSync(new URL('../../components/ChatArea.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../../styles/trajectory.sass', import.meta.url), 'utf8')
const simpleStyles = readFileSync(new URL('../../styles/simple-project.sass', import.meta.url), 'utf8')

test('trajectory registers one shared project URL and only the mode-specific topbar changes', () => {
  assert.match(workspace, /threads\/:threadUuid\/trajectory["']\s+element=\{<ThreadTrajectoryPage/)
  assert.match(workspace, /hideChat=\{chapterPreview \|\| trajectoryView\}/)
  assert.match(workspace, /!trajectoryView \? <WorkspaceGroupTabs/)
  assert.match(simpleWorkspace, /import ThreadTrajectoryPage from '\.\/trajectory\/ThreadTrajectoryPage\.jsx'/)
  assert.match(simpleWorkspace, /trajectoryView = routeState\.key === 'trajectory'/)
  assert.match(simpleWorkspace, /threads\/:threadUuid\/trajectory["']\s+element=\{<ThreadTrajectoryPage/)
  assert.match(simpleWorkspace, /hideChat=\{trajectoryView\}/)
  assert.match(simpleWorkspace, /!compact && !trajectoryView \? <ChatArea/)
  assert.match(simpleStyles, /simple-project-workbench--solo[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/)
})

test('simple topbar stacking context stays above the trajectory toolbar', () => {
  const topbar = simpleStyles.slice(simpleStyles.indexOf('  .simple-project-topbar\n'), simpleStyles.indexOf('  .simple-project-topbar__context'))
  const toolbar = styles.slice(styles.indexOf('.trajectory-toolbar\n'), styles.indexOf('.trajectory-toolbar__heading'))
  const topbarZ = Number(topbar.match(/z-index:\s*(\d+)/)?.[1])
  const toolbarZ = Number(toolbar.match(/z-index:\s*(\d+)/)?.[1])
  assert.match(topbar, /position:\s*relative/)
  assert.ok(topbarZ > toolbarZ, `topbar z-index ${topbarZ} must exceed trajectory toolbar z-index ${toolbarZ}`)
})

test('direct item_uuid selection loads an anchored page and restores ledger selection', () => {
  assert.match(page, /useSearchParams\(\)/)
  assert.match(page, /initialItemUuidRef = useRef\(searchParams\.get\('item_uuid'\)/)
  assert.match(page, /getChatTrajectory\(projectUuid, threadUuid,[\s\S]*?itemUuid:/)
  assert.match(page, /next\.set\('item_uuid', sourceUuid\)/)
  assert.match(page, /next\.set\('item_kind', sourceKind\)/)
  assert.match(page, /'chat-trajectory'[\s\S]*?'anchor'[\s\S]*?itemUuid: selectedUuid/)
  assert.match(page, /scrollIntoView\(\{ block: 'center' \}\)/)
})

test('ledger uses stable source keys and Inspector provides tabs plus pointer resize', () => {
  assert.match(ledger, /key=\{entry\.key\}/)
  assert.match(ledger, /aria-rowindex=\{row\.ariaRowIndex\}/)
  assert.doesNotMatch(ledger, /key=\{index\}/)
  assert.match(inspector, /\['summary', 'payload', 'result', 'timing', 'raw'\]/)
  assert.match(inspector, /onPointerDown=\{onResizeStart\}/)
  assert.match(page, /window\.addEventListener\('pointermove', resize\)/)
})

test('Tool summary exposes highlighted Payload and Response without raw HTML execution', () => {
  assert.match(inspector, /selected\.kind === 'tool'[\s\S]*?trajectory-tool-summary/)
  assert.match(inspector, /MachineValue title=\{t\('trajectory\.inspector\.payload'\)\} value=\{selected\.input\}/)
  assert.match(inspector, /MachineValue title=\{t\('trajectory\.inspector\.result'\)\} value=\{selected\.output \?\? notRecorded\}/)
  assert.match(inspector, /jsonSyntaxSegments\(value\)/)
  assert.match(inspector, /trajectory-json-token--\$\{segment\.kind\}/)
  assert.doesNotMatch(inspector, /dangerouslySetInnerHTML/)
})

test('ChatArea Thread detail exposes a safe real trajectory href in a new tab', () => {
  assert.match(chatArea, /function threadTrajectoryHref[\s\S]*?`\/projects\/\$\{encodeURIComponent\(projectUuid\)\}\/threads\/\$\{encodeURIComponent\(threadUuid\)\}\/trajectory`/)
  assert.match(chatArea, /href=\{threadTrajectoryHref\(projectUuid, selectedThread\.uuid\)\}/)
  assert.match(chatArea, /target="_blank"/)
  assert.match(chatArea, /rel="noopener noreferrer"/)
})

test('trajectory selected controls retain a later combined hover rule', () => {
  const selected = styles.indexOf('&[aria-pressed="true"]')
  const combined = styles.indexOf('&[aria-pressed="true"]:hover')
  assert.ok(selected >= 0)
  assert.ok(combined > selected)
})

test('timeline defaults to duration and exposes every scale with ticks and user-wait semantics', () => {
  assert.match(timeline, /timelineModes = \['duration', 'time', 'actual', 'sequence'\]/)
  assert.match(timeline, /useState\('duration'\)/)
  assert.match(timeline, /trajectoryTimelineTicks\(activeView, mode\)/)
  assert.match(timeline, /data-activity=\{item\.activity\}/)
  assert.match(timeline, /trajectory-timeline__wait-legend/)
  assert.match(styles, /\.trajectory-timeline__mode-switch[\s\S]*?\&\[aria-pressed="true"\]:hover/)
  assert.match(styles, /\.trajectory-timeline__axis/)
  assert.match(styles, /\.trajectory-timeline__span\[data-activity="user_wait"\]/)
})

test('trajectory UI exposes loaded-history search and independent collapse controls', () => {
  const toolbar = readFileSync(new URL('./TrajectoryToolbar.jsx', import.meta.url), 'utf8')
  assert.match(toolbar, /type="search"/)
  assert.match(toolbar, /search && !projection\.historyComplete/)
  assert.match(ledger, /collapsedTurns/)
  assert.match(ledger, /collapsedToolGroups/)
  assert.match(ledger, /trajectory-tool-group-toggle/)
  assert.match(ledger, /trajectory-turn-rail/)
  assert.match(ledger, /trajectory-turn-label/)
  assert.match(ledger, /trajectory-request-boundary/)
  assert.match(inspector, /onRequestDetailLoaded/)
})

test('trajectory infinite query declares both TanStack Query v5 page directions', () => {
  assert.match(page, /getPreviousPageParam:/)
  assert.match(page, /getNextPageParam:\s*\(\) => undefined/)
})

test('trajectory uses the full solo workspace and only reserves Inspector width after selection', () => {
  assert.match(styles, /\.project-workbench\.project-workbench--solo[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/)
  assert.match(page, /trajectory-workbench--inspector-open/)
  assert.match(styles, /\.trajectory-workbench--inspector-open[\s\S]*?var\(--trajectory-inspector-width/)
  assert.match(styles, /\.trajectory-inspector--empty[\s\S]*?display:\s*none/)
  assert.match(page, /stored == null \|\| stored === ''/)
})

test('trajectory keeps a compact whole-Thread stats line at the bottom', () => {
  assert.match(page, /<TrajectoryStats overview=\{projection\.overview\} \/>/)
  assert.match(stats, /trajectoryStatsGroups\(overview, t\)/)
  assert.match(stats, /trajectory-stats__separator/)
  assert.match(styles, /grid-template-rows:\s*auto auto auto minmax\(0, 1fr\) auto/)
  assert.match(styles, /\.trajectory-stats[\s\S]*?text-overflow:\s*ellipsis/)
})
