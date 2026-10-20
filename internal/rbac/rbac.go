// Package rbac defines permissions and the role → permission matrix.
package rbac

import "github.com/trustline/trustline/internal/domain"

type Permission string

// Permissions are coarse per module; finer ones are added as modules land.
const (
	OrgRead        Permission = "org:read"
	OrgWrite       Permission = "org:write"
	MembersRead    Permission = "members:read"
	MembersWrite   Permission = "members:write"
	ActivityRead   Permission = "activity:read"
	SessionsManage Permission = "sessions:manage"

	// Phase 1+
	ControlsRead      Permission = "controls:read"
	ControlsWrite     Permission = "controls:write"
	IntegrationsRead  Permission = "integrations:read"
	IntegrationsWrite Permission = "integrations:write"
	TestsRead         Permission = "tests:read"
	TestsWrite        Permission = "tests:write"
	EvidenceRead      Permission = "evidence:read"
	EvidenceWrite     Permission = "evidence:write"
	PoliciesRead      Permission = "policies:read"
	PoliciesWrite     Permission = "policies:write"
	PoliciesAccept    Permission = "policies:accept"
	RisksRead         Permission = "risks:read"
	RisksWrite        Permission = "risks:write"
	VendorsRead       Permission = "vendors:read"
	VendorsWrite      Permission = "vendors:write"
	PeopleRead        Permission = "people:read"
	PeopleWrite       Permission = "people:write"
	TasksOwn          Permission = "tasks:own"
	AuditsRead        Permission = "audits:read"
	AuditsWrite       Permission = "audits:write"
	AuditsRequest     Permission = "audits:request"
	TrustRead         Permission = "trust:read"
	TrustWrite        Permission = "trust:write"
	AIUse             Permission = "ai:use"
)

var readAll = []Permission{
	OrgRead, MembersRead, ActivityRead,
	ControlsRead, IntegrationsRead, TestsRead, EvidenceRead, PoliciesRead,
	RisksRead, VendorsRead, PeopleRead, AuditsRead, TrustRead,
}

var writeAll = []Permission{
	ControlsWrite, IntegrationsWrite, TestsWrite, EvidenceWrite, PoliciesWrite,
	RisksWrite, VendorsWrite, PeopleWrite, AuditsWrite, TrustWrite, AIUse,
}

// Matrix maps each role to its permission set.
var Matrix = map[domain.Role]map[Permission]bool{
	domain.RoleAdmin:    set(readAll, writeAll, []Permission{OrgWrite, MembersWrite, SessionsManage, PoliciesAccept, TasksOwn}),
	domain.RoleEditor:   set(readAll, writeAll, []Permission{PoliciesAccept, TasksOwn}),
	domain.RoleAuditor:  set(readAll, []Permission{AuditsRequest}),
	domain.RoleEmployee: set([]Permission{OrgRead, PoliciesRead, PoliciesAccept, TasksOwn}),
}

func set(groups ...[]Permission) map[Permission]bool {
	m := map[Permission]bool{}
	for _, g := range groups {
		for _, p := range g {
			m[p] = true
		}
	}
	return m
}

// Can reports whether role holds perm.
func Can(role domain.Role, perm Permission) bool {
	return Matrix[role][perm]
}

// PermissionsOf returns the sorted permission list for a role (for the UI).
func PermissionsOf(role domain.Role) []string {
	m := Matrix[role]
	out := make([]string, 0, len(m))
	for p := range m {
		out = append(out, string(p))
	}
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
