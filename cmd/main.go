package main

import (
	"context"
	"log/slog"
	"os"
	"paguu/configs"
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
	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelDebug, // 设置日志级别
		TimeFormat: time.Kitchen,    // 优化时间显示
		AddSource:  true,            // 可选：显示源码位置
	})

	logger := slog.New(handler)

	slog.SetDefault(logger)

	config, err := configs.LoadConfig()
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		panic(err)
	}

	// 创建 Volcengine Ark 客户端（用于问题丰富化）
	arkClient := arkruntime.NewClientWithApiKey(
		config.Ark.ApiKey,
		arkruntime.WithBaseUrl(config.Ark.BaseUrl),
	)

	// 创建 Google Gemini 客户端（用于嵌入）
	ctx := context.Background()
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  config.Gemini.ApiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		slog.Error("创建 Gemini 客户端失败", "error", err)
		panic(err)
	}

	testQuestions := `
2. 项目引发：io.ReadAll和io.Copy的区别
3. slice的扩容机制
4. golang的GC
5. GC触发条件
6. 除了io.ReadAll改io.Copy，还有什么内存优化方案
7. 高频创建小结构体的情况，有什么优化方案
8. sync.Pool的具体实现
9. go程序如何定位哪里需要做内存优化
10. MySQL慢查询优化
11. 业务预期有一个写多读多的MySQL表，在建表时有什么规范吗，只考虑单表情况
12. 自增主键有什么好处
`

	questionEnricher, err := enrich.NewQuestionsEnricher(arkClient, config.Ark.EnrichTemplatePath, config.Ark.EnrichModel)
	if err != nil {
		slog.Error("QuestionsEnricher init error", "error", err)
		panic(err)
	}
	/*	_, err = questionEnricher.EnrichQuestions(context.TODO(), testQuestions)
		if err != nil {
			slog.Error("QuestionsEnricher enrich error : ", err)
		}*/

	repo, err := postgres.NewRepository(config.Database.DSN)
	if err != nil {
		slog.Error("repo init error", "error", err)
		panic(err)
	}

	embedder, err := embedding.NewEmbedder(config.Gemini.EmbeddingModel, geminiClient)
	if err != nil {
		slog.Error("embedder init error", "error", err)
		panic(err)
	}

	taskProcessor := processor.NewTaskProcessor(questionEnricher, repo, embedder)

	task := processor.Task{
		RawQuestions: testQuestions,
	}
	task.FillMetadata()
	err = taskProcessor.NewTask(context.TODO(), "default", task)
	if err != nil {
		slog.Error("NewTask error", "error", err)
		return
	}

	err = taskProcessor.ProcessNextTask(context.TODO())
	if err != nil {
		slog.Error("ProcessNextTask error", "error", err)
	}
}
