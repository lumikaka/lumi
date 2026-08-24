package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	agentprompts "lumi/internal/agent/prompts"
	"lumi/internal/llm"
	"lumi/internal/production"
	"lumi/internal/project"
	"lumi/internal/promptcatalog"
	"lumi/internal/story"
)

type contextItem struct {
	itemRecord
	TurnUUID, RunUUID   string
	ImageReferenceCount int64
}

var systemPromptTemplate = template.Must(template.New("system.md").Option("missingkey=error").Parse(agentprompts.SystemTemplate()))

type systemPromptTemplateData struct {
	LanguageInstruction string
	BasePrompt          string
	ScenePrompt         string
	APIOverview         string
}

func (service *Service) buildContext(ctx context.Context, store *project.Store, tc toolContext) ([]llm.ChatMessage, int, int64, error) {
	var generationLanguage string
	if err := store.DB().WithContext(ctx).Model(&project.Project{}).Where("uuid = ?", store.ProjectUUID()).Pluck("generation_language", &generationLanguage).Error; err != nil {
		return nil, 0, 0, err
	}
	items, err := loadContextItems(ctx, store, tc.Thread.ID, tc.Turn.QueueSequence)
	if err != nil {
		return nil, 0, 0, err
	}
	prompts, frozen := frozenContextPrompts(items, tc.Turn.ID)
	if !frozen {
		prompts, err = service.loadContextPrompts(ctx, store, tc.Thread)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	prompts.ToolMode = normalizedToolMode(prompts.ToolMode)
	throughSequence := maxItemSequence(items)
	messages := contextMessages(items, "", generationLanguage, prompts)
	encoded, _ := json.Marshal(messages)
	if len(encoded) <= MaxContextBytes {
		return messages, len(encoded), throughSequence, nil
	}
	if len(items) < 24 {
		return nil, len(encoded), throughSequence, domainError(CodeContextTooLarge, "Agent 上下文过大", "单次模型上下文超过本机配置限制。", nil)
	}
	cut := len(items) - 20
	summary := summarizeItems(items[:cut])
	if strings.TrimSpace(summary) == "" {
		return nil, len(encoded), throughSequence, domainError(CodeContextTooLarge, "Agent 上下文无法压缩", "请创建新 thread 继续。", nil)
	}
	if err := service.persistSummary(ctx, store, tc, maxItemSequence(items[:cut]), summary, len(encoded)); err != nil {
		return nil, len(encoded), throughSequence, err
	}
	messages = contextMessages(items[cut:], summary, generationLanguage, prompts)
	encoded, _ = json.Marshal(messages)
	if len(encoded) > MaxContextBytes {
		return nil, len(encoded), throughSequence, domainError(CodeContextTooLarge, "Agent 上下文过大", "压缩后仍超过本机配置限制，请创建新 thread。", nil)
	}
	return messages, len(encoded), throughSequence, nil
}

func loadContextItems(ctx context.Context, store *project.Store, threadID, throughTurnSequence int64) ([]contextItem, error) {
	var records []itemRecord
	if err := store.DB().WithContext(ctx).Where("thread_id=?", threadID).Order("sequence,id").Find(&records).Error; err != nil {
		return nil, err
	}
	type publicTurn struct {
		ID            int64
		QueueSequence int64
		UUID          string
	}
	type publicRun struct {
		ID   int64
		UUID string
	}
	var turns []publicTurn
	var runs []publicRun
	if err := store.DB().WithContext(ctx).Table("chat_turns").Select("id,queue_sequence,uuid").Where("thread_id=?", threadID).Scan(&turns).Error; err != nil {
		return nil, err
	}
	if err := store.DB().WithContext(ctx).Table("chat_runs").Select("id,uuid").Where("thread_id=?", threadID).Scan(&runs).Error; err != nil {
		return nil, err
	}
	turnUUIDs, runUUIDs, turnSequences := make(map[int64]string, len(turns)), make(map[int64]string, len(runs)), make(map[int64]int64, len(turns))
	for _, turn := range turns {
		turnUUIDs[turn.ID] = turn.UUID
		turnSequences[turn.ID] = turn.QueueSequence
	}
	for _, run := range runs {
		runUUIDs[run.ID] = run.UUID
	}
	items := make([]contextItem, 0, len(records))
	for _, record := range records {
		if record.TurnID != nil && turnSequences[*record.TurnID] > throughTurnSequence {
			continue
		}
		item := contextItem{itemRecord: record}
		if record.TurnID != nil {
			item.TurnUUID = turnUUIDs[*record.TurnID]
		}
		if record.RunID != nil {
			item.RunUUID = runUUIDs[*record.RunID]
		}
		if record.ItemType == "user_message" {
			if err := store.DB().WithContext(ctx).Table("chat_item_file_references").Where("chat_item_id=?", record.ID).Count(&item.ImageReferenceCount).Error; err != nil {
				return nil, err
			}
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(left, right int) bool {
		leftTurn, rightTurn := int64(0), int64(0)
		if items[left].TurnID != nil {
			leftTurn = turnSequences[*items[left].TurnID]
		}
		if items[right].TurnID != nil {
			rightTurn = turnSequences[*items[right].TurnID]
		}
		if leftTurn == rightTurn {
			return items[left].Sequence < items[right].Sequence
		}
		return leftTurn < rightTurn
	})
	return items, nil
}

func maxItemSequence(items []contextItem) int64 {
	var maximum int64
	for _, item := range items {
		if item.Sequence > maximum {
			maximum = item.Sequence
		}
	}
	return maximum
}

type contextPromptSet struct {
	Assistant           string `json:"assistant"`
	Scene               string `json:"scene,omitempty"`
	APIOverview         string `json:"api_overview,omitempty"`
	Summary             string `json:"summary"`
	LanguageInstruction string `json:"language_instruction"`
	ToolProtocol        string `json:"tool_protocol,omitempty"`
	ToolMode            string `json:"tool_mode,omitempty"`
}

func frozenContextPrompts(items []contextItem, turnID int64) (contextPromptSet, bool) {
	for _, item := range items {
		if item.TurnID == nil || *item.TurnID != turnID || item.ItemType != "user_message" {
			continue
		}
		var metadata struct {
			PromptSnapshot contextPromptSet `json:"prompt_snapshot"`
		}
		if json.Unmarshal([]byte(item.MetadataJSON), &metadata) == nil && strings.TrimSpace(metadata.PromptSnapshot.Assistant) != "" && strings.TrimSpace(metadata.PromptSnapshot.Summary) != "" && strings.TrimSpace(metadata.PromptSnapshot.LanguageInstruction) != "" {
			return metadata.PromptSnapshot, true
		}
	}
	return contextPromptSet{}, false
}

func loadContextPrompts(ctx context.Context, store *project.Store, thread threadRecord) (contextPromptSet, error) {
	return loadContextPromptsForModeWithRoutes(ctx, store, thread, ToolModeProjectAPI, agentAPIRoutes())
}

func (service *Service) loadContextPrompts(ctx context.Context, store *project.Store, thread threadRecord) (contextPromptSet, error) {
	return loadContextPromptsForModeWithRoutes(ctx, store, thread, service.configuredToolMode(thread), service.requestAPIRoutes())
}

func loadContextPromptsForMode(ctx context.Context, store *project.Store, thread threadRecord, mode string) (contextPromptSet, error) {
	return loadContextPromptsForModeWithRoutes(ctx, store, thread, mode, agentAPIRoutes())
}

func loadContextPromptsForModeWithRoutes(ctx context.Context, store *project.Store, thread threadRecord, mode string, routes []agentAPIRoute) (contextPromptSet, error) {
	service := story.NewService(store)
	load := func(key string) (string, error) { return service.EffectivePrompt(ctx, promptcatalog.GroupAgent, key) }
	mode = normalizedToolMode(mode)
	if mode == "" {
		return contextPromptSet{}, domainError(CodeToolNotAllowed, "Tool Protocol 无效", "新 Run 必须显式使用 project_api_tools；空 mode 不会回退到 legacy typed tools。", nil)
	}
	definition, defined := sceneDefinitionForThread(thread)
	if mode != ToolModeProjectAPI || !defined {
		return contextPromptSet{}, domainError(CodeToolNotAllowed, "Tool Protocol 无效", "只有 project_api_tools 可以装配新的 Prompt snapshot；legacy 仅可恢复已持久化快照。", nil)
	}
	assistant, err := load(definition.BasePromptKey)
	if err != nil {
		return contextPromptSet{}, err
	}
	summary, err := load("conversation_summary")
	if err != nil {
		return contextPromptSet{}, err
	}
	languageInstruction, err := service.EffectiveLanguageInstruction(ctx)
	if err != nil {
		return contextPromptSet{}, err
	}
	scene := ""
	var template string
	template, err = load(definition.ScenePromptKey)
	if err == nil {
		recommendedGuides := renderRecommendedGuideList(definition.RecommendedGuideIDs)
		switch definition.Key {
		case SceneProjectAssistant, ScenePremiseAsset:
			scene, err = promptcatalog.Render(template, map[string]string{"project_uuid": promptSceneValue(store.ProjectUUID()), "recommended_guides": recommendedGuides})
		case SceneAssetReference:
			scene, err = renderAssetReferenceScene(ctx, store, thread, template, recommendedGuides)
		case SceneStoryboardReference:
			scene, err = renderStoryboardReferenceScene(ctx, store, thread, template, recommendedGuides)
		}
	}
	if err != nil {
		return contextPromptSet{}, err
	}
	apiOverview, err := renderAgentDocWithRoutes(agentDocOverviewPath, routes)
	if err != nil {
		return contextPromptSet{}, err
	}
	return contextPromptSet{Assistant: assistant, Scene: scene, APIOverview: apiOverview, Summary: summary, LanguageInstruction: languageInstruction, ToolProtocol: ToolProtocolProjectAPI, ToolMode: mode}, nil
}

func renderAssetReferenceScene(ctx context.Context, store *project.Store, thread threadRecord, template, recommendedGuides string) (string, error) {
	productionService := production.NewService(store, nil)
	asset, err := productionService.GetPremiseAsset(ctx, thread.SubjectUUID)
	if err != nil {
		return "", err
	}
	if asset.DeletedAt != nil {
		return "", domainError(CodeToolNotAllowed, "引用设定项已在回收站", "该引用会话不能继续操作；如需继续，请从 active 设定项重新打开会话。", nil)
	}
	if asset.CurrentVariant == nil || !isUUIDv7(asset.CurrentVariant.Asset.UUID) {
		return "", domainError(CodeStateConflict, "当前设定图不可用", "asset_reference 会话要求当前设定项具有可读取的图片。", nil)
	}
	premise, err := productionService.GetPremise(ctx)
	if err != nil {
		return "", err
	}
	tags, _ := json.Marshal(asset.Tags)
	values := map[string]string{
		"project_uuid":       promptSceneValue(store.ProjectUUID()),
		"subject_uuid":       promptSceneValue(asset.UUID),
		"asset_type":         promptSceneValue(asset.AssetType),
		"asset_title":        promptSceneValue(asset.Title),
		"asset_summary":      promptSceneValue(asset.Summary),
		"asset_tags":         promptSceneValue(string(tags)),
		"current_file_uuid":  promptSceneValue(asset.CurrentVariant.Asset.UUID),
		"asset_revision":     strconv.FormatInt(asset.Revision, 10),
		"overall_style":      promptSceneValue(premise.DefaultStyle),
		"recommended_guides": recommendedGuides,
	}
	return promptcatalog.Render(template, values)
}

func renderStoryboardReferenceScene(ctx context.Context, store *project.Store, thread threadRecord, template, recommendedGuides string) (string, error) {
	var binding struct {
		ChapterUUID string
		SectionUUID string
	}
	err := store.DB().WithContext(ctx).Table("comic_sections AS sections").
		Select("chapters.uuid AS chapter_uuid, sections.uuid AS section_uuid").
		Joins("JOIN chapter_comic_states AS states ON states.id = sections.chapter_comic_state_id").
		Joins("JOIN chapters ON chapters.id = states.chapter_id").
		Where("chapters.project_id = ? AND chapters.deleted_at IS NULL AND sections.uuid = ? AND sections.deleted_at IS NULL", thread.ProjectID, thread.SubjectUUID).
		Take(&binding).Error
	if err != nil {
		return "", err
	}
	return promptcatalog.Render(template, map[string]string{
		"project_uuid":       promptSceneValue(store.ProjectUUID()),
		"chapter_uuid":       promptSceneValue(binding.ChapterUUID),
		"section_uuid":       promptSceneValue(binding.SectionUUID),
		"recommended_guides": recommendedGuides,
	})
}

func promptSceneValue(value string) string {
	value = strings.ReplaceAll(value, "{{", "{ {")
	return strings.ReplaceAll(value, "}}", "} }")
}

func contextMessages(items []contextItem, summary, generationLanguage string, prompts contextPromptSet) []llm.ChatMessage {
	messages := make([]llm.ChatMessage, 0, len(items)+2)
	lastDocResult := -1
	for index, item := range items {
		if item.ItemType == "tool_result" && item.ToolName == "read_agent_doc" {
			lastDocResult = index
		}
	}
	languageInstruction := strings.TrimSpace(prompts.LanguageInstruction)
	if languageInstruction == "" {
		languageInstruction = project.GenerationLanguageInstruction(generationLanguage)
	}
	systemPrompt := renderSystemPrompt(languageInstruction, prompts)
	messages = append(messages, llm.ChatMessage{Role: "system", Content: systemPrompt})
	if summary != "" {
		rendered, err := promptcatalog.Render(prompts.Summary, map[string]string{"summary": summary})
		if err != nil {
			rendered = summary
		}
		messages = append(messages, llm.ChatMessage{Role: "system", Content: rendered})
	}
	for index, item := range items {
		switch item.ItemType {
		case "user_message":
			content := item.Content
			if item.ImageReferenceCount > 0 {
				content += fmt.Sprintf("\n\n[System note: this message has %d image reference(s). They are supplied to image_gen automatically; do not ask the user for file UUIDs or repeat them in reference_file_uuids.]", item.ImageReferenceCount)
			}
			messages = append(messages, llm.ChatMessage{Role: "user", Content: content})
		case "assistant_message":
			messages = append(messages, llm.ChatMessage{Role: "assistant", Content: item.Content})
		case "tool_call":
			providerCallID := metadataString(item.MetadataJSON, "provider_call_id")
			if providerCallID == "" {
				providerCallID = item.RemoteItemUUID
			}
			messages = append(messages, llm.ChatMessage{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: providerCallID, Name: item.ToolName, Arguments: item.Content}}})
		case "tool_result":
			providerCallID := metadataString(item.MetadataJSON, "provider_call_id")
			if providerCallID == "" {
				providerCallID = item.RemoteItemUUID
			}
			content := item.Content
			if item.ToolName == "read_agent_doc" && index != lastDocResult {
				content = compactAgentDocContextResult(content)
			}
			messages = append(messages, llm.ChatMessage{Role: "tool", ToolCallID: providerCallID, Content: content})
		case "error":
			messages = append(messages, llm.ChatMessage{Role: "system", Content: "Previous local runtime error: " + item.Content})
		}
	}
	return messages
}

