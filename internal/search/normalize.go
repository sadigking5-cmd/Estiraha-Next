// Package search executes dual search (standard + advanced multi-facet + natural-language query parser)
// over local SQLite FTS5 and optional external sources, with first-class Arabic orthographic normalization.
package search

import "strings"

var arabicNormalizer = strings.NewReplacer(
	"أ", "ا",
	"إ", "ا",
	"آ", "ا",
	"ٱ", "ا",
	"ة", "ه",
	"ى", "ي",
	"ؤ", "و",
	"ئ", "ي",
	"ـ", "", // Tatweel / Kashida
)

// NormalizeQuery normalizes Arabic alef variants, teh marbuta, alef maksura, and tatweel
// so local FTS5 queries match seamlessly whether the user types 'احمد زكي' or 'أحمد زكي'.
func NormalizeQuery(q string) string {
	return strings.TrimSpace(arabicNormalizer.Replace(q))
}
