package space

import "testing"

func TestRoleOrdering(t *testing.T) {
	if !RoleAdmin.AtLeast(RoleViewer) || RoleViewer.AtLeast(RoleAdmin) {
		t.Fatal("admin should outrank viewer")
	}
	if !RoleEditor.HigherThan(RoleViewer) || RoleViewer.HigherThan(RoleEditor) {
		t.Fatal("editor should be higher than viewer")
	}
	if RoleOwner.CanDeleteSpace() != true || RoleAdmin.CanDeleteSpace() {
		t.Fatal("only owner deletes space")
	}
	if !RoleAdmin.CanCreateInvite() || RoleEditor.CanCreateInvite() {
		t.Fatal("invite is owner/admin")
	}
}

func TestParseInviteRole(t *testing.T) {
	if _, err := ParseInviteRole("owner"); err == nil {
		t.Fatal("owner invite should fail")
	}
	r, err := ParseInviteRole("Editor")
	if err != nil || r != RoleEditor {
		t.Fatalf("got %v %v", r, err)
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify(" Family Photos "); got != "family-photos" {
		t.Fatalf("got %q", got)
	}
	if got := Slugify("@@@"); got != "space" {
		t.Fatalf("got %q", got)
	}
}