func renderSystemPrompt(languageInstruction string, prompts contextPromptSet) string {
	var rendered strings.Builder
	err := systemPromptTemplate.Execute(&rendered, systemPromptTemplateData{
		LanguageInstruction: strings.TrimSpace(languageInstruction),
		BasePrompt:          strings.TrimSpace(prompts.Assistant),
		ScenePrompt:         strings.TrimSpace(prompts.Scene),
		APIOverview:         strings.TrimSpace(prompts.APIOverview),
	})
	if err != nil {
		panic(fmt.Sprintf("render embedded Agent system prompt: %v", err))
	}
	return strings.TrimSpace(rendered.String())
}

func compactAgentDocContextResult(content string) string {
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			Path   string `json:"path"`
			DocRef string `json:"doc_ref"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(content), &envelope) != nil || !envelope.Success {
		return content
	}
	ref := envelope.Data.DocRef
	if ref == "" {
		ref = envelope.Data.Path
	}
	encoded, _ := json.Marshal(map[string]any{"success": true, "data": map[string]any{"doc_ref": ref, "compacted": true}})
	return string(encoded)
}

func metadataString(raw, key string) string {
	var value map[string]any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return ""
	}
	text, _ := value[key].(string)
	return text
}

func summarizeItems(items []contextItem) string {
	var builder strings.Builder
	for _, item := range items {
		if builder.Len() >= MaxSummaryBytes {
			break
		}
		content := strings.TrimSpace(item.Content)
		if len(content) > 1200 {
			content = content[:1200] + "…"
		}
		line := fmt.Sprintf("[%d %s/%s] %s\n", item.Sequence, item.Role, item.ItemType, content)
		remaining := MaxSummaryBytes - builder.Len()
		if len(line) > remaining {
			line = line[:remaining]
		}
		builder.WriteString(line)
	}
	return strings.TrimSpace(builder.String())
}

func (service *Service) persistSummary(ctx context.Context, store *project.Store, tc toolContext, through int64, summary string, sourceBytes int) error {
	uuid, err := newUUIDv7()
	if err != nil {
		return err
	}
	sqlDB, err := store.DB().DB()
	if err != nil {
		return err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	thread, err := lockThreadSQL(ctx, tx, tc.Thread.ProjectID, tc.Thread.UUID)
	if err != nil {
		return err
	}
	now := service.now().UTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO agent_context_summaries(uuid,thread_id,through_item_sequence,summary,source_bytes,created_at) VALUES(?,?,?,?,?,?) ON CONFLICT(thread_id,through_item_sequence) DO NOTHING`, uuid, thread.ID, through, summary, sourceBytes, now)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted > 0 {
		if _, err := appendEventTx(ctx, tx, &thread, &tc.Run.ID, "compaction_created", map[string]any{
			"project_uuid":          tc.ProjectUUID,
			"thread_uuid":           tc.Thread.UUID,
			"turn_uuid":             tc.Turn.UUID,
			"run_uuid":              tc.Run.UUID,
			"summary_uuid":          uuid,
			"through_item_sequence": through,
		}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE chat_threads SET next_event_sequence=?,updated_at=? WHERE id=?`, thread.NextEventSequence, now, thread.ID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if inserted > 0 {
		service.broadcastThread(tc.ProjectUUID, tc.Thread.UUID, "chat:compaction_changed", map[string]any{
			"project_uuid": tc.ProjectUUID,
			"thread_uuid":  tc.Thread.UUID,
			"turn_uuid":    tc.Turn.UUID,
			"run_uuid":     tc.Run.UUID,
			"summary_uuid": uuid,
		})
	}
	return nil
}

var _ = time.Time{}
