package judge

import (
	"strings"
	"testing"
)

func TestPromptTemplate_IsEmbedded(t *testing.T) {
	tpl := PromptTemplate()
	if !strings.Contains(tpl, "You are an expert website content auditor") {
		t.Fatal("expected canonical template content")
	}
}

func TestResponseSchema_IsEmbedded(t *testing.T) {
	s := ResponseSchema()
	if !strings.Contains(string(s), "primary_label") {
		t.Fatal("expected schema with primary_label")
	}
}
