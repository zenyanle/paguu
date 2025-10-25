package main

import (
	"context"
	"log/slog"
	"os"
	"paguu/configs"
	"paguu/internal/enrich"
	"time"

	"github.com/lmittmann/tint"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
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
		slog.Error("加载配置失败: %v", err)
		panic(err)
	}

	client := arkruntime.NewClientWithApiKey(
		config.Ark.ApiKey,
		arkruntime.WithBaseUrl(config.Ark.BaseUrl),
	)

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

	qe, err := enrich.NewQuestionsEnricher(client, "./prompts/enrich_questions.txt", config.Ark.Model)
	if err != nil {
		slog.Error("QuestionsEnricher init error : ", err)
		panic(err)
	}
	_, err = qe.EnrichQuestions(context.TODO(), testQuestions)
	if err != nil {
		slog.Error("QuestionsEnricher enrich error : ", err)
	}

	return
}
