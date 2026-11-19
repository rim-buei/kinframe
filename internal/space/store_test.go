package space

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/rim-buei/kinframe/internal/db"
)

func setupStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
INSERT INTO accounts (id, provider, provider_subject, email, display_name) VALUES
 ('a1', 'google', 's1', 'a1@example.com', 'A One'),
 ('a2', 'google', 's2', 'a2@example.com', 'A Two')
`)
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(database), "a1", "a2"
}

func TestCreateListInviteAccept(t *testing.T) {
	ctx := context.Background()
	st, owner, guest := setupStore(t)

	sw, err := st.CreateSpace(ctx, "Family", owner)
	if err != nil {
		t.Fatal(err)
	}
	if sw.Role != RoleOwner || sw.Space.Slug != "family" {
		t.Fatalf("unexpected: %+v", sw)
	}

	list, err := st.ListSpacesForAccount(ctx, owner)
	if err != nil || len(list) != 1 {
		t.Fatalf("list owner: %v %d", err, len(list))
	}
	if _, err := st.ListSpacesForAccount(ctx, guest); err != nil {
		t.Fatal(err)
	}

	created, err := st.CreateInvite(ctx, sw.Space.ID, owner, RoleEditor, nil, nil)
	if err != nil || created.Token == "" {
		t.Fatalf("invite: %+v %v", created, err)
	}

	sp, mem, err := st.AcceptInvite(ctx, created.Token, guest)
	if err != nil {
		t.Fatal(err)
	}
	if sp.ID != sw.Space.ID || mem.Role != RoleEditor {
		t.Fatalf("accept: %+v %+v", sp, mem)
	}

	// Upgrade-only: viewer invite should not demote editor.
	inv2, err := st.CreateInvite(ctx, sw.Space.ID, owner, RoleViewer, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, mem2, err := st.AcceptInvite(ctx, inv2.Token, guest)
	if err != nil {
		t.Fatal(err)
	}
	if mem2.Role != RoleEditor {
		t.Fatalf("demoted to %s", mem2.Role)
	}

	if _, _, err := st.RequireMembership(ctx, sw.Space.Slug, guest, RoleEditor); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RequireMembership(ctx, sw.Space.Slug, guest, RoleAdmin); err != ErrForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestRevokeAndAuthz(t *testing.T) {
	ctx := context.Background()
	st, owner, guest := setupStore(t)
	sw, err := st.CreateSpace(ctx, "S", owner)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := st.CreateInvite(ctx, sw.Space.ID, owner, RoleViewer, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeInvite(ctx, sw.Space.ID, inv.Invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.AcceptInvite(ctx, inv.Token, guest); err != ErrInviteGone {
		t.Fatalf("want gone, got %v", err)
	}

	if err := st.SoftDeleteSpace(ctx, sw.Space.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RequireMembership(ctx, sw.Space.ID, owner, RoleOwner); err != ErrNotFound {
		t.Fatalf("deleted space: %v", err)
	}
}
