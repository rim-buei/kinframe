package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/rim-buei/kinframe/internal/db"
)

func TestSessionCreateLoadRevoke(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}

	_, err = database.ExecContext(ctx, `
INSERT INTO accounts (id, provider, provider_subject, email, display_name)
VALUES ('acc1', 'google', 'sub1', 'a@example.com', 'A')
`)
	if err != nil {
		t.Fatal(err)
	}

	sealer, err := NewSealer("unit-test-session-secret")
	if err != nil {
		t.Fatal(err)
	}
	sm := NewSessionManager(database, sealer, time.Hour, false)

	rec := httptest.NewRecorder()
	sess, err := sm.Create(ctx, rec, "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccountID != "acc1" {
		t.Fatalf("account id: %s", sess.AccountID)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName {
		t.Fatalf("cookies: %+v", cookies)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookies[0])
	acct, loaded, err := sm.LoadAccount(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if acct.Email != "a@example.com" || loaded.ID != sess.ID {
		t.Fatalf("loaded %+v %+v", acct, loaded)
	}

	rec2 := httptest.NewRecorder()
	if err := sm.Revoke(ctx, rec2, req); err != nil {
		t.Fatal(err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(cookies[0])
	if _, _, err := sm.LoadAccount(ctx, req2); err != ErrNoSession {
		t.Fatalf("want ErrNoSession after revoke, got %v", err)
	}
}

func TestUpsertAccount(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(ctx, database); err != nil {
		t.Fatal(err)
	}

	a1, err := UpsertAccount(ctx, database, Identity{
		Provider: ProviderGoogle, ProviderSubject: "sub-x",
		Email: "one@example.com", DisplayName: "One",
	})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := UpsertAccount(ctx, database, Identity{
		Provider: ProviderGoogle, ProviderSubject: "sub-x",
		Email: "two@example.com", DisplayName: "Two",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a1.ID != a2.ID {
		t.Fatalf("expected same id, got %s vs %s", a1.ID, a2.ID)
	}
	if a2.Email != "two@example.com" || a2.DisplayName != "Two" {
		t.Fatalf("update failed: %+v", a2)
	}
}
