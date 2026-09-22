package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// GitHubTokenCredential stores token metadata alongside the secret used by the runtime.
type GitHubTokenCredential struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TestURL     string `json:"testUrl,omitempty"`
	Token       string `json:"token"`
}

func NormalizeGitHubTokenCredentials(credentials []GitHubTokenCredential) []GitHubTokenCredential {
	out := make([]GitHubTokenCredential, 0, len(credentials))
	seen := map[string]bool{}
	for _, credential := range credentials {
		credential.Token = strings.TrimSpace(credential.Token)
		if credential.Token == "" || seen[credential.Token] {
			continue
		}
		credential.ID = strings.TrimSpace(credential.ID)
		if credential.ID == "" {
			credential.ID = GitHubTokenCredentialID(credential.Token)
		}
		credential.Name = strings.TrimSpace(credential.Name)
		if credential.Name == "" {
			credential.Name = fmt.Sprintf("GitHub Token %d", len(out)+1)
		}
		credential.Description = strings.TrimSpace(credential.Description)
		credential.TestURL = strings.TrimSpace(credential.TestURL)
		seen[credential.Token] = true
		out = append(out, credential)
	}
	return out
}

func GitHubTokenCredentialsFromValues(tokens []string) []GitHubTokenCredential {
	credentials := make([]GitHubTokenCredential, 0, len(tokens))
	for _, token := range NormalizeGitHubTokens(tokens) {
		credentials = append(credentials, GitHubTokenCredential{Token: token})
	}
	return NormalizeGitHubTokenCredentials(credentials)
}

func GitHubTokenValues(credentials []GitHubTokenCredential) []string {
	credentials = NormalizeGitHubTokenCredentials(credentials)
	out := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		out = append(out, credential.Token)
	}
	return out
}

func GitHubTokenCredentialID(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return "token-" + hex.EncodeToString(sum[:8])
}
