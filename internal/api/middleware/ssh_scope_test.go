package middleware

import "testing"

func TestScopesForRoleIncludesSSH(t *testing.T) {
	withSSH := []AuthRole{AuthRoleAPI, AuthRoleOperator, AuthRoleAdmin, AuthRoleTenantAdmin}
	for _, role := range withSSH {
		id := AuthIdentity{Role: role, Scopes: scopesForRole(role)}
		if !id.HasScope(ScopeSSH) {
			t.Errorf("role %q should carry %q", role, ScopeSSH)
		}
	}

	withoutSSH := []AuthRole{AuthRoleViewer, AuthRoleWorker}
	for _, role := range withoutSSH {
		id := AuthIdentity{Role: role, Scopes: scopesForRole(role)}
		if id.HasScope(ScopeSSH) {
			t.Errorf("role %q should not carry %q", role, ScopeSSH)
		}
	}
}
