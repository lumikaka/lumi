import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const workspaceSource = readFileSync(new URL('./SimpleProjectWorkspace.jsx', import.meta.url), 'utf8')
const pagesSource = readFileSync(new URL('./SimpleProjectPages.jsx', import.meta.url), 'utf8')
const storyWorkspaceSource = readFileSync(new URL('./StoryWorkspacePage.jsx', import.meta.url), 'utf8')
const overviewSource = readFileSync(new URL('./ProjectOverviewPanels.jsx', import.meta.url), 'utf8')
const draftWorkspaceSource = readFileSync(new URL('../components/DraftProjectWorkspace.jsx', import.meta.url), 'utf8')
const modeSettingSource = readFileSync(new URL('../components/ProjectDashboardModeSetting.jsx', import.meta.url), 'utf8')
const expertLayoutSource = readFileSync(new URL('../components/ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
const appPageShellSource = readFileSync(new URL('../components/AppPageShell.jsx', import.meta.url), 'utf8')
const globalSidebarSource = readFileSync(new URL('../components/GlobalSidebar.jsx', import.meta.url), 'utf8')
const styles = readFileSync(new URL('../styles/simple-project.sass', import.meta.url), 'utf8')
const projectStyles = readFileSync(new URL('../styles/projects.sass', import.meta.url), 'utf8')
const messages = readFileSync(new URL('../i18n/messages/simpleDashboard.js', import.meta.url), 'utf8')
const commonMessages = readFileSync(new URL('../i18n/messages/common.js', import.meta.url), 'utf8')

test('simple mode owns its project workspace while reusing the app sidebar, ChatArea, and shared lifecycle UI', () => {
  const combined = `${workspaceSource}\n${pagesSource}`
  const componentImports = [...combined.matchAll(/from '\.\.\/components\/([^']+)'/g)].map((match) => match[1])
  assert.deepEqual(componentImports, ['ChatArea.jsx', 'DraftProjectWorkspace.jsx', 'GlobalSidebar.jsx', 'ProjectDashboardModeSetting.jsx'])
  assert.match(workspaceSource, /<main className=\{`simple-project-shell/)
  assert.match(workspaceSource, /<GlobalSidebar/)
  assert.match(workspaceSource, /useGlobalSidebarState\(\)/)
  assert.doesNotMatch(workspaceSource, /SimpleProjectSidebar|simple-project-sidebar|SIMPLE_SIDEBAR_KEY/)
  assert.match(globalSidebarSource, /stableRecentProjects\.slice\(0, 6\)/)
  assert.match(workspaceSource, /<SimpleProjectTopbar/)
  assert.doesNotMatch(workspaceSource, /className="simple-project-topbar__nav"/)
  assert.match(workspaceSource, /<MessageCircle size=\{16\} strokeWidth=\{1\.6\}/)
  assert.match(workspaceSource, /<MoreHorizontal size=\{16\} strokeWidth=\{1\.6\}/)
  assert.match(workspaceSource, /openProjectConfiguration: true/)
  assert.match(pagesSource, /location\.state\?\.openProjectConfiguration/)
  assert.match(workspaceSource, /<ChatArea projectUuid=\{projectUuid\}/)
  assert.doesNotMatch(combined, /ProjectWorkspaceLayout|OverviewSummaryPanel|PremiseWorkspace|ChaptersWorkspace|ChapterWorkbenchPage/)
  assert.match(pagesSource, /useMutation/)
})

test('draft project setup renders one shared UI in simple and expert dashboards', () => {
  assert.match(workspaceSource, /import DraftProjectWorkspace from '\.\.\/components\/DraftProjectWorkspace\.jsx'/)
  assert.match(storyWorkspaceSource, /import DraftProjectWorkspace from '\.\.\/components\/DraftProjectWorkspace\.jsx'/)
  assert.equal((workspaceSource.match(/<DraftProjectWorkspace \/>/g) || []).length, 1)
  assert.equal((storyWorkspaceSource.match(/<DraftProjectWorkspace \/>/g) || []).length, 1)
  assert.doesNotMatch(workspaceSource, /projects\.draft\./)
  assert.doesNotMatch(storyWorkspaceSource, /projects\.draft\./)
  assert.doesNotMatch(pagesSource, /SimpleDraftProject/)
  for (const key of ['eyebrow', 'title', 'body', 'directory_hint']) {
    assert.match(draftWorkspaceSource, new RegExp(`projects\\.draft\\.${key}`))
  }
  assert.match(draftWorkspaceSource, /className="draft-project-workspace"/)
  assert.match(projectStyles, /^\.draft-project-workspace$/m)
  assert.doesNotMatch(styles, /draft-project-workspace/)
})

test('one canonical route tree selects the simple or expert renderer from project preference', () => {
  assert.match(storyWorkspaceSource, /<ProjectDashboardModeProvider key=\{projectUuid\}/)
  assert.match(storyWorkspaceSource, /canonicalProjectLocation\(/)
  assert.match(storyWorkspaceSource, /if \(simple\)[\s\S]*<SimpleProjectWorkspace/)
  for (const route of ['', 'story', 'prompts', 'llm-logs', 'exports', 'chapters', 'chapters/:chapterUuid', 'chapters/:chapterUuid/sections/:sectionUuid', 'premise', 'premise/assets/:assetUuid', 'assets']) {
    if (!route) {
      assert.match(storyWorkspaceSource, /<Route index element=\{<OverviewSummaryPanel/)
      continue
    }
    assert.ok(storyWorkspaceSource.includes(`path="${route}"`), `missing expert route ${route}`)
  }
  assert.doesNotMatch(expertLayoutSource, /writeProjectDashboardMode/)
  assert.match(expertLayoutSource, /forcedExpert[\s\S]*expert_page_notice/)
})

test('project configuration and the expert topbar expose mode switching', () => {
  assert.match(pagesSource, /<ProjectDashboardModeSetting projectUuid=\{projectUuid\} dirty=\{configurationDirty\}/)
  assert.match(overviewSource, /<ProjectDashboardModeSetting projectUuid=\{projectUuid\} dirty=\{configurationDirty\}/)
  assert.match(pagesSource, /t\('projects\.configuration'\)/)
  assert.match(overviewSource, /t\('projects\.configuration'\)/)
  assert.doesNotMatch(workspaceSource, /mode-switch|switch_to_expert|projectDashboardModeDestination/)
  assert.match(expertLayoutSource, /project-topbar__mode-switch/)
  assert.match(expertLayoutSource, /simple\.mode\.switch_to_simple/)
  assert.match(expertLayoutSource, /selectMode\(PROJECT_DASHBOARD_MODE_SIMPLE\)/)
  assert.match(expertLayoutSource, /projectRouteRequiresExpert\(location\.pathname, projectUuid\)/)
  assert.doesNotMatch(expertLayoutSource, /projectDashboardModeDestination/)
  assert.match(modeSettingSource, /useProjectDashboardMode\(\)/)
  assert.match(modeSettingSource, /updateMode\(mode\)/)
  assert.doesNotMatch(modeSettingSource, /navigate\(|projectDashboardModeDestination/)
  assert.match(modeSettingSource, /aria-pressed=\{preferredMode === mode\.value\}/)
})

test('application-level project switching is mode-neutral and resolves through the project root', () => {
  for (const source of [workspaceSource, expertLayoutSource, appPageShellSource]) {
    assert.match(source, /navigate\(`\/projects\/\$\{encodeURIComponent\(uuid\)\}`\)/)
    assert.doesNotMatch(source, /onSwitchProject=\{[^}]*overview\/summary/)
  }
})

test('every simple workflow is independently deep-linkable through the shared resource paths', () => {
  for (const route of [
    'story',
    'premise',
    'premise/assets/:assetUuid',
    'chapters',
    'chapters/:chapterUuid',
    'chapters/:chapterUuid/sections/:sectionUuid',
    'chapters/:chapterUuid/preview',
  ]) {
    assert.ok(workspaceSource.includes(`path="${route}"`), `missing simple route ${route}`)
  }
  assert.match(workspaceSource, /<Route index element=\{<SimpleHomePage/)
  assert.match(pagesSource, /chapters\.length === 1[\s\S]*chapters\/\$\{encodeURIComponent\(chapters\[0\]\.uuid\)\}/)
  assert.match(pagesSource, /const multipleChapters = \(chaptersQuery\.data\?\.items \|\| \[\]\)\.length > 1/)
  assert.match(pagesSource, /backTo=\{projectRoute\(projectUuid, multipleChapters \? 'chapters' : ''/)
  assert.doesNotMatch(`${workspaceSource}\n${pagesSource}`, /\/simple\/|simpleProjectRoute\(/)
})

test('simple pages read and mutate the shared REST facts through TanStack Query', () => {
  for (const queryFunction of ['getStoryProfile', 'getPremise', 'getPremiseAsset', 'listPremiseAssets', 'listChapters', 'getChapter', 'listComicSections']) {
    assert.match(pagesSource, new RegExp(`queryFn: \\(\\) => ${queryFunction.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\(`), queryFunction)
  }
  for (const operation of [
    'updateStoryProject', 'updateStoryProfile', 'restoreStoryProfileVersion', 'regenerateStoryMD',
    'createPremiseAsset', 'updatePremiseAsset', 'trashPremiseAsset', 'restorePremiseAsset',
    'createPremiseAssetVariant', 'generatePremiseAssetVariant', 'selectPremiseAssetVariant',
    'createChapter', 'updateChapter', 'trashChapter', 'restoreChapter', 'reorderChapters',
    'createComicSection', 'updateComicSection', 'deleteComicSection', 'reorderComicSections',
    'createStoryboard', 'setComicSectionPremiseAssets', 'generateSectionImage', 'importSectionImage',
    'selectImageVariant', 'selectStoryboard', 'generateChapterImagesBatch',
  ]) {
    assert.ok(pagesSource.includes(`${operation}(`), `missing native simple operation ${operation}`)
  }
  assert.match(pagesSource, /document\.fullscreenElement/)
  assert.match(pagesSource, /requestFullscreen/)
  assert.match(pagesSource, /catch \{[\s\S]*setFullscreen\(true\)/)
  assert.match(pagesSource, /simple-book-reader\$\{fullscreen \? ' is-fullscreen' : ''\}/)
  assert.match(pagesSource, /expected_revision/)
  assert.match(pagesSource, /SimpleFeedback/)
  assert.match(pagesSource, /SimpleConfirm/)
})

test('simple operations use realtime invalidation and never HTTP polling or expert-operation handoffs', () => {
  const combined = `${workspaceSource}\n${pagesSource}`
  assert.match(workspaceSource, /useProjectRealtimeSync\(projectUuid\)/)
  assert.doesNotMatch(combined, /refetchInterval|setInterval|expertProjectRoute|simple\.(?:read_only|\w+\.expert_action)/)
  assert.doesNotMatch(messages, /simple\.(?:read_only|\w+\.expert_action)/)
  for (const queryKey of ['story-project', 'story-profile', 'premise-assets', 'premise-asset', 'story-chapters', 'story-chapter', 'comic-sections', 'comic-storyboards', 'comic-images', 'production-tasks']) {
    assert.ok(combined.includes(`'${queryKey}'`), `missing query key ${queryKey}`)
  }
})

test('setting, chapter, and page routes provide public-UUID ChatArea context', () => {
  assert.match(workspaceSource, /queryKey: \['premise-asset', projectUuid, routeState\.assetUuid\]/)
  assert.match(workspaceSource, /queryKey: \['story-chapter', projectUuid, routeState\.chapterUuid\]/)
  assert.match(workspaceSource, /queryKey: \['comic-sections', projectUuid, routeState\.chapterUuid\]/)
  assert.match(workspaceSource, /newThreadReference=\{chatReference\}/)
  assert.match(pagesSource, /withChatReference\(location\.search, 'premise_asset', asset\.uuid/)
  assert.match(pagesSource, /withChatReference\(location\.search, 'chapter', chapter\.uuid/)
  assert.match(pagesSource, /withChatReference\(location\.search, 'comic_section', section\.uuid/)
})

test('every statically referenced simple message exists in both language dictionaries', () => {
  const keys = new Set([...`${workspaceSource}\n${pagesSource}`.matchAll(/'((?:simple)\.[a-z0-9_.]+)'/g)].map((match) => match[1]))
  for (const key of keys) assert.ok(messages.includes(`'${key}'`), `missing message ${key}`)

  const commonKeys = new Set([...`${workspaceSource}\n${pagesSource}`.matchAll(/'((?:common)\.[a-z0-9_.]+)'/g)].map((match) => match[1]))
  for (const key of commonKeys) assert.ok(commonMessages.includes(`'${key}'`), `missing common message ${key}`)
})

test('simple styles leave the global sidebar untouched, stay namespaced, and retain active-hover feedback', () => {
  const unscopedSelectors = styles.split('\n').filter((line) => line.trim() && !/^\s/.test(line) && !line.startsWith('@use') && !line.startsWith('@keyframes'))
  assert.deepEqual(unscopedSelectors, ['.simple-project-shell'])
  assert.doesNotMatch(styles, /simple-project-sidebar/)
  assert.match(styles, /&\.global-sidebar-collapsed/)
  assert.doesNotMatch(styles, /\.simple-project-topbar__nav/)
  assert.match(styles, /\.simple-project-topbar[\s\S]*display: flex[\s\S]*gap: 12px/)
  assert.match(styles, /\.simple-project-topbar__actions[\s\S]*> button[\s\S]*width: 34px/)
  assert.match(styles, /button\[aria-pressed="true"\]:hover/)
  assert.match(styles, /\.simple-page-references[\s\S]*\.is-selected:hover/)
  assert.match(styles, /&\.is-fullscreen[\s\S]*position: fixed[\s\S]*inset: 0/)
  assert.match(styles, /@media \(max-width: 1199px\)/)
  assert.match(styles, /@media \(max-width: 760px\)/)
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/)
})
