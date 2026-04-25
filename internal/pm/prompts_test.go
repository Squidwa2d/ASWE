package pm

import (
	"strings"
	"testing"
)

func TestBuildClarifyPrompt_DoesNotExposeMinTurns(t *testing.T) {
	prompt := buildClarifyPrompt([]qaPair{{Role: "user", Content: "做一个任务管理工具"}}, true)

	for _, forbidden := range []string{"至少", "最少", "第 1/3", "3 轮"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt should not expose turn threshold or READY in forced mode, found %q in:\n%s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "STATUS: NEEDS_MORE_INFO") {
		t.Fatalf("prompt should ask for structured needs-more-info output:\n%s", prompt)
	}
}

func TestParseClarifyResponse_StructuredStatus(t *testing.T) {
	questions, done := parseClarifyResponse(`STATUS: NEEDS_MORE_INFO
MISSING:
- 验收标准不明确
QUESTIONS:
- 什么样算完成?
- 需要哪些权限?`)
	if done {
		t.Fatal("NEEDS_MORE_INFO should not be done")
	}
	if len(questions) != 2 || questions[0] != "什么样算完成?" || questions[1] != "需要哪些权限?" {
		t.Fatalf("unexpected questions: %#v", questions)
	}

	questions, done = parseClarifyResponse("STATUS: READY")
	if !done || len(questions) != 0 {
		t.Fatalf("READY should be done without questions, got done=%v questions=%#v", done, questions)
	}
}
