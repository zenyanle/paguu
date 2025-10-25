package enrich

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

/*
使用示例：
var result InterviewQuestionSet
err := json.Unmarshal(llmResponseBytes, &result)
*/
