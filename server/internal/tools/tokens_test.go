package tools

import "testing"

func TestNormalizeGitHubTokenCredentialsPreservesMetadataAndGeneratesStableIDs(t *testing.T) {
	got := NormalizeGitHubTokenCredentials([]GitHubTokenCredential{
		{
			Name:        "  主账号 ",
			Description: " 日常发现 ",
			TestURL:     " https://api.github.com/rate_limit ",
			Token:       " ghp_secret ",
		},
		{
			Name:  "重复账号",
			Token: "ghp_secret",
		},
	})
	if len(got) != 1 {
		t.Fatalf("credential count = %d, want 1", len(got))
	}
	if got[0].ID != GitHubTokenCredentialID("ghp_secret") {
		t.Fatalf("generated id = %q", got[0].ID)
	}
	if got[0].Name != "主账号" || got[0].Description != "日常发现" || got[0].TestURL != "https://api.github.com/rate_limit" {
		t.Fatalf("metadata was not normalized: %+v", got[0])
	}
}

func TestDecodeGitHubTokenCredentialsSupportsLegacyValues(t *testing.T) {
	legacy, err := decodeGitHubTokenCredentials(`["legacy-token"]`)
	if err != nil {
		t.Fatalf("decode legacy credentials: %v", err)
	}
	if len(legacy) != 1 || legacy[0].Token != "legacy-token" || legacy[0].Name == "" || legacy[0].ID == "" {
		t.Fatalf("legacy credential conversion = %+v", legacy)
	}

	structured, err := decodeGitHubTokenCredentials(`[{"id":"token-1","name":"主账号","description":"daily","testUrl":"https://example.com","token":"new-token"}]`)
	if err != nil {
		t.Fatalf("decode structured credentials: %v", err)
	}
	if len(structured) != 1 || structured[0].ID != "token-1" || structured[0].Name != "主账号" || structured[0].Token != "new-token" {
		t.Fatalf("structured credential conversion = %+v", structured)
	}
}
