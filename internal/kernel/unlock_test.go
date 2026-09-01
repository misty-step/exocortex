package kernel

import (
	"errors"
	"strings"
	"testing"
)

func TestAttachUnlockPreservesSuccessAndJoinsFailure(t *testing.T) {
	if got := attachUnlock(nil, errors.New("unlock"), "get", "notes/x.md"); got != nil {
		t.Fatalf("success became %s", got.Code)
	}
	conf := conflict("exists", "create", "notes/x.md", "retry", nil)
	got := attachUnlock(conf, errors.New("unlock"), "create", "notes/x.md")
	if got.Code != "exists" {
		t.Fatalf("primary failure replaced with %s", got.Code)
	}
	if got.Detail["unlock"] != "unlock" {
		t.Fatalf("unlock detail = %v", got.Detail)
	}
	if err := attachUnlockErr(nil, errors.New("unlock"), "register", "box"); err != nil {
		t.Fatalf("successful register became %v", err)
	}
}

func TestUnlockAfterLandedPutIsWarningNotConflict(t *testing.T) {
	f := newFixture(t)
	releaseHook = func() error { return errors.New("injected unlock") }
	t.Cleanup(func() { releaseHook = nil })
	res, conf := f.put("hosta", "notes/unlock.md", mkNote("note", "landed"))
	if conf != nil {
		t.Fatalf("landed put became %s", conf.Code)
	}
	found := false
	for _, w := range res.Warnings {
		if w.Rule == "unlock_failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want unlock_failed warning, got %+v", res.Warnings)
	}
	got, gconf := Get(f.cs, "hosta", "notes/unlock.md")
	if gconf != nil {
		t.Fatal(gconf.Code)
	}
	if !strings.Contains(got.Content, "landed") {
		t.Fatalf("get missed landed bytes: %q", got.Content)
	}
}

func TestUnlockAfterSnapshotDoesNotSkipGet(t *testing.T) {
	f := newFixture(t)
	if _, conf := f.put("hosta", "notes/visible.md", mkNote("note", "visible")); conf != nil {
		t.Fatal(conf.Code)
	}
	releaseHook = func() error { return errors.New("injected unlock") }
	t.Cleanup(func() { releaseHook = nil })
	got, conf := Get(f.cs, "hosta", "notes/visible.md")
	if conf != nil {
		t.Fatalf("get skipped snapshot: %s", conf.Code)
	}
	if !strings.Contains(got.Content, "visible") {
		t.Fatalf("get missed snapshot bytes: %q", got.Content)
	}
}
