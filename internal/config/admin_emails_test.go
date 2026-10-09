package config

import "testing"

func TestParseCabinetAdminEmails(t *testing.T) {
	got := ParseCabinetAdminEmails(" MyXaXa2403@gmail.com, other@example.com ")
	if len(got) != 2 || got[0] != "myxaxa2403@gmail.com" || got[1] != "other@example.com" {
		t.Fatalf("%v", got)
	}
	if again := ParseCabinetAdminEmails("myxaxa2403@gmail.com,myxaxa2403@gmail.com"); len(again) != 1 {
		t.Fatalf("dup %v", again)
	}
}
