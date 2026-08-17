const attachment = (status, suffix = '') => ({
  localId: `acceptance-${status}${suffix}`,
  filename: `封面参考图${suffix}.png`,
  status,
  previewUrl: '',
  upload: status === 'ready' ? { uuid: `01a-acceptance-upload-${suffix || 'ready'}` } : null,
})

export const CHAT_COMPOSER_ACCEPTANCE_STATES = [
  { id: 'idle', groupKey: 'chat.acceptance.group.base', titleKey: 'chat.acceptance.idle.title', triggerKey: 'chat.acceptance.idle.trigger', expectationKey: 'chat.acceptance.idle.expectation', initialDraft: '' },
  { id: 'focused', groupKey: 'chat.acceptance.group.base', titleKey: 'chat.acceptance.focused.title', triggerKey: 'chat.acceptance.focused.trigger', expectationKey: 'chat.acceptance.focused.expectation', initialDraft: '', forceFocus: true },
  { id: 'draft', groupKey: 'chat.acceptance.group.base', titleKey: 'chat.acceptance.draft.title', triggerKey: 'chat.acceptance.draft.trigger', expectationKey: 'chat.acceptance.draft.expectation', initialDraft: '让小女孩在月球背面发现一座发光的城市。' },
  { id: 'multiline', groupKey: 'chat.acceptance.group.base', titleKey: 'chat.acceptance.multiline.title', triggerKey: 'chat.acceptance.multiline.trigger', expectationKey: 'chat.acceptance.multiline.expectation', initialDraft: '第一幕：飞船进入月球轨道。\n第二幕：主角收到来自地表的神秘信号。\n第三幕：舱门打开，远处城市逐盏亮灯。' },
  { id: 'attachment_uploading', groupKey: 'chat.acceptance.group.attachment', titleKey: 'chat.acceptance.attachment_uploading.title', triggerKey: 'chat.acceptance.attachment_uploading.trigger', expectationKey: 'chat.acceptance.attachment_uploading.expectation', initialDraft: '以这张图片作为角色服装参考。', scene: 'premise_asset_generation', attachments: [attachment('uploading')], attachmentBlocked: true },
  { id: 'attachment_ready', groupKey: 'chat.acceptance.group.attachment', titleKey: 'chat.acceptance.attachment_ready.title', triggerKey: 'chat.acceptance.attachment_ready.trigger', expectationKey: 'chat.acceptance.attachment_ready.expectation', initialDraft: '保留参考图的头盔轮廓，改成暖黄色宇航服。', scene: 'premise_asset_generation', attachments: [attachment('ready', '-A'), attachment('ready', '-B')] },
  { id: 'attachment_error', groupKey: 'chat.acceptance.group.attachment', titleKey: 'chat.acceptance.attachment_error.title', triggerKey: 'chat.acceptance.attachment_error.trigger', expectationKey: 'chat.acceptance.attachment_error.expectation', initialDraft: '继续使用这张参考图。', scene: 'premise_asset_generation', attachments: [attachment('error')], attachmentBlocked: true },
  { id: 'sending', groupKey: 'chat.acceptance.group.submission', titleKey: 'chat.acceptance.sending.title', triggerKey: 'chat.acceptance.sending.trigger', expectationKey: 'chat.acceptance.sending.expectation', initialDraft: '生成下一页分镜。', pending: true },
  { id: 'running_stop', groupKey: 'chat.acceptance.group.runtime', titleKey: 'chat.acceptance.running_stop.title', triggerKey: 'chat.acceptance.running_stop.trigger', expectationKey: 'chat.acceptance.running_stop.expectation', initialDraft: '', activeTurn: { status: 'in_progress', queue_sequence: 3 } },
  { id: 'running_queue', groupKey: 'chat.acceptance.group.runtime', titleKey: 'chat.acceptance.running_queue.title', triggerKey: 'chat.acceptance.running_queue.trigger', expectationKey: 'chat.acceptance.running_queue.expectation', initialDraft: '下一轮把镜头拉远，露出完整的环形城市。', activeTurn: { status: 'in_progress', queue_sequence: 3 } },
  { id: 'waiting_input', groupKey: 'chat.acceptance.group.runtime', titleKey: 'chat.acceptance.waiting_input.title', triggerKey: 'chat.acceptance.waiting_input.trigger', expectationKey: 'chat.acceptance.waiting_input.expectation', initialDraft: '', activeTurn: { status: 'waiting_for_input', queue_sequence: 3 } },
  { id: 'stopping', groupKey: 'chat.acceptance.group.runtime', titleKey: 'chat.acceptance.stopping.title', triggerKey: 'chat.acceptance.stopping.trigger', expectationKey: 'chat.acceptance.stopping.expectation', initialDraft: '', activeTurn: { status: 'in_progress', queue_sequence: 3 }, abortPending: true },
]
