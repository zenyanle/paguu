package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"paguu/internal/enrich"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ===================================================================
// Article 相关操作
// ===================================================================

// FilterByTag 实现了按 tag 筛选并按 ID 降序排列
func (r *Repository) FilterByTag(ctx context.Context, tag string, limit, offset int) ([]Article, int64, error) {
	var articles []Article
	var total int64

	// 1. 构建查询
	query := r.db.WithContext(ctx).Model(&Article{})

	// 2. 添加 Tag 筛选条件
	// "tags @> ?" 是 GIN 索引支持的 "数组包含" 查询
	query = query.Where("tags @> ?", pq.Array([]string{tag}))

	// 3. 获取总数 (用于分页)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count articles by tag: %w", err)
	}

	// 4. 获取分页后的数据
	err := query.Order("id DESC"). // 按 ID 降序
					Limit(limit).
					Offset(offset).
					Find(&articles).Error

	if err != nil {
		return nil, total, fmt.Errorf("failed to find articles by tag: %w", err)
	}

	return articles, total, nil
}

// FindSimilarBySourceID 实现了 "查找与 ID=x 最相似的 k 个"
func (r *Repository) FindSimilarBySourceID(ctx context.Context, sourceID uint, k int) ([]Article, error) {
	// 1. 获取源文章的向量
	var sourceArticle Article
	err := r.db.WithContext(ctx).Select("embedding").First(&sourceArticle, sourceID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("未找到源文章 ID: %d", sourceID)
		}
		return nil, fmt.Errorf("获取源向量失败: %w", err)
	}
	anchorVector := sourceArticle.Embedding

	// 2. 执行向量搜索
	var similarResults []Article
	err = r.db.WithContext(ctx).Clauses(clause.OrderBy{
		Expression: clause.Expr{
			SQL:  "embedding <#> ?",
			Vars: []interface{}{anchorVector},
		},
	}).
		Where("id != ?", sourceID).
		Limit(k).
		Find(&similarResults).Error

	if err != nil {
		return nil, fmt.Errorf("向量搜索失败: %w", err)
	}

	return similarResults, nil
}

// InsertArticle 插入新文章
func (r *Repository) InsertArticle(ctx context.Context, article *Article) error {
	result := r.db.WithContext(ctx).Create(article)
	return result.Error
}

// FindClosestArticle 查找最接近的向量及其距离 (用于检查重复)
// 返回: (最接近的文章, 距离, 错误)
func (r *Repository) FindClosestArticle(ctx context.Context, queryVector pgvector.Vector) (*Article, float64, error) {
	// 我们需要一个临时结构体来接收查询结果
	var closest struct {
		Article
		Distance float64 `gorm:"column:distance"`
	}

	err := r.db.WithContext(ctx).Model(&Article{}).
		Select("*, embedding <#> ? AS distance", queryVector).
		Order("distance ASC").
		Limit(1).
		First(&closest).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, nil // 没找到 (数据库是空的)，不是错误
		}
		return nil, 0, fmt.Errorf("查找最近向量失败: %w", err)
	}

	return &closest.Article, closest.Distance, nil
}

// MergeDuplicate 合并重复项
// 将新的 InterviewQuestion 添加到 ext 字段的数组中
func (r *Repository) MergeDuplicate(ctx context.Context, targetID uint, duplicate enrich.InterviewQuestion) error {
	// 序列化 InterviewQuestion 为 JSON
	duplicateJSON, err := json.Marshal(duplicate)
	if err != nil {
		return fmt.Errorf("序列化 InterviewQuestion 失败: %w", err)
	}

	// 将新的 InterviewQuestion 追加到 ext 数组末尾
	// COALESCE(ext, '[]'::jsonb) 确保如果 ext 为 NULL，则初始化为空数组
	result := r.db.WithContext(ctx).Model(&Article{}).
		Where("id = ?", targetID).
		UpdateColumn("ext", gorm.Expr(
			"COALESCE(ext, '[]'::jsonb) || ?::jsonb",
			duplicateJSON,
		))

	return result.Error
}

// QuestionInsertStatus 表示问题插入的状态
type QuestionInsertStatus int

const (
	QuestionInsertStatusFailed QuestionInsertStatus = iota
	QuestionInsertStatusMerged
	QuestionInsertStatusSuccess
)

// ProcessEnrichedQuestion 实现了完整的新增记录逻辑 (去重与合并)
//
// 这是你的 worker 应该调用的主要方法。它接收：
// 1. q: 一个从 LLM 返回的、已丰富的 InterviewQuestion 结构体。
// 2. vector: q 对应的、已归一化的 1024 维向量。
// 3. similarityThreshold: 你设定的重复项阈值 (例如 -0.95)。
//
// 它会自动处理"查找-决策-插入/合并"的完整流程。
// 返回值: (QuestionInsertStatus, error)
func (r *Repository) ProcessEnrichedQuestion(
	ctx context.Context,
	q enrich.InterviewQuestion,
	vector []float32,
	similarityThreshold float64, // e.g., -0.95
) (QuestionInsertStatus, error) {

	pgNewVec := pgvector.NewVector(vector)

	// 1. 【查找】使用向量查找最接近的现有文章
	closestArticle, distance, err := r.FindClosestArticle(ctx, pgNewVec)
	if err != nil {
		return QuestionInsertStatusFailed, fmt.Errorf("查找最近向量失败: %w", err)
	}

	// 2. 【决策】
	if closestArticle != nil && distance < similarityThreshold {
		// --- 【合并逻辑】---
		// 判定为重复项
		slog.Info("发现重复项，正在合并",
			"target_id", closestArticle.ID,
			"distance", distance)

		// 将新的 InterviewQuestion 追加到 ext 数组中
		err = r.MergeDuplicate(ctx, closestArticle.ID, q)
		if err != nil {
			return QuestionInsertStatusFailed, fmt.Errorf("合并重复项到 ID %d 失败: %w", closestArticle.ID, err)
		}

		return QuestionInsertStatusMerged, nil

	} else {
		// --- 【新增逻辑】---
		if closestArticle != nil {
			slog.Info("判定为新文章，正在插入",
				"nearest_id", closestArticle.ID,
				"distance", distance,
				"threshold", similarityThreshold)
		} else {
			slog.Info("判定为新文章 (库为空)，正在插入")
		}

		// 3. 【映射】将 InterviewQuestion 直接映射到 Article
		// ext 初始化为空数组
		emptyArray, _ := json.Marshal([]enrich.InterviewQuestion{})

		newArticle := &Article{
			OriginalQuestion: q.OriginalQuestion,
			DetailedQuestion: &q.DetailedQuestion,
			ConciseAnswer:    &q.ConciseAnswer,
			Tags:             pq.StringArray(q.Tags),
			Embedding:        pgNewVec,
			Ext:              datatypes.JSON(emptyArray), // 初始化为空的 InterviewQuestion 数组
		}

		// 4. 【执行插入】
		err = r.InsertArticle(ctx, newArticle)
		if err != nil {
			return QuestionInsertStatusFailed, fmt.Errorf("插入新文章失败: %w", err)
		}

		return QuestionInsertStatusSuccess, nil
	}
}
