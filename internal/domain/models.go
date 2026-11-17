// Package domain defines the immutable core entities, value objects, and enumerations
// shared across Estiraha Next engines without coupling to infrastructure or UI layers.
package domain

import "github.com/google/uuid"

// NewImmutableID generates a stable UUID string that NEVER changes when a media work
// or physical file is renamed or moved across folders or disks.
func NewImmutableID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.NewString()
	}
	return id.String()
}

// MediaType represents all 12 officially supported media categories in Estiraha Next.
type MediaType string

const (
	MediaTypeMovie                MediaType = "movie"
	MediaTypeSeries               MediaType = "series"
	MediaTypeTVProgram            MediaType = "tv_program"
	MediaTypeTalkShow             MediaType = "talk_show"
	MediaTypeGameShow             MediaType = "game_show"
	MediaTypeSportsProgram        MediaType = "sports_program"
	MediaTypeEntertainmentProgram MediaType = "entertainment_program"
	MediaTypeDocumentaryMovie     MediaType = "documentary_movie"
	MediaTypeDocumentarySeries    MediaType = "documentary_series"
	MediaTypeAnime                MediaType = "anime"
	MediaTypeCartoon              MediaType = "cartoon"
	MediaTypeFranchiseEntry       MediaType = "franchise_entry"
)

// AllMediaTypes returns the complete canonical list of supported media types.
func AllMediaTypes() []MediaType {
	return []MediaType{
		MediaTypeMovie,
		MediaTypeSeries,
		MediaTypeTVProgram,
		MediaTypeTalkShow,
		MediaTypeGameShow,
		MediaTypeSportsProgram,
		MediaTypeEntertainmentProgram,
		MediaTypeDocumentaryMovie,
		MediaTypeDocumentarySeries,
		MediaTypeAnime,
		MediaTypeCartoon,
		MediaTypeFranchiseEntry,
	}
}
