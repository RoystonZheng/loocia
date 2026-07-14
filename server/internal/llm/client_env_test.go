package llm

import "testing"

func TestNewTermsClientFromEnvDefaults(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "k")
	t.Setenv("AIHOT_TERMS_MODEL", "")
	c, err := NewTermsClientFromEnv()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.model != "deepseek-v4-flash" {
		t.Fatalf("default model: %q", c.model)
	}

	t.Setenv("AIHOT_TERMS_MODEL", "auto-mini")
	c, err = NewTermsClientFromEnv()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c.model != "auto-mini" {
		t.Fatalf("override model: %q", c.model)
	}
}

func TestNewTermsClientFromEnvRequiresKey(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "")
	if _, err := NewTermsClientFromEnv(); err == nil {
		t.Fatal("expected error without AIHOT_LLM_API_KEY")
	}
}
