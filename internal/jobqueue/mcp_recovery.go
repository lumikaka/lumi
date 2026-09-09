package jobqueue

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"lumi/internal/agent"
	"strings"
)

// RecoverExternalTask reads a committed task before revalidating current model
// settings or resource inputs. The original task owns its frozen configuration.
func (m *Manager) RecoverExternalTask(ctx context.Context, p, kind, key string) (agent.DomainTask, bool, error) {
	if !strings.HasPrefix(key, "mcp:") || !isUUIDv7(strings.TrimPrefix(key, "mcp:")) {
		return agent.DomainTask{}, false, taskError(CodeInvalidTask, "外部任务幂等键无效", "", nil)
	}
	runtime, err := m.runtimeFor(p)
	if err != nil {
		return agent.DomainTask{}, false, err
	}
	if storyDomainTaskKind(kind) {
		var row taskRecord
		err = runtime.store.DB().WithContext(ctx).Where("project_id = ? AND kind = ? AND idempotency_key = ?", runtime.projectID, kind, key).First(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agent.DomainTask{}, false, nil
		}
		return storyDomainTask(row.DTO()), err == nil, err
	}
	var row productionTaskRecord
	err = runtime.store.DB().WithContext(ctx).Where("project_id = ? AND kind = ? AND idempotency_key = ?", runtime.projectID, kind, key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agent.DomainTask{}, false, nil
	}
	return productionDomainTask(row.DTO()), err == nil, err
}
