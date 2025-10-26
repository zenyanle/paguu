package postgres

import (
	"context"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EnqueueTask 向队列添加一个新任务（状态默认为 ready）
func (r *Repository) EnqueueTask(ctx context.Context, taskType string, payload datatypes.JSON) error {
	task := ProcessingQueue{
		TaskType: taskType,
		Payload:  payload,
		Status:   "ready",
	}
	result := r.db.WithContext(ctx).Create(&task)
	return result.Error
}

// DequeueTask 以事务方式锁定并获取一个 ready 状态的任务
func (r *Repository) DequeueTask(ctx context.Context) (*ProcessingQueue, error) {
	var task ProcessingQueue

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 使用 FOR UPDATE SKIP LOCKED 锁定一行 ready 任务
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", "ready").
			Order("created_at ASC").
			First(&task).
			Error

		if err != nil {
			return err
		}

		// 2. 锁定成功，更新状态为 processing
		err = tx.Model(&task).Update("status", "processing").Error
		return err
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没有任务可取
		}
		return nil, err
	}

	return &task, nil
}

// UpdateTaskCompleted 将任务标记为完成
func (r *Repository) UpdateTaskCompleted(ctx context.Context, task *ProcessingQueue) error {
	err := r.db.WithContext(ctx).Model(task).Update("status", "completed").Error
	return err
}

// UpdateTaskFailed 将任务标记为失败（不再重新排队,由专门的错误处理器处理）
func (r *Repository) UpdateTaskFailed(ctx context.Context, task *ProcessingQueue, taskError error) error {
	err := r.db.WithContext(ctx).Model(task).Updates(map[string]interface{}{
		"status":     "failed",
		"last_error": taskError.Error(),
		"retries":    gorm.Expr("retries + 1"),
	}).Error
	return err
}

// DequeueFailedTask 供专门的错误处理器使用，获取一个 failed 状态的任务并重试
// 使用指数退避策略：根据 retries 次数计算最小重试间隔
func (r *Repository) DequeueFailedTask(ctx context.Context, maxRetries int) (*ProcessingQueue, error) {
	var task ProcessingQueue

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 查询 failed 状态且未超过最大重试次数的任务
		// 使用指数退避：updated_at + (2^retries * 10秒) <= now()
		// 这样 retry=0 等 10s，retry=1 等 20s，retry=2 等 40s...
		query := `
			status = 'failed' 
			AND retries < ? 
			AND updated_at + (POWER(2, retries) * INTERVAL '10 seconds') <= NOW()
		`

		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(query, maxRetries).
			Order("updated_at ASC"). // 优先处理最早失败的
			First(&task).
			Error

		if err != nil {
			return err
		}

		// 2. 锁定成功，将状态改回 processing 准备重试
		err = tx.Model(&task).Update("status", "processing").Error
		return err
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没有符合条件的失败任务
		}
		return nil, err
	}

	return &task, nil
}

// ResetStuckTasks 将长时间处于 processing 状态的任务重置为 ready
// 用于处理 worker 崩溃导致的"僵尸任务"
func (r *Repository) ResetStuckTasks(ctx context.Context, timeout time.Duration) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&ProcessingQueue{}).
		Where("status = ? AND updated_at < ?", "processing", time.Now().Add(-timeout)).
		Update("status", "ready")

	return result.RowsAffected, result.Error
}
