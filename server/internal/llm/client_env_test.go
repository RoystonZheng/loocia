package llm

import "testing"

func TestAllEnvClientsUseDeepSeekFlash(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "k")
	t.Setenv("AIHOT_LLM_MODEL", "auto-max")
	t.Setenv("AIHOT_TRANSLATE_MODEL", "auto-max")
	t.Setenv("AIHOT_TERMS_MODEL", "auto-mini")
	t.Setenv("AIHOT_TRANSLATE_RETRY_MODEL", "auto-std")
	constructors := []struct {
		name string
		fn   func() (*Client, error)
	}{
		{"main", NewClientFromEnv},
		{"translate", NewTranslateClientFromEnv},
		{"retry translate", NewRetryTranslateClientFromEnv},
		{"terms", NewTermsClientFromEnv},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.fn()
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if c.model != defaultModel {
				t.Fatalf("model: %q", c.model)
			}
		})
	}
}

func TestNewTermsClientFromEnvRequiresKey(t *testing.T) {
	t.Setenv("AIHOT_LLM_API_KEY", "")
	if _, err := NewTermsClientFromEnv(); err == nil {
		t.Fatal("expected error without AIHOT_LLM_API_KEY")
	}
}
