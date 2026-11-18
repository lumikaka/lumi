'use strict';
// Pure policy over freshly read Lumi facts. No task is dispatched here; the
// chapter owner persists the decision before using the Host task adapter.
function schedule(chapter, pages) {
  const scenes = chapter.scenes || [], byPage = new Map(pages.map(p => [p.uuid, p]));
  const pendingValid = value => value === undefined || (Array.isArray(value) && value.every(key => typeof key === 'string' && !!key.trim()));
  if (!Array.isArray(scenes) || scenes.some(s => !s || !Array.isArray(s.pages) || s.pages.some(p => typeof p !== 'string')
    || !['waiting', 'active', 'paused', 'done'].includes(s.status) || !pendingValid(s.pending_requests))
    || !pendingValid(chapter.pending_requests) || (chapter.user_review_pending !== undefined && typeof chapter.user_review_pending !== 'boolean')) return { eligible: [], active: [], pause: [], blocked: ['invalid_scene_plan'], complete: false };
  const active = scenes.filter(s => ['active', 'paused'].includes(s.status) || s.pending_requests?.length);
  const pagePositions = new Map(pages.map((page, index) => [page.uuid, index]));
  const explicitScope = Object.hasOwn(chapter, 'page_scope'), scope = chapter.page_scope;
  const targets = explicitScope ? scope?.page_uuids : pages.map(page => page.uuid);
  if (explicitScope && (!scope || !Array.isArray(targets) || !targets.length
    || typeof scope.user_request !== 'string' || !scope.user_request.trim()
    || targets.some((uuid, index) => typeof uuid !== 'string' || !pagePositions.has(uuid)
      || (index > 0 && pagePositions.get(uuid) <= pagePositions.get(targets[index - 1]))))) {
    return { eligible: [], active: active.map(s => s.number), pause: [], blocked: ['invalid_page_scope'], complete: false };
  }
  const targetPositions = new Map(targets.map((uuid, index) => [uuid, index]));
  const issues = [], seen = new Set(), numbers = new Set();
  let previous = -1;
  for (const [position, scene] of scenes.entries()) {
    if (!Number.isSafeInteger(scene.number) || scene.number < 1 || scene.number > 99999 || numbers.has(scene.number) || !scene.pages?.length) issues.push('invalid_scene');
    if (scene.depends_on !== undefined && (!Array.isArray(scene.depends_on)
      || scene.depends_on.some(number => !scenes.slice(0, position).some(prior => prior.number === number)))) issues.push('invalid_scene_dependency');
    numbers.add(scene.number);
    let scenePrevious = -1;
    for (const uuid of scene.pages || []) {
      const index = targetPositions.get(uuid), fullIndex = pagePositions.get(uuid);
      if (seen.has(uuid) || index === undefined || (previous >= 0 && index !== previous + 1)
        || (scenePrevious >= 0 && fullIndex !== scenePrevious + 1)) issues.push('page_ownership_or_order');
      seen.add(uuid); previous = index;
      scenePrevious = fullIndex;
    }
  }
  if (targets.some(uuid => !seen.has(uuid))) issues.push('page_scope_incomplete');
  const verified = scene => {
    const evidence = scene.verification;
    return !!(scene.status === 'done' && !scene.pending_requests?.length && scene.pages?.length
      && typeof evidence?.checked_at === 'string' && Number.isFinite(Date.parse(evidence.checked_at))
      && typeof evidence.handoff === 'string' && evidence.handoff.trim() && Array.isArray(evidence.pages)
      && scene.pages.every(uuid => { const fact = byPage.get(uuid), receipt = evidence.pages.find(p => p?.uuid === uuid);
        return fact && fact.review_status === 'passed' && receipt?.variant_uuid === fact.variant_uuid && receipt.revision === fact.revision; }));
  };
  const pair = chapter.scheduling?.mode === 'independent_pair' && typeof chapter.scheduling.user_choice === 'string' && !!chapter.scheduling.user_choice.trim();
  const capacity = pair ? 2 : 1;
  if (chapter.scheduling?.mode && !['serial', 'independent_pair'].includes(chapter.scheduling.mode)) issues.push('invalid_scheduling_mode');
  if (chapter.scheduling?.mode === 'independent_pair' && !pair) issues.push('explicit_user_choice_required');
  if (active.length > capacity) issues.push('too_many_active_scenes');
  if (issues.length || !scenes.length) return { eligible: [], active: active.map(s => s.number), pause: [], blocked: [...new Set(issues.length ? issues : ['no_scenes'])], complete: false };
  const repair = chapter.settings_repair;
  if (repair && (!['pending', 'passed'].includes(repair.status) || !Array.isArray(repair.affected_scenes)
    || repair.affected_scenes.some(number => !numbers.has(number)))) return { eligible: [], active: active.map(s => s.number), pause: [], blocked: ['invalid_settings_repair'], complete: false };
  const affected = number => repair && repair.status !== 'passed' && repair.affected_scenes?.includes(number);
  const pause = active.filter(s => affected(s.number)).map(s => s.number), eligible = [], blocked = [];
  if (chapter.pending_requests?.length) return { eligible, active: active.map(s => s.number), pause, blocked: ['chapter_request_pending'], capacity, complete: false };
  let free = Math.max(0, capacity - active.length);
  for (let i = 0; i < scenes.length; i++) {
    const scene = scenes[i];
    if (verified(scene) || active.includes(scene)) continue;
    if (scene.status === 'done' || scene.pages.every(uuid => byPage.get(uuid)?.review_status === 'passed')) { blocked.push({ scene: scene.number, reason: 'handoff_unverified' }); continue; }
    if (affected(scene.number)) { blocked.push({ scene: scene.number, reason: 'settings_repair' }); continue; }
    if (scene.initial_state_verified !== true) { blocked.push({ scene: scene.number, reason: 'initial_state_unverified' }); continue; }
    const dependencies = pair ? scene.depends_on : scenes.slice(0, i).map(s => s.number);
    if (!Array.isArray(dependencies) || dependencies.some(n => n === scene.number || !numbers.has(n)
      || scenes.findIndex(s => s.number === n) >= i || !verified(scenes.find(s => s.number === n)))) {
      blocked.push({ scene: scene.number, reason: 'dependency_unverified' }); continue;
    }
    if (chapter.user_review_pending === true) { blocked.push({ scene: scene.number, reason: 'user_review_pending' }); continue; }
    if (free > 0) { eligible.push(scene.number); free--; }
  }
  return { eligible, active: active.map(s => s.number), pause, blocked, capacity,
    complete: !active.length && scenes.every(verified) && repair?.status !== 'pending' && chapter.user_review_pending !== true };
}
module.exports = { schedule };
