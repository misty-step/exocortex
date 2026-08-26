package kernel

import "testing"

func TestJournalPrefix(t *testing.T) {
	if JournalPrefix(nil) != "" {
		t.Fatal("nil cortex must not invent a prefix")
	}
	if got := JournalPrefix(&Cortex{}); got != "journal" {
		t.Fatalf("empty field default = %q", got)
	}
	if got := JournalPrefix(&Cortex{JournalPrefix: "daily"}); got != "daily" {
		t.Fatalf("custom = %q", got)
	}
}

func TestCortexNamed(t *testing.T) {
	cs := []Cortex{{Name: "vault"}, {Name: "emma"}}
	if CortexNamed(cs, "emma").Name != "emma" {
		t.Fatal("lookup missed emma")
	}
	if CortexNamed(cs, "missing") != nil {
		t.Fatal("missing name must be nil")
	}
}
