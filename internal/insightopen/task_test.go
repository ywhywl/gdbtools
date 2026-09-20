package insightopen

import (
	"testing"
)

func TestPollTaskResultReportsProgress(t *testing.T) {
	var progress []PollProgress
	callback := func(item PollProgress) { progress = append(progress, item) }
	notifyPollProgress(callback, PollProgress{Attempt: 1, Data: map[string]any{"process": 35, "totalResult": 0}})
	notifyPollProgress(callback, PollProgress{Attempt: 2, Done: true, Data: map[string]any{"process": 100, "totalResult": 1}})
	if len(progress) != 2 || progress[0].Done || !progress[1].Done || progress[0].Attempt != 1 || progress[1].Attempt != 2 {
		t.Fatalf("unexpected progress callbacks: %#v", progress)
	}
}
