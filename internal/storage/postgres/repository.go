// 位于: internal/storage/postgres/repository.go
package postgres

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ===================================================================
// GORM 模型定义
// ===================================================================

// Article 对应 'articles' 表
type Article struct {
	ID uint `gorm:"primaryKey"`

	// --- 与 InterviewQuestion 同步的字段 ---
	OriginalQuestion string         `gorm:"type:text;not null"`
	DetailedQuestion *string        `gorm:"type:text"`
	ConciseAnswer    *string        `gorm:"type:text"`
	Tags             pq.StringArray `gorm:"type:text[]"`

	// --- 元数据和向量字段 ---
	Embedding    pgvector.Vector `gorm:"type:vector(1536)"`
	Ext          datatypes.JSON  `gorm:"type:jsonb"` // 存储 []InterviewQuestion (重复项列表)
	NotionPageID *string         `gorm:"type:text"`
	LastSyncedAt *time.Time      `gorm:"type:timestamptz"`
	CreatedAt    time.Time       `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (Article) TableName() string {
	return "articles"
}

// ProcessingQueue 对应 'processing_queue' 表
// 状态流转: ready -> processing -> completed/failed
type ProcessingQueue struct {
	ID        uint           `gorm:"primaryKey"`
	TaskType  string         `gorm:"type:text;not null"`
	Payload   datatypes.JSON `gorm:"type:jsonb;not null"`
	Status    string         `gorm:"type:text;default:'ready'"` // ready/processing/completed/failed
	Retries   int            `gorm:"default:0"`
	LastError *string        `gorm:"type:text"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
}

// TableName 指定表名
func (ProcessingQueue) TableName() string {
	return "processing_queue"
}

// ===================================================================
// Repository 结构体和初始化
// ===================================================================

// Repository 封装了所有数据库操作
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建 Repository 实例并初始化数据库
func NewRepository(dsn string) (*Repository, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	slog.Info("数据库连接成功")

	// 1. 启用 vector 扩展
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return nil, fmt.Errorf("failed to create vector extension: %w", err)
	}
	slog.Info("Vector 扩展已启用")

	// 2. 自动迁移
	slog.Info("正在自动迁移 GORM schema (articles, processing_queue)...")
	if err := db.AutoMigrate(&Article{}, &ProcessingQueue{}); err != nil {
		return nil, fmt.Errorf("failed to auto-migrate schema: %w", err)
	}
	slog.Info("GORM schema 迁移完成")

	// 3. 创建自定义索引
	slog.Info("正在确保自定义索引存在...")

	// 队列索引
	if err := createQueueIndexes(db); err != nil {
		return nil, err
	}

	// Article 索引
	if err := createArticleIndexes(db); err != nil {
		return nil, err
	}

	slog.Info("所有自定义索引已确保存在")
	return &Repository{db: db}, nil
}

// createQueueIndexes 创建队列相关索引
func createQueueIndexes(db *gorm.DB) error {
	indexes := []string{
		// ready 状态任务按创建时间排序
		`CREATE INDEX IF NOT EXISTS idx_queue_ready 
		 ON processing_queue (created_at) 
		 WHERE status = 'ready'`,

		// failed 状态任务按更新时间排序（供错误处理器使用）
		`CREATE INDEX IF NOT EXISTS idx_queue_failed 
		 ON processing_queue (updated_at, retries) 
		 WHERE status = 'failed'`,

		// processing 状态任务按更新时间排序（供僵尸任务清理使用）
		`CREATE INDEX IF NOT EXISTS idx_queue_processing 
		 ON processing_queue (updated_at) 
		 WHERE status = 'processing'`,
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to create queue index: %w", err)
		}
	}

	return nil
}

// createArticleIndexes 创建 Article 相关索引
func createArticleIndexes(db *gorm.DB) error {
	indexes := []string{
		// Tags GIN 索引 (用于 @> 查询)
		`CREATE INDEX IF NOT EXISTS idx_articles_tags_gin 
		 ON articles USING GIN (tags)`,

		// Embedding HNSW 索引 (用于向量相似度搜索)
		// 使用 vector_ip_ops 因为向量是归一化的，使用内积 <#> 查询
		`CREATE INDEX IF NOT EXISTS idx_articles_embedding_hnsw_ip
		 ON articles USING hnsw (embedding vector_ip_ops)`,
	}

	for _, sql := range indexes {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("failed to create article index: %w", err)
		}
	}

	return nil
}
