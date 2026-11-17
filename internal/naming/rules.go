// Package naming enforces Estiraha Next's mandatory, non-negotiable naming laws
// across Movies, Series, Programs, Cinematic Franchises, Celebrities, and Documentaries.
package naming

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	errEmptyTitle          = errors.New("naming: title cannot be empty")
	errInvalidYear         = errors.New("naming: movie/standalone documentary production year must be between 1888 and 2100")
	forbiddenQualityTokens = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|4k|8k|uhd|bluray|blu-ray|web-dl|webdl|webrip|hdrip|brrip|dvdrip|remux|x264|x265|h264|h265|hevc|av1|aac|dts|atmos|hdr|hdr10|dv|proper|repack|yify|yts|rarbg)\b`)
	forbiddenBrackets      = regexp.MustCompile(`[\(\)\[\]\{\}]`)
	multiSpaceRegex        = regexp.MustCompile(`\s+`)
	windowsIllegalChars    = regexp.MustCompile(`[<>:"/\\|?*]`)
)

// CleanTitle strips dot-separators, quality/codec/release tags, brackets, and Windows-illegal
// path characters WITHOUT speculatively translating the title language.
func CleanTitle(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errEmptyTitle
	}
	// Replace dots and underscores used as word separators
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = forbiddenBrackets.ReplaceAllString(s, " ")
	s = forbiddenQualityTokens.ReplaceAllString(s, " ")
	s = windowsIllegalChars.ReplaceAllString(s, " ")
	s = strings.Trim(s, " -–—~+*#@!")
	s = multiSpaceRegex.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errEmptyTitle
	}
	return s, nil
}

// FormatMovieName returns the canonical folder and file stem for Movies and Standalone Documentaries:
// "Movie Name YYYY" (NO parentheses around year, NO quality/codec/group tags, NO dot separators).
func FormatMovieName(cleanOrRawTitle string, productionYear int) (string, error) {
	if productionYear < 1888 || productionYear > 2100 {
		return "", errInvalidYear
	}
	title, err := CleanTitle(cleanOrRawTitle)
	if err != nil {
		return "", err
	}
	// Remove trailing year if already embedded in raw title to prevent "Movie 2008 2008"
	yearSuffix := fmt.Sprintf(" %d", productionYear)
	if strings.HasSuffix(title, yearSuffix) {
		title = strings.TrimSpace(strings.TrimSuffix(title, yearSuffix))
	}
	return fmt.Sprintf("%s %d", title, productionYear), nil
}

// FormatSeriesFolder returns the canonical main folder name for Series and Episodic Documentaries:
// Series Name ONLY — NEVER includes production year.
func FormatSeriesFolder(cleanOrRawTitle string) (string, error) {
	title, err := CleanTitle(cleanOrRawTitle)
	if err != nil {
		return "", err
	}
	return title, nil
}

// FormatSeasonFolder returns canonical season subfolder name (e.g., "S01", "S02").
func FormatSeasonFolder(seasonNumber int) (string, error) {
	if seasonNumber < 0 {
		return "", errors.New("naming: season number cannot be negative")
	}
	return fmt.Sprintf("S%02d", seasonNumber), nil
}

// FormatEpisodeFile returns canonical episode filename inside a season folder (e.g., "01.mp4").
func FormatEpisodeFile(episodeNumber int, extWithDot string) (string, error) {
	if episodeNumber <= 0 {
		return "", errors.New("naming: episode number must be >= 1")
	}
	ext := strings.ToLower(strings.TrimSpace(extWithDot))
	if !strings.HasPrefix(ext, ".") || len(ext) < 2 {
		return "", errors.New("naming: valid file extension with leading dot required")
	}
	return fmt.Sprintf("%02d%s", episodeNumber, ext), nil
}

// FormatFranchiseFolder returns the canonical Cinematic Franchise parent folder name (without year).
func FormatFranchiseFolder(franchiseName string) (string, error) {
	return CleanTitle(franchiseName)
}

// FormatCelebrityFolder enforces the official celebrity folder/display naming rule:
// "الاسم بالعربية (English Name)"
func FormatCelebrityFolder(arabicName, englishName string) (string, error) {
	ar := multiSpaceRegex.ReplaceAllString(strings.TrimSpace(windowsIllegalChars.ReplaceAllString(arabicName, " ")), " ")
	en := multiSpaceRegex.ReplaceAllString(strings.TrimSpace(windowsIllegalChars.ReplaceAllString(englishName, " ")), " ")
	if ar == "" && en == "" {
		return "", errors.New("naming: celebrity requires at least Arabic or English name")
	}
	if ar != "" && en != "" {
		return fmt.Sprintf("%s (%s)", ar, en), nil
	}
	if ar != "" {
		return ar, nil
	}
	return en, nil
}

// ProgramHandlingPlan captures mandatory routing for Known vs Unknown TV Programs.
type ProgramHandlingPlan struct {
	FolderName         string
	AllowAutoSeasons   bool
	RouteToReviewQueue bool
}

// PlanProgramNaming applies official program rules:
// - Known program: clean canonical folder, seasons/episodes enabled.
// - Unknown program: preserve original folder name (sanitized only for OS safety), forbid auto-season creation, send to Review Center.
func PlanProgramNaming(originalName string, isKnownProgram bool) (ProgramHandlingPlan, error) {
	trimmed := strings.TrimSpace(windowsIllegalChars.ReplaceAllString(originalName, " "))
	if trimmed == "" {
		return ProgramHandlingPlan{}, errEmptyTitle
	}
	if !isKnownProgram {
		return ProgramHandlingPlan{
			FolderName:         multiSpaceRegex.ReplaceAllString(trimmed, " "),
			AllowAutoSeasons:   false,
			RouteToReviewQueue: true,
		}, nil
	}
	clean, err := CleanTitle(trimmed)
	if err != nil {
		return ProgramHandlingPlan{}, err
	}
	return ProgramHandlingPlan{
		FolderName:         clean,
		AllowAutoSeasons:   true,
		RouteToReviewQueue: false,
	}, nil
}
