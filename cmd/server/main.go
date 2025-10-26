package main

import (
	"context"
	"log/slog"
	"os"
	"paguu/configs"
	"paguu/internal/api"
	"paguu/internal/embedding"
	"paguu/internal/enrich"
	"paguu/internal/processor"
	"paguu/internal/storage/postgres"
	"time"

	"github.com/lmittmann/tint"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"google.golang.org/genai"
)

func main() {
	// 设置日志
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.Kitchen,
		AddSource:  false,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 加载配置
	config, err := configs.LoadConfig()
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		panic(err)
	}

	// 初始化数据库
	repo, err := postgres.NewRepository(config.Database.DSN)
	if err != nil {
		slog.Error("数据库初始化失败", "error", err)
		panic(err)
	}
	slog.Info("数据库连接成功")

	// 初始化 Gemini 客户端
	ctx := context.Background()
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  config.Gemini.ApiKey,
		Backend: genai.BackendVertexAI,
	})
	if err != nil {
		slog.Error("创建 Gemini 客户端失败", "error", err)
		panic(err)
	}
	slog.Info("Gemini 客户端初始化成功")

	// 初始化 Embedder
	embedder, err := embedding.NewEmbedder(config.Gemini.EmbeddingModel, geminiClient)
	if err != nil {
		slog.Error("Embedder 初始化失败", "error", err)
		panic(err)
	}

	// 初始化 Volcengine Ark 客户端（用于问题丰富化）
	arkClient := arkruntime.NewClientWithApiKey(
		config.Ark.ApiKey,
		arkruntime.WithBaseUrl(config.Ark.BaseUrl),
	)

	// 初始化 QuestionsEnricher
	questionEnricher, err := enrich.NewQuestionsEnricher(arkClient, config.Ark.EnrichTemplatePath, config.Ark.EnrichModel)
	if err != nil {
		slog.Error("QuestionsEnricher 初始化失败", "error", err)
		panic(err)
	}

	// 创建 TaskProcessor
	taskProcessor := processor.NewTaskProcessor(questionEnricher, repo, embedder)

	// 创建 API Handler
	apiHandler := api.NewHandler(repo, embedder, taskProcessor)

	// 设置路由
	router := api.SetupRouter(apiHandler)

	// 启动服务器
	port := ":8080"
	slog.Info("API 服务器启动", "port", port)
	if err := router.Run(port); err != nil {
		slog.Error("服务器启动失败", "error", err)
		panic(err)
	}
}
