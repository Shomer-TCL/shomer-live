package app

// NewTestQuestion makes a question for front-end tests.
func NewTestQuestion(k QKind, prompt string, def bool) *Question {
	return &Question{Kind: k, Prompt: prompt, Default: def, ch: make(chan Answer, 1)}
}

// TestAnswer reads the answer given to a test question.
func TestAnswer(q *Question) Answer { return <-q.ch }
