package translation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitTranslationsFillsMissingFromBundled(t *testing.T) {
	dir := t.TempDir()
	// На диске только старый ключ — как том, отставший от релиза.
	if err := os.WriteFile(filepath.Join(dir, "ru.json"), []byte(`{"greeting":"Привет с диска"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{"greeting":"Hello from disk"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	tm := &Manager{translations: make(map[string]Translation), defaultLanguage: "ru"}
	if err := tm.InitTranslations(dir, "ru"); err != nil {
		t.Fatal(err)
	}

	if got := tm.GetText("ru", "greeting"); got != "Привет с диска" {
		t.Fatalf("disk override: got %q", got)
	}
	// Ключ из вшитого ru.json должен остаться.
	if got := tm.GetText("ru", "subscription_activated"); got == "" || got == "subscription_activated" {
		t.Fatalf("expected bundled key, got %q", got)
	}
}

func TestDeepMergeJSONKeepsBundledArrayItems(t *testing.T) {
	base := []byte(`{"platforms":{"ios":[{"id":"rabbithole","name":"RabbitHole"},{"id":"happ","name":"Happ"}]}}`)
	overlay := []byte(`{"platforms":{"ios":[{"id":"happ","name":"Happ Custom"}]}}`)
	out, err := DeepMergeJSON(base, overlay)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !containsAll(s, "rabbithole", "Happ Custom") {
		t.Fatalf("merge lost bundled app or override: %s", s)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
