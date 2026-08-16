import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ChatArea.jsx', import.meta.url), 'utf8')
const projectThreadsSource = readFileSync(new URL('../pages/projectThreads.js', import.meta.url), 'utf8')

test('new thread opens a composer draft and persists the first message without a title form', () => {
  assert.match(source, /function NewThreadDraft[\s\S]*?<ChatComposer/)
  assert.match(source, /if \(showCreate\)[\s\S]*?<NewThreadDraft/)
  assert.match(source, /const createThreadMutation = useMutation\(\{[\s\S]*?suggestedChatThreadTitle\(text\)[\s\S]*?createChatTurn\(projectUuid, thread\.uuid/)
  assert.doesNotMatch(source, /NewThreadDetail|chat\.thread\.title_field/)
  assert.match(source, /title: suggestedChatThreadTitle\(text\),[\s\S]*?scope: 'project'/)
  assert.doesNotMatch(source, /requestedScope/)
})

test('image scenes upload references and block sends until every attachment is ready', () => {
  assert.match(source, /purpose: 'project_chatbot_reference'/)
  assert.match(source, /attachmentBlocked = attachments\.some\([\s\S]*?'uploading'[\s\S]*?'error'/)
  assert.match(source, /readyProjectChatUploadUUIDs\(attachments\)/)
  assert.match(source, /<SceneThreadDraft[\s\S]*?attachments=\{attachments\}[\s\S]*?onPaste=\{handleAttachmentPaste\}/)
  assert.match(source, /<ChatComposer[\s\S]*?scene=\{selectedThread\?\.scene\}[\s\S]*?onAddFiles=\{addAttachmentFiles\}/)
  assert.match(source, /URL\.createObjectURL\(file\)/)
  assert.match(source, /function releaseAttachmentPreview[\s\S]*?URL\.revokeObjectURL/)
  assert.match(source, /const retryAttachment[\s\S]*?uploadAttachmentDraft\(draft\)/)
  assert.match(source, /const createSceneThreadMutation[\s\S]*?catch \(turnError\)[\s\S]*?return \{ thread, turnError \}[\s\S]*?setQueryData\(\['chat-thread'/)
})

test('running image and current-project API tools expose method-specific progress copy', () => {
  assert.match(source, /tool_name === 'image_gen'[\s\S]*?chat\.activity\.image_gen/)
  assert.match(source, /tool_name === 'request_current_project_api'[\s\S]*?currentProjectAPIActivityKey/)
  assert.match(source, /GET: 'chat\.activity\.asset_read'[\s\S]*?POST: 'chat\.activity\.asset_create'[\s\S]*?PATCH: 'chat\.activity\.asset_update'[\s\S]*?DELETE: 'chat\.activity\.asset_trash'/)
  assert.match(source, /\['create_premise_asset', 'update_premise_asset'\][\s\S]*?chat\.activity\.writeback/)
  assert.match(source, /latestItem\?\.item_type === 'tool_result'[\s\S]*?chat\.activity\.finalizing/)
})

test('second-stage chat parity includes safe markdown, paged history, queue steering and diagnostics', () => {
  assert.match(source, /<SafeMarkdown value=\{item\.content\}/)
  assert.match(source, /const threadsQuery = useProjectThreads\(projectUuid, expanded\)/)
  assert.match(projectThreadsSource, /queryKey: projectThreadsQueryKey\(projectUuid\)/)
  assert.match(projectThreadsSource, /return \['chat-threads', projectUuid, 'pages'\]/)
  assert.match(projectThreadsSource, /listChatThreads\(projectUuid, \{ page: pageParam, perPage: PROJECT_THREADS_PAGE_SIZE \}\)/)
  assert.match(source, /const itemsQuery = useInfiniteQuery\(\{[\s\S]*?queryKey: \['chat-items', projectUuid, selectedThreadUuid, 'pages'\]/)
  assert.match(source, /const eventsQuery = useInfiniteQuery\(\{[\s\S]*?queryKey: \['chat-events', projectUuid, selectedThreadUuid, 'pages'\]/)
  assert.match(source, /const eventsQuery = useInfiniteQuery\(\{[\s\S]*?'chat-events'[\s\S]*?listChatEvents\(projectUuid, selectedThreadUuid, \{ after: pageParam, limit: 100 \}\)/)
  assert.match(source, /<ThreadEventDiagnostics events=\{events\}[\s\S]*?eventsQuery\.fetchNextPage\(\)/)
  assert.match(source, /function ThreadEventDiagnostics[\s\S]*?prettyDiagnosticJSON\(event\.payload\)/)
  assert.match(projectThreadsSource, /PROJECT_THREADS_PAGE_SIZE = 20/)
  assert.match(source, /const MESSAGE_PAGE_LIMIT = 30/)
  assert.match(source, /rootMargin: '48px 0px'[\s\S]*?threshold: 0\.01/)
  assert.match(source, /isAutoLoadingRef\.current = true[\s\S]*?onLoadMore\(\)/)
  assert.match(source, /chatThreadCountLabel\(threads\.length, total\)/)
  assert.match(source, /onScroll=\{handleMessagesScroll\}/)
  assert.match(source, /isLoadingEarlierRef\.current = true[\s\S]*?finally[\s\S]*?isLoadingEarlierRef\.current = false/)
  assert.match(source, /captureChatScrollAnchor\(messagesRef\.current\)[\s\S]*?itemsQuery\.fetchPreviousPage\(\)/)
  assert.match(source, /restoreChatScrollAnchor\(messagesRef\.current, anchor\)/)
  assert.match(source, /draggable=\{!pending[\s\S]*?aria-grabbed/)
  assert.match(source, /steerFollowUp\(projectUuid, selectedThreadUuid, uuid\)/)
  assert.match(source, /function WorkflowDiagnostics[\s\S]*?'workflow-runs'[\s\S]*?'workflow-events'[\s\S]*?'workflow-llm-logs'/)
  assert.match(source, /openStepDiagnostics\(step\.uuid\)[\s\S]*?focusStepUuid=\{diagnosticStepUuid\}/)
  assert.match(source, /listWorkflowLLMLogs\(projectUuid, workflow\.uuid, \{ page: pageParam, perPage: 10, stepUuid: focusStepUuid \}\)/)
  assert.match(source, /workflow-diagnostics__step-detail[\s\S]*?prettyDiagnosticJSON\(focusedStep\.input\)[\s\S]*?prettyDiagnosticJSON\(focusedStep\.output\)/)
})

test('workflow diagnostics load only while open and rely on project realtime reconciliation', () => {
  const diagnostics = source.match(/function WorkflowDiagnostics[\s\S]*?\n}\n\nfunction ThreadEventDiagnostics/)?.[0] || ''
  assert.ok(diagnostics)
  assert.equal((diagnostics.match(/enabled: open/g) || []).length, 3)
  assert.equal((diagnostics.match(/refetchOnWindowFocus: false/g) || []).length, 3)
  assert.equal((diagnostics.match(/refetchOnReconnect: false/g) || []).length, 3)
  assert.doesNotMatch(diagnostics, /refetchInterval|setInterval|setTimeout/)
  assert.match(diagnostics, /runsQuery\.fetchPreviousPage\(\)/)
  assert.match(diagnostics, /eventsQuery\.fetchPreviousPage\(\)/)
  assert.match(diagnostics, /logsQuery\.fetchNextPage\(\)/)
})
