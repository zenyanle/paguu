// 位于: internal/embedding/embedder.go
package embedding

import (
	"context"
	"fmt"
	"math"

	"google.golang.org/genai"
)

const DIMENSION = 1536

type Embedder struct {
	client    *genai.Client
	modelName string
}

func NewEmbedder(modelName string, client *genai.Client) (*Embedder, error) {
	// 验证依赖
	if client == nil {
		return nil, fmt.Errorf("genai client is required")
	}
	if modelName == "" {
		return nil, fmt.Errorf("embedding model name is required")
	}

	return &Embedder{
		client:    client,
		modelName: modelName,
	}, nil
}

func (e *Embedder) Embed(ctx context.Context, text string) ([]float32, error) {
	batchResults, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(batchResults) == 0 {
		return nil, fmt.Errorf("embedding API returned no vectors")
	}
	return batchResults[0], nil
}

func (e *Embedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// 将文本转换为 genai.Content 格式
	contents := make([]*genai.Content, len(texts))
	for i, text := range texts {
		contents[i] = &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(text)},
		}
	}

	// 调用 Gemini Embedding API，指定输出维度
	outputDim := int32(DIMENSION)
	result, err := e.client.Models.EmbedContent(ctx, e.modelName, contents, &genai.EmbedContentConfig{
		OutputDimensionality: &outputDim,
		TaskType:             "RETRIEVAL_DOCUMENT", // 用于文档检索的嵌入
	})
	if err != nil {
		return nil, fmt.Errorf("gemini embedding API error: %w", err)
	}

	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("API response count (%d) does not match input count (%d)", len(result.Embeddings), len(texts))
	}

	results := make([][]float32, len(result.Embeddings))
	for i, embedding := range result.Embeddings {
		results[i] = e.normalize(embedding.Values)
	}

	return results, nil
}

// normalize 对向量进行 L2 归一化 (使其长度为 1)
// 这是使用 pgvector 内积 (<#>) 查询所必需的
func (e *Embedder) normalize(v []float32) []float32 {
	var norm float64
	for _, val := range v {
		norm += float64(val) * float64(val)
	}
	norm = math.Sqrt(norm)

	if norm == 0 {
		return v
	}

	normalized := make([]float32, len(v))
	for i, val := range v {
		normalized[i] = val / float32(norm)
	}
	return normalized
}
