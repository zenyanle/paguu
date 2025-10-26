package api

import (
	"context"
	"log/slog"
	"net/http"
	"paguu/internal/embedding"
	"paguu/internal/processor"
	"paguu/internal/storage/postgres"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type Handler struct {
	repo          *postgres.Repository
	embedder      *embedding.Embedder
	taskProcessor *processor.TaskProcessor
}

func NewHandler(repo *postgres.Repository, embedder *embedding.Embedder, taskProcessor *processor.TaskProcessor) *Handler {
	return &Handler{
		repo:          repo,
		embedder:      embedder,
		taskProcessor: taskProcessor,
	}
}

// ArticleResponse 文章响应结构
type ArticleResponse struct {
	ID               uint           `json:"id"`
	OriginalQuestion string         `json:"original_question"`
	DetailedQuestion *string        `json:"detailed_question,omitempty"`
	ConciseAnswer    *string        `json:"concise_answer,omitempty"`
	Tags             pq.StringArray `json:"tags"`
	CreatedAt        string         `json:"created_at"`
	Similarity       *float64       `json:"similarity,omitempty"`
}

// ListArticlesRequest 列表请求参数
type ListArticlesRequest struct {
	Page     int      `form:"page" binding:"omitempty,min=1"`
	PageSize int      `form:"page_size" binding:"omitempty,min=1,max=100"`
	Tags     []string `form:"tags[]"`
}

// VectorSearchRequest 向量搜索请求参数
type VectorSearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit" binding:"required,min=1,max=100"`
}

// ListArticles 获取文章列表
func (h *Handler) ListArticles(c *gin.Context) {
	var req ListArticlesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	offset := (req.Page - 1) * req.PageSize

	articles, total, err := h.repo.ListArticles(c.Request.Context(), req.Tags, req.PageSize, offset)
	if err != nil {
		slog.Error("ListArticles error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list articles"})
		return
	}

	responses := make([]ArticleResponse, len(articles))
	for i, article := range articles {
		responses[i] = ArticleResponse{
			ID:               article.ID,
			OriginalQuestion: article.OriginalQuestion,
			DetailedQuestion: article.DetailedQuestion,
			ConciseAnswer:    article.ConciseAnswer,
			Tags:             article.Tags,
			CreatedAt:        article.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": responses,
		"pagination": gin.H{
			"page":       req.Page,
			"page_size":  req.PageSize,
			"total":      total,
			"total_page": (total + int64(req.PageSize) - 1) / int64(req.PageSize),
		},
	})
}

// VectorSearch 向量相似度搜索
func (h *Handler) VectorSearch(c *gin.Context) {
	var req VectorSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	vector, err := h.embedder.Embed(context.Background(), req.Query)
	if err != nil {
		slog.Error("Embedding error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate embedding"})
		return
	}

	articles, similarities, err := h.repo.VectorSearchArticles(c.Request.Context(), vector, req.Limit)
	if err != nil {
		slog.Error("VectorSearchArticles error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search articles"})
		return
	}

	responses := make([]ArticleResponse, len(articles))
	for i, article := range articles {
		similarity := similarities[i]
		responses[i] = ArticleResponse{
			ID:               article.ID,
			OriginalQuestion: article.OriginalQuestion,
			DetailedQuestion: article.DetailedQuestion,
			ConciseAnswer:    article.ConciseAnswer,
			Tags:             article.Tags,
			CreatedAt:        article.CreatedAt.Format("2006-01-02 15:04:05"),
			Similarity:       &similarity,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  responses,
		"query": req.Query,
	})
}

// GetArticle 获取单篇文章详情
func (h *Handler) GetArticle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}

	article, err := h.repo.GetArticleByID(c.Request.Context(), uint(id))
	if err != nil {
		slog.Error("GetArticleByID error", "error", err, "id", id)
		c.JSON(http.StatusNotFound, gin.H{"error": "article not found"})
		return
	}

	response := ArticleResponse{
		ID:               article.ID,
		OriginalQuestion: article.OriginalQuestion,
		DetailedQuestion: article.DetailedQuestion,
		ConciseAnswer:    article.ConciseAnswer,
		Tags:             article.Tags,
		CreatedAt:        article.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// GetAllTags 获取所有 tag 列表
func (h *Handler) GetAllTags(c *gin.Context) {
	tags, err := h.repo.GetAllTags(c.Request.Context())
	if err != nil {
		slog.Error("GetAllTags error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tags"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": tags,
	})
}

// CreateTaskRequest 创建任务请求参数
type CreateTaskRequest struct {
	RawQuestions string                 `json:"raw_questions" binding:"required"`
	Source       string                 `json:"source"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// CreateTask 创建新的处理任务
func (h *Handler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建任务
	task := processor.Task{
		RawQuestions: req.RawQuestions,
		Source:       req.Source,
		Metadata:     req.Metadata,
	}
	task.FillMetadata()

	err := h.taskProcessor.NewTask(c.Request.Context(), "enrich_questions", task)
	if err != nil {
		slog.Error("CreateTask error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "task created successfully",
		"task_id": task.TaskID,
		"data": gin.H{
			"task_id":    task.TaskID,
			"source":     task.Source,
			"created_at": task.CreatedAt.Format("2006-01-02 15:04:05"),
		},
	})
}

// FindSimilarArticles 根据文章 ID 查找相似文章
func (h *Handler) FindSimilarArticles(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid article id"})
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
		return
	}

	articles, err := h.repo.FindSimilarBySourceID(c.Request.Context(), uint(id), limit)
	if err != nil {
		slog.Error("FindSimilarBySourceID error", "error", err, "id", id)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	responses := make([]ArticleResponse, len(articles))
	for i, article := range articles {
		responses[i] = ArticleResponse{
			ID:               article.ID,
			OriginalQuestion: article.OriginalQuestion,
			DetailedQuestion: article.DetailedQuestion,
			ConciseAnswer:    article.ConciseAnswer,
			Tags:             article.Tags,
			CreatedAt:        article.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":      responses,
		"source_id": uint(id),
		"limit":     limit,
	})
}
