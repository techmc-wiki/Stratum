package consistency

import "testing"

func TestLevelValidateRejectsUnknownLevel(t *testing.T) {
	if err := Level("unknown").Validate(); err == nil {
		t.Fatal("expected unknown level to fail")
	}
}

func TestParse(t *testing.T) {
	level, err := Parse("plugin_backup")
	if err != nil {
		t.Fatal(err)
	}
	if level != LevelPluginBackup {
		t.Fatalf("level = %q, want %q", level, LevelPluginBackup)
	}
}
