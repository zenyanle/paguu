package enrich

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"text/template"

	"github.com/volcengine/volcengine-go-sdk/service/arkruntime"
	"github.com/volcengine/volcengine-go-sdk/service/arkruntime/model/responses"
)

func loadTemplate(templatePath string) (*template.Template, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(templatePath).Parse(string(content))
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

type QuestionsEnricher struct {
	modelName      string
	client         *arkruntime.Client
	templatePath   string
	promptTemplate *template.Template
}

func NewQuestionsEnricher(client *arkruntime.Client, templatePath string, modelName string) (*QuestionsEnricher, error) {
	tp, err := loadTemplate(templatePath)
	if err != nil {
		return nil, err
	}
	return &QuestionsEnricher{
		templatePath:   templatePath,
		promptTemplate: tp,
		client:         client,
		modelName:      modelName,
	}, nil
}

func (qe *QuestionsEnricher) EnrichQuestions(ctx context.Context, questions string) (InterviewQuestionSet, error) {
	data := map[string]string{
		"InputText": questions,
	}

	var buf bytes.Buffer

	err := qe.promptTemplate.Execute(&buf, data)
	if err != nil {
		return InterviewQuestionSet{}, err
	}

	promptString := buf.String()
	resp, err := qe.client.CreateResponses(ctx, &responses.ResponsesRequest{
		Model: qe.modelName,
		Input: &responses.ResponsesInput{Union: &responses.ResponsesInput_StringValue{StringValue: promptString}},
	})
	if err != nil {
		fmt.Printf("response error: %v\n", err)
		return InterviewQuestionSet{}, err
	}

	// Extract JSON text content from the response and unmarshal
	text := extractTextFromSingleResponse(resp)
	if strings.TrimSpace(text) == "" {
		return InterviewQuestionSet{}, fmt.Errorf("empty response text")
	}

	slog.Info(text)

	var questionSet InterviewQuestionSet
	if err := json.Unmarshal([]byte(text), &questionSet); err != nil {
		return InterviewQuestionSet{}, fmt.Errorf("failed to unmarshal response JSON: %w", err)
	}

	return questionSet, nil
}

func extractTextFromSingleResponse(resp *responses.ResponseObject) string {
	if resp == nil {
		return ""
	}

	if len(resp.Output) > 0 {
		var b strings.Builder
		for _, item := range resp.Output {
			if item == nil {
				continue
			}
			if msg, ok := item.GetUnion().(*responses.OutputItem_OutputMessage); ok {
				if msg.OutputMessage != nil && msg.OutputMessage.Content != nil {
					for _, c := range msg.OutputMessage.Content {
						if c == nil {
							continue
						}
						if t, ok := c.GetUnion().(*responses.OutputContentItem_Text); ok {
							if t.Text != nil {
								b.WriteString(t.Text.Text)
							}
						}
					}
				}
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
	}

	// Fallback: no recognized output text
	return ""
}
