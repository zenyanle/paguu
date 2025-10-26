package enrich

import "strings"

type InterviewQuestionSet struct {
	Questions []InterviewQuestion `json:"questions"`
}

// InterviewQuestion 对应列表中的每一个被处理过的问题
type InterviewQuestion struct {
	OriginalQuestion string `json:"original_question"`

	DetailedQuestion string `json:"detailed_question"`

	ConciseAnswer string `json:"concise_answer"`

	Tags []string `json:"tags"`
}

func (set *InterviewQuestionSet) GetEmbeddableTexts() []string {
	texts := make([]string, 0, len(set.Questions))

	var sb strings.Builder

	for _, q := range set.Questions {
		sb.Reset()

		sb.WriteString(q.DetailedQuestion)

		sb.WriteString("\n\n")

		sb.WriteString(q.ConciseAnswer)

		texts = append(texts, sb.String())
	}

	return texts
}

/*
使用示例：
var result InterviewQuestionSet
err := json.Unmarshal(llmResponseBytes, &result)
*/
