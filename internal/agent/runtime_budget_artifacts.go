package agent

import (
	"context"
	"fmt"

	"lumi/internal/project"
)

// Reconcile only artifacts attributable to this Run, never unrelated content
// already present in the chapter. Read narrow receipts and SQLite facts rather
// than loading storyboard text into another model request.
func (service *Service) reconcileBudgetComicArtifacts(ctx context.Context, store *project.Store, tc toolContext) (bool, error) {
	run, err := service.refreshRun(ctx, store, tc.Run.ID)
	if err != nil {
		return false, err
	}
	if run.Status == TurnCompleted || run.Status == TurnCancelled {
		return true, nil
	}
	if run.CancelRequestedAt != nil {
		return true, service.cancelRun(ctx, store, tc)
	}
	var artifacts []struct {
		UUID          string
		HasStoryboard bool
	}
	err = store.DB().WithContext(ctx).Raw(`WITH written_sections AS (
		SELECT s.uuid FROM agent_tool_executions x
		JOIN chat_items receipt ON receipt.id=x.item_id
		JOIN chapters c ON c.project_id=?
		JOIN chapter_comic_states cs ON cs.chapter_id=c.id
		JOIN comic_sections s ON s.chapter_comic_state_id=cs.id AND (
			s.uuid=json_extract(x.result_json,'$.data.uuid') OR
			json_extract(x.result_json,'$.ui_ref.href')='@project/chapters/' || c.uuid || '/sections/' || s.uuid OR
			(x.tool_name='update_comic_storyboard' AND s.uuid=x.target_uuid)
		)
		WHERE x.run_id=? AND x.state='completed' AND json_extract(x.result_json,'$.success')=1
		  AND (json_extract(receipt.metadata_json,'$.route_id') IN (?,?,?) OR x.tool_name='update_comic_storyboard')
		UNION
		SELECT j.value FROM workflow_awaits a
		JOIN workflows w ON w.id=a.workflow_id
		JOIN workflow_steps ws ON ws.workflow_id=w.id
		JOIN json_each(ws.output_json,'$.section_uuids') j
		WHERE a.chat_run_id=? AND a.status IN ('ready','resuming','resumed')
		  AND w.project_id=? AND w.kind=? AND w.status='completed' AND ws.status='completed'
	)
	SELECT DISTINCT s.uuid, (sb.id IS NOT NULL AND length(trim(sb.content_md))>0) AS has_storyboard
	FROM written_sections written
	JOIN comic_sections s ON s.uuid=written.uuid
	JOIN chapter_comic_states cs ON cs.id=s.chapter_comic_state_id
	JOIN chapters c ON c.id=cs.chapter_id
	LEFT JOIN comic_storyboard_variants sb ON sb.id=s.current_storyboard_variant_id AND sb.comic_section_id=s.id
	WHERE c.project_id=? AND c.deleted_at IS NULL AND s.deleted_at IS NULL
	ORDER BY s.uuid`, tc.Thread.ProjectID, tc.Run.ID, RouteComicSectionCreate, RouteStoryboardUpdate, RouteStoryboardSelect,
		tc.Run.ID, tc.Thread.ProjectID, WorkflowComicStoryboard, tc.Thread.ProjectID).Scan(&artifacts).Error
	if err != nil || len(artifacts) == 0 {
		return false, err
	}
	profile, err := store.RequirePictureBookProfile()
	if err != nil {
		return false, err
	}
	sectionName := "页面"
	if profile.Format == project.PictureBookVertical {
		sectionName = "画面段落"
	}
	ready := 0
	uuids := make([]string, len(artifacts))
	for index, artifact := range artifacts {
		uuids[index] = artifact.UUID
		if artifact.HasStoryboard {
			ready++
		}
	}
	// Saved pages prove progress, not fulfillment of an arbitrary natural-language
	// request. Keep the existing terminal status contract and explicitly record a
	// partial result instead of claiming that the whole chapter is complete.
	content := fmt.Sprintf("本轮仅部分完成：已核验保存 %d 个%s，其中 %d 个已有可用分镜。", len(artifacts), sectionName, ready)
	if ready < len(artifacts) {
		content += fmt.Sprintf("另有 %d 个仅建结构、需再生成分镜。", len(artifacts)-ready)
	}
	content += "已达到本轮安全预算，执行已停止；其余请求尚未核验，不能确认整体完成。已保存内容会保留。"
	return true, service.completeRun(ctx, store, tc, content, map[string]any{
		"runtime_generated":       true,
		"completion_reason":       "budget_limit",
		"budget_reason":           run.LimitReason,
		"completion_status":       "partial",
		"section_uuids":           uuids,
		"persisted_section_count": len(artifacts),
		"storyboard_ready_count":  ready,
	})
}
