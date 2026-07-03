package version

// PublicVersion mirrors components.schemas.PublicVersion in docs/references/openapi.yaml.
type PublicVersion struct {
	APIVersion    string   `json:"apiVersion"`
	SkillVersion  string   `json:"skillVersion"`
	UpdatedAt     string   `json:"updatedAt"`
	ChangelogURL  string   `json:"changelogUrl"`
	RecentChanges []string `json:"recentChanges"`
}

// Current returns the version this build advertises.
func Current() PublicVersion {
	return PublicVersion{
		APIVersion:    "1.1.0",
		SkillVersion:  "0.1.0",
		UpdatedAt:     "2026-07-03",
		ChangelogURL:  "/changelog",
		RecentChanges: []string{"internal aihot walking skeleton"},
	}
}
