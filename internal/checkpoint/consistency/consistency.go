package consistency

import "fmt"

type Level string

const (
	LevelMetadataOnly     Level = "metadata_only"
	LevelStopped          Level = "stopped"
	LevelBestEffort       Level = "best_effort"
	LevelCommandQuiesced  Level = "command_quiesced"
	LevelPluginBackup     Level = "plugin_backup"
	LevelMCBridgePrepared Level = "mc_bridge_prepared"
)

func (l Level) Validate() error {
	switch l {
	case LevelMetadataOnly, LevelStopped, LevelBestEffort, LevelCommandQuiesced, LevelPluginBackup, LevelMCBridgePrepared:
		return nil
	default:
		return fmt.Errorf("unsupported checkpoint consistency level %q", l)
	}
}

func Parse(value string) (Level, error) {
	level := Level(value)
	if err := level.Validate(); err != nil {
		return "", err
	}
	return level, nil
}
