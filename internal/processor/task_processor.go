package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"paguu/internal/embedding"
	"paguu/internal/enrich"
	"paguu/internal/storage/postgres"
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
	enricher *enrich.QuestionsEnricher
	repo     *postgres.Repository
	embedder *embedding.Embedder
}

func NewTaskProcessor(enricher *enrich.QuestionsEnricher, repo *postgres.Repository, embedder *embedding.Embedder) *TaskProcessor {
	return &TaskProcessor{
		enricher: enricher,
		repo:     repo,
		embedder: embedder,
	}
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
	processingQueue, err := tp.repo.DequeueTask(ctx)
	if err != nil {
		slog.Error("repo task dequeue error", err)
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
			slog.Error("UpdateTaskFailed error after json unmarshal", err2)
		}
		return err
	}
	questionSet, err := tp.enricher.EnrichQuestions(ctx, task.RawQuestions)
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", err)
		}
		return err
	}
	vectors, err := tp.embedder.EmbedBatch(ctx, questionSet.GetEmbeddableTexts())
	if err != nil {
		if err2 := tp.repo.UpdateTaskFailed(ctx, processingQueue, err); err2 != nil {
			slog.Error("UpdateTaskFailed error", err)
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
		slog.Error("UpdateTaskCompleted error", err2)
		return err2
	}

	return nil
}

func (tp *TaskProcessor) ProcessFailedTask(ctx context.Context, maxRetries int) error {
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
