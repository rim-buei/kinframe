// Package space implements Spaces, memberships, roles, and invite links.
package space

import (
	"fmt"
	"strings"
)

// Role is a Space membership role.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

var roleRank = map[Role]int{
	RoleViewer: 1,
	RoleEditor: 2,
	RoleAdmin:  3,
	RoleOwner:  4,
}

// ParseRole validates a role string.
func ParseRole(s string) (Role, error) {
	r := Role(strings.ToLower(strings.TrimSpace(s)))
	if _, ok := roleRank[r]; !ok {
		return "", fmt.Errorf("invalid role %q", s)
	}
	return r, nil
}

// ParseInviteRole validates roles allowed on invite links (not owner).
func ParseInviteRole(s string) (Role, error) {
	r, err := ParseRole(s)
	if err != nil {
		return "", err
	}
	if r == RoleOwner {
		return "", fmt.Errorf("invite cannot grant owner")
	}
	return r, nil
}

// AtLeast reports whether r is at least as privileged as min.
func (r Role) AtLeast(min Role) bool {
	return roleRank[r] >= roleRank[min]
}

// HigherThan reports whether r is strictly above other (for upgrade-only joins).
func (r Role) HigherThan(other Role) bool {
	return roleRank[r] > roleRank[other]
}

// CanManageMembers is true for owner and admin.
func (r Role) CanManageMembers() bool {
	return r.AtLeast(RoleAdmin)
}

// CanEditSpaceSettings is true for owner and admin.
func (r Role) CanEditSpaceSettings() bool {
	return r.AtLeast(RoleAdmin)
}

// CanDeleteSpace is owner-only.
func (r Role) CanDeleteSpace() bool {
	return r == RoleOwner
}

// CanCreateInvite is true for owner and admin.
func (r Role) CanCreateInvite() bool {
	return r.AtLeast(RoleAdmin)
}
