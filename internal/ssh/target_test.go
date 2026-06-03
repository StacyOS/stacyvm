package ssh

import "testing"

func TestParseSandboxTarget(t *testing.T) {
	tests := []struct {
		user       string
		wantSB     string
		wantTenant string
	}{
		{"sb-08f3a1b2", "sb-08f3a1b2", ""},
		{"sb-08f3a1b2.tenant-acme", "sb-08f3a1b2", "tenant-acme"},
		{"  sb-08f3a1b2  ", "sb-08f3a1b2", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		gotSB, gotTenant := parseSandboxTarget(tt.user)
		if gotSB != tt.wantSB || gotTenant != tt.wantTenant {
			t.Errorf("parseSandboxTarget(%q) = (%q,%q), want (%q,%q)",
				tt.user, gotSB, gotTenant, tt.wantSB, tt.wantTenant)
		}
	}
}
