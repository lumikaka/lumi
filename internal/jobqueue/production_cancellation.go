package jobqueue

import (
	"context"
	"errors"
)

// This is server-side recovery of durable cancellation commands. It does not
// poll providers or drive UI synchronization; clients still use WS + REST.
func (runtime *projectRuntime) reconcileProductionCancellations(ctx context.Context) error {
	rows, err := runtime.sqlDB.QueryContext(ctx, `SELECT uuid FROM production_task_runs WHERE project_id=? AND cancel_requested_at IS NOT NULL AND status NOT IN ('completed','cancelled')`, runtime.projectID)
	if err != nil {
		return err
	}
	var uuids []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			rows.Close()
			return err
		}
		uuids = append(uuids, uuid)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var result error
	for _, uuid := range uuids {
		_, err := runtime.finishProductionCancellation(ctx, uuid)
		result = errors.Join(result, err)
	}
	return result
}
