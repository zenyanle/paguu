package configs

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Ark struct {
		ApiKey             string `mapstructure:"api_key"`
		BaseUrl            string `mapstructure:"base_url"`
		EnrichModel        string `mapstructure:"enrich_model"`
		EnrichTemplatePath string `mapstructure:"enrich_template_path"`
		EmbeddingModel     string `mapstructure:"embedding_model"` // 已废弃，改用 Gemini
	} `mapstructure:"ark"`
	Gemini struct {
		ApiKey         string `mapstructure:"api_key"`
		EmbeddingModel string `mapstructure:"embedding_model"`
	} `mapstructure:"gemini"`
	Database struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"database"`
}

// loadConfig 函数负责使用 viper 加载配置
func LoadConfig() (config Config, err error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	err = viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Println("警告：未找到 config.yaml 文件，将完全依赖环境变量。")
		} else {
			return Config{}, fmt.Errorf("解析 config.yaml 出错: %w", err)
		}
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return Config{}, fmt.Errorf("反序列化配置出错: %w", err)
	}

	return config, nil
}
