package db

import "testing"

func TestSecurityWorkflowSkillVisibilityDefaults(t *testing.T) {
	want := map[string][]string{
		"apk-recon":      {"auto", "pentest", "worker"},
		"chains-meta":    {"auto", "pentest", "planner", "worker"},
		"cloud-security": {"auto", "pentest", "worker"},
		"资金盘":            {"auto", "pentest", "worker"},
	}
	for skill, agents := range want {
		got, ok := securityWorkflowSkillVisibility[skill]
		if !ok {
			t.Fatalf("missing visibility defaults for %s", skill)
		}
		if len(got) != len(agents) {
			t.Fatalf("%s agents=%v, want %v", skill, got, agents)
		}
		for i := range agents {
			if got[i] != agents[i] {
				t.Fatalf("%s agents=%v, want %v", skill, got, agents)
			}
		}
	}
	if len(securityWorkflowSkillVisibility) != 17 {
		t.Fatalf("visibility defaults cover %d skills, want 17", len(securityWorkflowSkillVisibility))
	}
}
