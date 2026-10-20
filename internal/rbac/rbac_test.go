package rbac

import (
	"testing"

	"github.com/trustline/trustline/internal/domain"
)

func TestMatrix(t *testing.T) {
	cases := []struct {
		role domain.Role
		perm Permission
		want bool
	}{
		{domain.RoleAdmin, MembersWrite, true},
		{domain.RoleAdmin, OrgWrite, true},
		{domain.RoleEditor, MembersWrite, false},
		{domain.RoleEditor, ControlsWrite, true},
		{domain.RoleAuditor, ControlsRead, true},
		{domain.RoleAuditor, ControlsWrite, false},
		{domain.RoleAuditor, MembersWrite, false},
		{domain.RoleAuditor, AuditsRequest, true},
		{domain.RoleEmployee, ControlsRead, false},
		{domain.RoleEmployee, PoliciesAccept, true},
		{domain.Role("bogus"), OrgRead, false},
	}
	for _, c := range cases {
		if got := Can(c.role, c.perm); got != c.want {
			t.Errorf("Can(%s,%s)=%v want %v", c.role, c.perm, got, c.want)
		}
	}
}

func TestAuditorIsReadOnly(t *testing.T) {
	for p := range Matrix[domain.RoleAuditor] {
		if p == AuditsRequest {
			continue
		}
		for _, w := range writeAll {
			if p == w {
				t.Errorf("auditor must not hold write permission %s", p)
			}
		}
		if p == OrgWrite || p == MembersWrite {
			t.Errorf("auditor must not hold %s", p)
		}
	}
}
