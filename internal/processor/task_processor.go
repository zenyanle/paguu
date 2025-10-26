package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"paguu/internal/embedding"
	"paguu/internal/enrich"
	"paguu/internal/storage/postgres"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type Task struct {
	RawQuestions string `json:"raw_questions"`

	TaskID    string                 `json:"task_id"`
	CreatedAt time.Time              `json:"created_at"`
	Source    string                 `json:"source"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// FillMetadata 自动填充元信息
func (t *Task) FillMetadata() {
	if t.TaskID == "" {
		t.TaskID = uuid.New().String()
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	if t.Source == "" {
		t.Source = "default"
	}

	if t.Metadata == nil {
		t.Metadata = make(map[string]interface{})
	}
}

func (t *Task) ToJSON() (datatypes.JSON, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task: %w", err)
	}
	return datatypes.JSON(data), nil
}

func (t *Task) FromJSON(data datatypes.JSON) error {
	if err := json.Unmarshal(data, t); err != nil {
		return fmt.Errorf("failed to unmarshal task: %w", err)
	}
	return nil
}

type TaskProcessor struct {
	enricher      *enrich.QuestionsEnricher
	repo          *postgres.Repository
	embedder      *embedding.Embedder
	activeWorkers atomic.Int32
	maxWorkers    int32
}

func NewTaskProcessor(enricher *enrich.QuestionsEnricher, repo *postgres.Repository, embedder *embedding.Embedder) *TaskProcessor {
	tp := &TaskProcessor{
		enricher:   enricher,
		repo:       repo,
		embedder:   embedder,
		maxWorkers: 3, // 默认最大并发数为3
	}
	tp.activeWorkers.Store(0)
	return tp
}

func (tp *TaskProcessor) NewTask(ctx context.Context, taskType string, task Task) error {
	payload, err := task.ToJSON()
	if err != nil {
		return err
	}
	err = tp.repo.EnqueueTask(ctx, taskType, payload)
	if err != nil {
		return err
	}
	return nil
}

func (tp *TaskProcessor) ProcessNextTask(ctx context.Context) error {
	// 检查并发限制
	if !tp.tryAcquireWorker() {
		return nil // 达到并发限制，直接返回
	}
	defer tp.releaseWorker()

	processingQueue, err := tp.repo.DequeueTask(ctx)
	if err != nil {
		slog.Error("repo task dequeue error", "error", err)
		return err
	}
	if processingQueue == nil {
		slog.Info("no left task")
		return nil
	}
	task := new(Task)
	err = task.FromJSON(processingQueue.Payload)
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error after json unmarshal", "error", err2)
		}
		return err
	}
	questionSet, err := tp.enricher.EnrichQuestions(ctx, task.RawQuestions)
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", "error", err2)
		}
		return err
	}
	vectors, err := tp.embedder.EmbedBatch(ctx, questionSet.GetEmbeddableTexts())
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", "error", err2)
		}
		return err
	}
	statusSet := make([]postgres.QuestionInsertStatus, len(vectors))
	for i := range vectors {
		statusSet[i], err = tp.repo.ProcessEnrichedQuestion(ctx, questionSet.Questions[i], vectors[i], -0.95)

		if err != nil {
			slog.Error("ProcessEnrichedQuestion error", "error", err, "question_index", i)
			if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
				slog.Error("UpdateTaskFailed error after ProcessEnrichedQuestion error", "original_error", err, "update_error", err2)
			}
			return err
		}
	}
	if err2 := tp.repo.UpdateTaskCompleted(ctx, processingQueue); err2 != nil {
		slog.Error("UpdateTaskCompleted error", "error", err2)
		return err2
	}

	return nil
}

func (tp *TaskProcessor) ProcessFailedTask(ctx context.Context, maxRetries int) error {
	// 检查并发限制
	if !tp.tryAcquireWorker() {
		return nil // 达到并发限制，直接返回
	}
	defer tp.releaseWorker()

	processingQueue, err := tp.repo.DequeueFailedTask(ctx, maxRetries)
	if err != nil {
		slog.Error("repo failed task dequeue error", "error", err)
		return err
	}
	if processingQueue == nil {
		slog.Info("no left failed task")
		return nil
	}

	task := new(Task)
	err = task.FromJSON(processingQueue.Payload)
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error after json unmarshal", "error", err2)
		}
		return err
	}

	questionSet, err := tp.enricher.EnrichQuestions(ctx, task.RawQuestions)
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", "error", err2)
		}
		return err
	}

	vectors, err := tp.embedder.EmbedBatch(ctx, questionSet.GetEmbeddableTexts())
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", "error", err2)
		}
		return err
	}

	statusSet := make([]postgres.QuestionInsertStatus, len(vectors))
	for i := range vectors {
		statusSet[i], err = tp.repo.ProcessEnrichedQuestion(ctx, questionSet.Questions[i], vectors[i], -0.95)
		if err != nil {
			slog.Error("ProcessEnrichedQuestion error", "error", err, "question_index", i)
			if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
				slog.Error("UpdateTaskFailed error after ProcessEnrichedQuestion error", "original_error", err, "update_error", err2)
			}
			return err
		}
	}

	if err2 := tp.repo.UpdateTaskCompleted(ctx, processingQueue); err2 != nil {
		slog.Error("UpdateTaskCompleted error", "error", err2)
		return err2
	}

	return nil
}

func (tp *TaskProcessor) tryAcquireWorker() bool {
	for {
		current := tp.activeWorkers.Load()
		if current >= tp.maxWorkers {
			return false
		}
		if tp.activeWorkers.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (tp *TaskProcessor) releaseWorker() {
	tp.activeWorkers.Add(-1)
}

func (tp *TaskProcessor) RunTaskWorkers(ctx context.Context, normalWorkers, retryWorkers, maxRetries int, pollInterval time.Duration, done <-chan struct{}) {
	for i := 0; i < normalWorkers; i++ {
		workerID := i + 1
		go tp.normalTaskWorker(ctx, workerID, pollInterval, done)
	}

	for i := 0; i < retryWorkers; i++ {
		workerID := i + 1
		go tp.retryTaskWorker(ctx, workerID, maxRetries, pollInterval, done)
	}

	slog.Info("任务处理工作者已启动", "normal_workers", normalWorkers, "retry_workers", retryWorkers, "max_concurrent", tp.maxWorkers)
}

func (tp *TaskProcessor) normalTaskWorker(ctx context.Context, workerID int, pollInterval time.Duration, done <-chan struct{}) {
	slog.Info("正常任务工作者启动", "worker_id", workerID)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			slog.Info("正常任务工作者收到关闭信号", "worker_id", workerID)
			return
		case <-ctx.Done():
			slog.Info("正常任务工作者上下文取消", "worker_id", workerID)
			return
		case <-ticker.C:
			err := tp.ProcessNextTask(ctx)
			if err != nil {
				slog.Error("正常任务处理失败", "worker_id", workerID, "error", err)
			}
		}
	}
}

func (tp *TaskProcessor) retryTaskWorker(ctx context.Context, workerID int, maxRetries int, pollInterval time.Duration, done <-chan struct{}) {
	slog.Info("失败任务重试工作者启动", "worker_id", workerID)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			slog.Info("失败任务重试工作者收到关闭信号", "worker_id", workerID)
			return
		case <-ctx.Done():
			slog.Info("失败任务重试工作者上下文取消", "worker_id", workerID)
			return
		case <-ticker.C:
			err := tp.ProcessFailedTask(ctx, maxRetries)
			if err != nil {
				slog.Error("失败任务重试失败", "worker_id", workerID, "error", err)
			}
		}
	}
}
