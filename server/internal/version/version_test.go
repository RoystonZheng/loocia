package version

import "testing"

func TestCurrentHasRequiredFields(t *testing.T) {
	v := Current()
	if v.APIVersion == "" {
		t.Error("APIVersion empty")
	}
	if v.SkillVersion == "" {
		t.Error("SkillVersion empty")
	}
	if v.ChangelogURL == "" {
		t.Error("ChangelogURL empty")
	}
}
