package api

import (
	"github.com/gin-gonic/gin"
)

// SetupRouter 设置路由
func SetupRouter(handler *Handler) *gin.Engine {
	r := gin.Default()

	// 添加 CORS 中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// 处理 OPTIONS 预检请求
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API v1 路由组
	v1 := r.Group("/api/v1")
	{
		// 文章相关
		articles := v1.Group("/articles")
		{
			articles.GET("", handler.ListArticles)                    // GET /api/v1/articles?page=1&page_size=20&tags[]=Go&tags[]=MySQL
			articles.GET("/:id", handler.GetArticle)                  // GET /api/v1/articles/123
			articles.GET("/:id/similar", handler.FindSimilarArticles) // GET /api/v1/articles/123/similar?limit=10
			articles.POST("/search", handler.VectorSearch)            // POST /api/v1/articles/search
		}

		// 任务相关
		tasks := v1.Group("/tasks")
		{
			tasks.POST("", handler.CreateTask) // POST /api/v1/tasks
		}

		// Tag 相关
		v1.GET("/tags", handler.GetAllTags) // GET /api/v1/tags
	}

	return r
}
