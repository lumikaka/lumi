import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ChatArea.jsx', import.meta.url), 'utf8')
const stylesSource = readFileSync(new URL('../styles/chat.sass', import.meta.url), 'utf8')
const projectThreadsSource = readFileSync(new URL('../pages/projectThreads.js', import.meta.url), 'utf8')
const messagesSource = readFileSync(new URL('../i18n/messages/chat.js', import.meta.url), 'utf8')

test('new thread opens a composer draft and persists the first message without a title form', () => {
  assert.match(source, /function NewThreadDraft[\s\S]*?<ChatComposer/)
  assert.match(source, /if \(showCreate\)[\s\S]*?<NewThreadDraft/)
  assert.match(source, /const createThreadMutation = useMutation\(\{[\s\S]*?suggestedChatThreadTitle\(text\)[\s\S]*?createChatTurn\(projectUuid, thread\.uuid/)
  assert.doesNotMatch(source, /NewThreadDetail|chat\.thread\.title_field/)
  assert.match(source, /createChatThread\(projectUuid, \{[\s\S]*?title: suggestedChatThreadTitle\(text\),[\s\S]*?\}\)/)
  assert.doesNotMatch(source, /requestedScope|subject_uuid|chat_scene|chat_scope/)
})

test('new threads opened from a chapter route start with that Chapter Reference', () => {
  const layoutSource = readFileSync(new URL('./ProjectWorkspaceLayout.jsx', import.meta.url), 'utf8')
  assert.match(layoutSource, /projectChapterUuidFromPath\(location\.pathname, projectUuid\)/)
  assert.match(layoutSource, /resource_type: 'chapter'/)
  assert.match(layoutSource, /<ChatArea[\s\S]*?newThreadReference=\{newThreadReference\}/)
  assert.match(source, /const startNewThread[\s\S]*?return newThreadReference \? \[newThreadReference\] : \[\]/)
  assert.match(source, /appendProjectChatReference\(current, newThreadReference\)/)
  assert.match(source, /\['premise_asset', 'chapter', 'comic_section', 'file'\]/)
})

test('entry Reference parameters are consumed once so cancellation survives remounts and explicit re-entry works', () => {
  assert.match(source, /setSearchParams\(consumeProjectChatReferenceQuery\(searchParams\), \{ replace: true \}\)/)
  assert.doesNotMatch(source, /seededReferenceKeyRef/)
})

test('every composer accepts finalized files and domain references without scene gating', () => {
  assert.match(source, /purpose: 'project_chatbot_reference'/)
  assert.match(source, /finalizeAssetUpload\(projectUuid, upload\.uuid, 'project_chatbot_reference'\)/)
  assert.match(source, /referenceBlocked = references\.some\([\s\S]*?'uploading'[\s\S]*?'error'/)
  assert.match(source, /readyProjectChatReferences\(references\)/)
  assert.match(source, /<NewThreadDraft[\s\S]*?references=\{references\}[\s\S]*?onPaste=\{handleAttachmentPaste\}/)
  assert.match(source, /<ChatComposer[\s\S]*?references=\{references\}[\s\S]*?onAddFiles=\{addAttachmentFiles\}/)
  assert.match(source, /<ReferencePicker[\s\S]*?<AttachmentPicker/)
  assert.match(source, /<ReferenceStrip projectUuid=\{projectUuid\} references=\{item\.references \|\| \[\]\}/)
  assert.match(source, /URL\.createObjectURL\(file\)/)
  assert.match(source, /function releaseAttachmentPreview[\s\S]*?URL\.revokeObjectURL/)
  assert.match(source, /const createThreadMutation[\s\S]*?catch \(turnError\)[\s\S]*?return \{ thread, turnError \}[\s\S]*?setQueryData\(\['chat-thread'/)
  assert.doesNotMatch(source, /SceneThreadDraft|canProjectChatAttachImages|upload_uuids|image_references/)
})

test('running image and current-project API tools expose method-specific progress copy', () => {
  assert.match(source, /toolVariables = \{ tool_name: runningTool\?\.toolName \|\| 'controlled_tool' \}[\s\S]*?toolName === 'image_gen'[\s\S]*?chat\.activity\.image_gen', toolVariables/)
  assert.match(source, /\['request_api', 'request_current_project_api'\]\.includes\(runningTool\.toolName\)[\s\S]*?currentProjectAPIActivityKey/)
  assert.match(source, /GET: 'chat\.activity\.asset_read'[\s\S]*?POST: 'chat\.activity\.asset_create'[\s\S]*?PATCH: 'chat\.activity\.asset_update'[\s\S]*?DELETE: 'chat\.activity\.asset_trash'/)
  assert.match(source, /if \(activityKey\) return t\(activityKey, toolVariables\)/)
  assert.match(source, /\['create_premise_asset', 'update_premise_asset'\][\s\S]*?chat\.activity\.writeback', toolVariables/)
  assert.match(source, /chat\.activity\.tool_running[\s\S]*?chat\.activity\.finalizing/)
  for (const key of ['image_gen', 'asset_read', 'asset_create', 'asset_update', 'asset_trash', 'writeback', 'tool_running']) {
    assert.match(messagesSource, new RegExp(`'chat\\.activity\\.${key}': \\[.*?\\{tool_name\\}`), key)
  }
})

test('terminal turns collapse paired tool activity while keeping raw diagnostics available', () => {
  const chatItem = source.match(/function ChatItem[\s\S]*?\n}\n\nfunction ToolActivitySummary/)?.[0] || ''
  assert.ok(chatItem)
  assert.doesNotMatch(chatItem, /item\.item_type === 'tool_call'|item\.item_type === 'tool_result'/)
  assert.match(source, /function ToolActivitySummary[\s\S]*?<details className="chat-tool-summary">/)
  assert.match(source, /function ToolActivitySummary[\s\S]*?chatTurnDurationLabel\(turn, t, formatNumber\)[\s\S]*?<span>\{duration \|\| t\('chat\.tool\.summary\.title'\)\}<\/span>[\s\S]*?<ChevronRight/)
  assert.doesNotMatch(source.match(/function ToolActivitySummary[\s\S]*?\n}\n\nfunction chatTurnDurationLabel/)?.[0] || '', /TerminalSquare|chat\.tool\.summary\.expand_hint|turnStatus/)
  assert.match(stylesSource, /\.chat-tool-summary[\s\S]*?width: 100%[\s\S]*?> summary[\s\S]*?border-bottom: 1px solid \$color-border-subtle/)
  assert.match(source, /activity\.tools\.map[\s\S]*?<details>[\s\S]*?<ToolActivityPayload label=\{t\('chat\.tool\.arguments'\)/)
  assert.match(source, /projectChatTurnActivity\(turn, group\.items, \{ historyMayBePartial \}\)/)
  assert.match(source, /index === activity\.summaryIndex[\s\S]*?<ChatItem item=\{item\}/)
  assert.match(source, /historyMayBePartial=\{Boolean\(index === 0 && itemsQuery\.hasNextPage && !group\.items\.some/)
  assert.equal((source.match(/aria-live="polite"/g) || []).length, 1)
  assert.match(messagesSource, /'chat\.tool\.summary\.title': \['工具活动', 'Tool activity'\]/)
  assert.doesNotMatch(messagesSource, /使用了 \{count\} 项工具|Used \{count\} tools/)
  assert.match(messagesSource, /'chat\.turn\.duration\.minutes_seconds': \['耗时 \{minutes\} 分 \{seconds\} 秒', 'Took \{minutes\}m \{seconds\}s'\]/)
  assert.match(messagesSource, /'chat\.activity\.tool_running': \['正在调用 \{tool_name\}…', 'Calling \{tool_name\}…'\]/)
})

test('answered user input collapses in place while pending input stays interactive', () => {
  assert.match(source, /function UserInputCard[\s\S]*?projectChatUserInput\(request\)[\s\S]*?presentation\.mode !== 'pending'[\s\S]*?<UserInputHistory/)
  assert.match(source, /function PendingUserInputCard[\s\S]*?<form className="chat-input-request"[\s\S]*?presentation\.questions\.map[\s\S]*?selected_option_uuid[\s\S]*?onRespond\(request\.uuid/)
  assert.match(source, /function UserInputHistory[\s\S]*?<details className="chat-input-history">[\s\S]*?presentation\.questions\.map[\s\S]*?chat\.input\.final_answer/)
  assert.match(source, /function OptionCopy[\s\S]*? \(Recommended\)[\s\S]*?chat-input-request__recommended/)
  assert.match(source, /chat-message--user-input-incomplete/)
  assert.match(messagesSource, /'chat\.input\.answered_summary': \['已选择：\{answer\}', 'Selected: \{answer\}'\]/)
  assert.match(messagesSource, /'chat\.input\.answered_question_count': \['已完成 \{count\} 个问题', 'Answered \{count\} questions'\]/)
  assert.match(messagesSource, /'chat\.input\.incomplete_summary': \['未完成选择', 'Choice not completed'\]/)
  assert.match(messagesSource, /'chat\.input\.recommended': \['推荐', 'Recommended'\]/)
})

test('second-stage chat parity includes safe markdown, paged history, queue steering and workflow diagnostics', () => {
  assert.match(source, /<SafeMarkdown value=\{item\.content\}/)
  assert.match(source, /const threadsQuery = useProjectThreads\(projectUuid, expanded\)/)
  assert.match(projectThreadsSource, /queryKey: projectThreadsQueryKey\(projectUuid\)/)
  assert.match(projectThreadsSource, /return \['chat-threads', projectUuid, 'pages'\]/)
  assert.match(projectThreadsSource, /listChatThreads\(projectUuid, \{ page: pageParam, perPage: PROJECT_THREADS_PAGE_SIZE \}\)/)
  assert.match(source, /const itemsQuery = useInfiniteQuery\(\{[\s\S]*?queryKey: \['chat-items', projectUuid, selectedThreadUuid, 'pages'\]/)
  assert.match(source, /queryKey: \['chat-items'[\s\S]*?getPreviousPageParam: \(\) => undefined,[\s\S]*?getNextPageParam: \(lastPage\) => lastPage\.cursor_pagination\?\.has_more \? lastPage\.cursor_pagination\.prev_cursor : undefined/)
  assert.match(projectThreadsSource, /PROJECT_THREADS_PAGE_SIZE = 20/)
  assert.match(source, /const MESSAGE_PAGE_LIMIT = 30/)
  assert.match(source, /rootMargin: '48px 0px'[\s\S]*?threshold: 0\.01/)
  assert.match(source, /isAutoLoadingRef\.current = true[\s\S]*?onLoadMore\(\)/)
  assert.match(source, /chatThreadCountLabel\(threads\.length, total\)/)
  assert.match(source, /onScroll=\{handleMessagesScroll\}/)
  assert.match(source, /isLoadingEarlierRef\.current = true[\s\S]*?finally[\s\S]*?isLoadingEarlierRef\.current = false/)
  assert.match(source, /captureChatScrollAnchor\(messagesRef\.current\)[\s\S]*?itemsQuery\.fetchNextPage\(\)/)
  assert.match(source, /restoreChatScrollAnchor\(messagesRef\.current, anchor\)/)
  assert.match(source, /shouldAutofillEarlierChatItems\(\{[\s\S]*?scrollHeight: container\.scrollHeight,[\s\S]*?clientHeight: container\.clientHeight/)
  assert.match(source, /itemsQuery\.isFetchingNextPage \? <div className="chat-history-loader" role="status"><span>\{t\('chat\.messages\.loading_earlier'\)\}<\/span><\/div> : null/)
  assert.doesNotMatch(source, /onClick=\{loadEarlierMessages\}|chat\.messages\.load_earlier/)
  assert.doesNotMatch(messagesSource, /chat\.messages\.load_earlier/)
  assert.match(source, /draggable=\{!pending[\s\S]*?aria-grabbed/)
  assert.match(source, /steerFollowUp\(projectUuid, selectedThreadUuid, uuid\)/)
  assert.match(source, /function WorkflowDiagnostics[\s\S]*?'workflow-runs'[\s\S]*?'workflow-events'[\s\S]*?'workflow-llm-logs'/)
  assert.match(source, /openStepDiagnostics\(step\.uuid\)[\s\S]*?focusStepUuid=\{diagnosticStepUuid\}/)
  assert.match(source, /listWorkflowLLMLogs\(projectUuid, workflow\.uuid, \{ page: pageParam, perPage: 10, stepUuid: focusStepUuid \}\)/)
  assert.match(source, /workflow-diagnostics__step-detail[\s\S]*?prettyDiagnosticJSON\(focusedStep\.input\)[\s\S]*?prettyDiagnosticJSON\(focusedStep\.output\)/)
})

test('workflow diagnostics load only while open and rely on project realtime reconciliation', () => {
  const diagnostics = source.match(/function WorkflowDiagnostics[\s\S]*?\n}\n\nfunction FollowUpQueue/)?.[0] || ''
  assert.ok(diagnostics)
  assert.equal((diagnostics.match(/enabled: open/g) || []).length, 3)
  assert.equal((diagnostics.match(/refetchOnWindowFocus: false/g) || []).length, 3)
  assert.equal((diagnostics.match(/refetchOnReconnect: false/g) || []).length, 3)
  assert.doesNotMatch(diagnostics, /refetchInterval|setInterval|setTimeout/)
  assert.match(diagnostics, /runsQuery\.fetchPreviousPage\(\)/)
  assert.match(diagnostics, /eventsQuery\.fetchPreviousPage\(\)/)
  assert.match(diagnostics, /logsQuery\.fetchNextPage\(\)/)
})

test('inline workflows render inside their origin turn without replacing conversation thread identity', () => {
	assert.match(source, /groupInlineWorkflowsByTurn\(workflows, selectedThreadUuid\)/)
	assert.match(source, /workflows: inlineWorkflowsByTurn\.get\(group\.uuid\) \|\| \[\]/)
	assert.match(source, /function TurnGroup[\s\S]*?chat-turn__workflows[\s\S]*?<WorkflowProgress[\s\S]*?inline/)
	assert.match(source, /workflow-progress--inline/)
	assert.match(source, /selectedDedicatedWorkflow[\s\S]*?threadDisplayTitle\(selectedThread, selectedDedicatedWorkflow, t\)/)
	assert.match(source, /\['queued', 'in_progress', 'waiting_for_input', 'waiting_for_workflow'\]/)
	assert.match(source, /chat\.composer\.waiting_for_workflow/)
	assert.match(messagesSource, /'chat\.activity\.waiting_for_workflow': \['工作流正在后台执行/)
	assert.match(stylesSource, /\.workflow-progress--inline[\s\S]*?margin: 0/)
})

test('thread detail replaces raw runtime events with a real Trajectory link', () => {
  assert.doesNotMatch(source, /ThreadEventDiagnostics|listChatEvents|\['chat-events', projectUuid, selectedThreadUuid/)
  assert.match(source, /function threadTrajectoryHref[\s\S]*?encodeURIComponent\(projectUuid\)[\s\S]*?encodeURIComponent\(threadUuid\)/)
  assert.match(source, /className="chat-detail__trajectory-link"[\s\S]*?href=\{threadTrajectoryHref\(projectUuid, selectedThread\.uuid\)\}[\s\S]*?target="_blank"[\s\S]*?rel="noopener noreferrer"/)
  assert.match(source, /<div className="chat-detail-actions">[\s\S]*?chat-detail__trajectory-link[\s\S]*?<CollapseButton overlay=\{overlay\} onToggle=\{toggleExpanded\}/)
  assert.match(source, /chat-detail__trajectory-link[\s\S]*?<RouteIcon size=\{15\} aria-hidden="true" \/>[\s\S]*?<\/a>/)
  assert.doesNotMatch(source, /trajectory\.thread\.button/)
})
