package consistency

import "testing"

func TestLevelValidateAcceptsKnownLevels(t *testing.T) {
	tests := []struct {
		name  string
		level Level
	}{
		{name: "metadata only", level: LevelMetadataOnly},
		{name: "stopped", level: LevelStopped},
		{name: "best effort", level: LevelBestEffort},
		{name: "command quiesced", level: LevelCommandQuiesced},
		{name: "plugin backup", level: LevelPluginBackup},
		{name: "mc bridge prepared", level: LevelMCBridgePrepared},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.level.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

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
