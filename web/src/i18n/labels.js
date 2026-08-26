export function statusLabel(t, value) {
  const key = {
    blank: 'common.status.blank',
    cancelled: 'common.status.cancelled',
    completed: 'common.status.completed',
    connected: 'common.status.connected',
    connecting: 'common.status.connecting',
    consumed: 'common.status.consumed',
    consuming: 'common.status.consuming',
    corrupt: 'common.status.corrupt',
    deleted: 'common.status.deleted',
    expired: 'common.status.expired',
    failed: 'common.status.failed',
    idle: 'chat.status.idle',
    missing: 'common.status.missing',
    interrupted: 'common.status.interrupted',
    pending: 'common.status.pending',
    processing: 'common.status.processing',
    quarantined: 'common.status.quarantined',
    queued: 'common.status.queued',
    ready: 'common.status.ready',
    receiving: 'common.status.receiving',
    running: 'common.status.in_progress',
    saved: 'common.status.saved',
    unavailable: 'common.status.unavailable',
    waiting_for_input: 'common.status.waiting_for_input',
	waiting_for_workflow: 'chat.status.waiting_for_workflow',
  }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}

export function sourceTypeLabel(t, value) {
  const key = {
    manual: 'story.source.manual',
    manual_entry: 'story.source.manual',
    manual_edit: 'story.source.manual',
    upload: 'story.source.upload',
    imported: 'story.source.upload',
    file_import: 'story.source.upload',
    external_import: 'story.source.upload',
    ai: 'story.source.ai',
    ai_generated: 'story.source.ai',
    generated: 'story.source.ai',
    ai_asset_thread: 'story.source.ai',
    restored: 'story.source.restored',
    restore: 'story.source.restored',
    version_restore: 'story.source.restored',
    replacement: 'story.source.replacement',
    breakdown: 'story.source.breakdown',
    derived: 'story.source.breakdown',
    exported: 'story.source.exported',
    project_created: 'story.source.project_created',
    migration: 'story.source.migration',
    default_restore: 'story.source.default_restore',
    project_language_changed: 'story.source.project_language_changed',
  }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}

export function projectionStateLabel(t, value) {
  const key = { synced: 'story.projection.synced', pending: 'story.projection.pending', conflict: 'story.projection.conflict' }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}

export function assetKindLabel(t, value) {
  const key = { image: 'premise.assets.kind.image', text: 'premise.assets.kind.text', file: 'premise.assets.kind.file' }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}

export function comicStateLabel(t, value) {
  const key = { empty: 'comic.state.empty', draft: 'comic.state.draft', ready: 'comic.state.ready' }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}

export function taskKindLabel(t, value) {
  const key = {
    story_chapter_generation: 'common.task_kind.story_chapter_generation',
    story_profile_generation: 'common.task_kind.story_profile_generation',
    story_profile_from_chapters: 'common.task_kind.story_profile_from_chapters',
    story_chapter_batch_plan: 'common.task_kind.story_chapter_batch_plan',
    comic_storyboard_generation: 'common.task_kind.comic_storyboard_generation',
    asset_reconcile: 'common.task_kind.asset_reconcile',
    asset_integrity_scan: 'common.task_kind.asset_integrity_scan',
    asset_thumbnail_rebuild: 'common.task_kind.asset_thumbnail_rebuild',
    asset_upload_cleanup: 'common.task_kind.asset_upload_cleanup',
    asset_gc_apply: 'common.task_kind.asset_gc_apply',
    premise_setting_generation: 'common.task_kind.premise_setting_generation',
    premise_asset_breakdown: 'common.task_kind.premise_asset_breakdown',
    premise_asset_generation: 'common.task_kind.premise_asset_generation',
    comic_image_generation: 'common.task_kind.comic_image_generation',
    comic_export: 'common.task_kind.comic_export',
  }[value]
  return key ? t(key) : t('common.status.unknown_with_code', { code: value || '—' })
}
