// Package config manages typed configuration, safe defaults, and validation rules
// for Estiraha Next. Safety invariants cannot be bypassed by malformed config files.
package config

import (
	"encoding/json"
	"errors"
	"os"
)

const DefaultImageCacheBytes int64 = 2 * 1024 * 1024 * 1024 // 2 GiB initial experimental default

type Config struct {
	App         AppConfig         `json:"app"`
	Database    DatabaseConfig    `json:"database"`
	Safety      SafetyConfig      `json:"safety"`
	ImageEngine ImageEngineConfig `json:"image_engine"`
	APIHub      APIHubConfig      `json:"api_hub"`
	Player      PlayerConfig      `json:"player"`
}

type AppConfig struct {
	Name               string   `json:"name"`
	CodeName           string   `json:"code_name"`
	Version            string   `json:"version"`
	DefaultLanguage    string   `json:"default_language"`
	SupportedLanguages []string `json:"supported_languages"`
	Theme              string   `json:"theme"`
	RunInTray          bool     `json:"run_in_tray"`
}

type DatabaseConfig struct {
	Driver                    string `json:"driver"`
	JournalMode               string `json:"journal_mode"`
	Synchronous               string `json:"synchronous"`
	ForeignKeys               bool   `json:"foreign_keys"`
	BusyTimeoutMS             int    `json:"busy_timeout_ms"`
	CacheSizeKB               int    `json:"cache_size_kb"`
	AutoBackupBeforeMigration bool   `json:"auto_backup_before_migration"`
}

type SafetyConfig struct {
	RequireExplicitReviewApproval bool `json:"require_explicit_review_approval"`
	AllowAutomaticMove            bool `json:"allow_automatic_move"`
	AllowAutomaticRename          bool `json:"allow_automatic_rename"`
	AllowAutomaticDelete          bool `json:"allow_automatic_delete"`
	AllowOverwriteExisting        bool `json:"allow_overwrite_existing"`
	WatcherReadOnlyMode           bool `json:"watcher_read_only_mode"`
	ArchiveAutoExtract            bool `json:"archive_auto_extract"`
	TrashManualEmptyOnly          bool `json:"trash_manual_empty_only"`
}

type ImageEngineConfig struct {
	MaxDiskCacheBytes        int64    `json:"max_disk_cache_bytes"`
	MemoryCacheMB            int      `json:"memory_cache_mb"`
	EvictionPolicy           string   `json:"eviction_policy"`
	AllowSVG                 bool     `json:"allow_svg"`
	AllowedMimeTypes         []string `json:"allowed_mime_types"`
	MaxImageBytes            int64    `json:"max_image_bytes"`
	MaxDimensionPX           int      `json:"max_dimension_px"`
	StoreTempOutsideUserLibs bool     `json:"store_temp_outside_user_library"`
}

type APIHubConfig struct {
	AutoConnectUnknownProviders   bool   `json:"auto_connect_unknown_providers"`
	DeepSeekEnabled               bool   `json:"deepseek_enabled"`
	DeepSeekRequireRequestConsent bool   `json:"deepseek_require_per_request_consent"`
	ElCinemaStatus                string `json:"elcinema_status"`
	OpenSubtitlesMode             string `json:"opensubtitles_mode"`
}

type PlayerConfig struct {
	DefaultEngine            string `json:"default_engine"`
	FallbackExternal         string `json:"fallback_external"`
	ResumeThresholdSeconds   int    `json:"resume_threshold_seconds"`
	WatchedCompletionPercent int    `json:"watched_completion_percent"`
}

// Default returns the non-negotiable safe baseline configuration.
func Default() Config {
	return Config{
		App: AppConfig{
			Name:               "Estiraha Next",
			CodeName:           "estiraha-next",
			Version:            "0.2.0-phase-b",
			DefaultLanguage:    "ar",
			SupportedLanguages: []string{"ar", "en"},
			Theme:              "dark",
			RunInTray:          true,
		},
		Database: DatabaseConfig{
			Driver:                    "sqlite",
			JournalMode:               "WAL",
			Synchronous:               "NORMAL",
			ForeignKeys:               true,
			BusyTimeoutMS:             10000,
			CacheSizeKB:               -64000,
			AutoBackupBeforeMigration: true,
		},
		Safety: SafetyConfig{
			RequireExplicitReviewApproval: true,
			AllowAutomaticMove:            false,
			AllowAutomaticRename:          false,
			AllowAutomaticDelete:          false,
			AllowOverwriteExisting:        false,
			WatcherReadOnlyMode:           true,
			ArchiveAutoExtract:            false,
			TrashManualEmptyOnly:          true,
		},
		ImageEngine: ImageEngineConfig{
			MaxDiskCacheBytes:        DefaultImageCacheBytes,
			MemoryCacheMB:            256,
			EvictionPolicy:           "LRU",
			AllowSVG:                 false,
			AllowedMimeTypes:         []string{"image/jpeg", "image/png", "image/webp"},
			MaxImageBytes:            15 * 1024 * 1024,
			MaxDimensionPX:           8192,
			StoreTempOutsideUserLibs: true,
		},
		APIHub: APIHubConfig{
			AutoConnectUnknownProviders:   false,
			DeepSeekEnabled:               false,
			DeepSeekRequireRequestConsent: true,
			ElCinemaStatus:                "deferred_pending_legal_verification",
			OpenSubtitlesMode:             "search_only",
		},
		Player: PlayerConfig{
			DefaultEngine:            "mpv_candidate",
			FallbackExternal:         "vlc",
			ResumeThresholdSeconds:   30,
			WatchedCompletionPercent: 90,
		},
	}
}

// Load reads JSON configuration from path and validates all mandatory safety invariants.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.ValidateSafetyInvariants(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ValidateSafetyInvariants enforces Estiraha Next non-negotiable protection laws.
func (c Config) ValidateSafetyInvariants() error {
	if !c.Safety.RequireExplicitReviewApproval {
		return errors.New("safety violation: review approval requirement cannot be disabled")
	}
	if c.Safety.AllowAutomaticMove || c.Safety.AllowAutomaticRename || c.Safety.AllowAutomaticDelete {
		return errors.New("safety violation: automatic move, rename, or delete is strictly prohibited")
	}
	if c.Safety.AllowOverwriteExisting {
		return errors.New("safety violation: overwriting existing files is strictly prohibited")
	}
	if !c.Safety.WatcherReadOnlyMode {
		return errors.New("safety violation: disk watcher must operate in read-only alert mode")
	}
	if c.Safety.ArchiveAutoExtract {
		return errors.New("safety violation: automatic extraction of ZIP/RAR archives is prohibited")
	}
	if !c.Safety.TrashManualEmptyOnly {
		return errors.New("safety violation: trash must only be emptied manually")
	}
	if c.ImageEngine.AllowSVG {
		return errors.New("security violation: unsafe SVG loading is prohibited in Smart Image Engine")
	}
	if !c.ImageEngine.StoreTempOutsideUserLibs {
		return errors.New("safety violation: temporary/cached images must never be placed inside user library folders")
	}
	if c.APIHub.AutoConnectUnknownProviders {
		return errors.New("security violation: unknown API providers must never be auto-connected")
	}
	if c.APIHub.DeepSeekEnabled && !c.APIHub.DeepSeekRequireRequestConsent {
		return errors.New("privacy violation: DeepSeek requires explicit per-request consent")
	}
	return nil
}
