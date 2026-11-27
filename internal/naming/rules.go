// Package naming enforces Estiraha Next's mandatory, non-negotiable naming laws
// across Movies, Series, Programs, Cinematic Franchises, Celebrities, and Documentaries.
package naming

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	errEmptyTitle  = errors.New("naming: title cannot be empty")
	errInvalidYear = errors.New("naming: movie/standalone documentary production year must be between 1888 and 2100")

	// Strips leading tracker/website prefixes like `[ www.UsaBit.com ] - ` or `(www.YTS.MX) - `
	leadingWebsiteBracketRe = regexp.MustCompile(`(?i)^\s*[\[\(\{]\s*(?:www\.)?[a-z0-9_-]+\.(?:com|net|org|to|mx|lt|ag|am|ph|io|me|info|cc|tv|ws|bz|re|sx|la|vc|ch|li|pw)\s*[\]\)\}]\s*[-–—._]*\s*`)
	// Strips inline tracker domain tokens while intentionally excluding `.to` so movie titles like `3000.Miles.to.Graceland` are NEVER corrupted!
	inlineWebsiteDomainRe = regexp.MustCompile(`(?i)\b(?:www\.[a-z0-9_-]+\.[a-z]{2,6}|[a-z0-9_-]+\.(?:com|net|org|mx|lt|ag|am|ph|torrent|torrents))\b`)

	forbiddenQualityTokens = regexp.MustCompile(`(?i)\b(2160p|1080p|720p|480p|360p|4k|8k|uhd|fhd|hd|sd|bluray|blu-ray|bdrip|brrip|web-dl|webdl|webrip|hdrip|dvdrip|dvdscr|camrip|hdcam|hdtv|remux|x264|x265|h264|h265|hevc|avc|av1|xvid|divx|aac|ac3|eac3|dts|truehd|atmos|mp3|flac|5\.1|7\.1|10bit|8bit|hdr|hdr10|hdr10plus|dv|dolbyvision|proper|repack|rerip|extended|unrated|directors\.cut|theatrical|yify|yts|rarbg|tigole|qxr|psa|evo|etrg|fgt|sparks|amiable|ganool|shaanig|pahe|usabit|arabseed|akwam|cima4u|mycima|egybest)\b`)
	forbiddenBrackets      = regexp.MustCompile(`[\(\)\[\]\{\}]`)
	multiSpaceRegex        = regexp.MustCompile(`\s+`)
	windowsIllegalChars    = regexp.MustCompile(`[<>:"/\\|?*]`)
	nonAlphaNumIdentityRe  = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	subtitleTokenSplitRe   = regexp.MustCompile(`[\s._\-\(\)\[\]\{\}]+`)
)

var englishNumberWords = map[string]string{
	"zero": "0", "one": "1", "two": "2", "three": "3", "four": "4",
	"five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9",
	"ten": "10", "eleven": "11", "twelve": "12", "thirteen": "13",
	"fourteen": "14", "fifteen": "15", "sixteen": "16", "seventeen": "17",
	"eighteen": "18", "nineteen": "19", "twenty": "20", "thirty": "30",
	"forty": "40", "fifty": "50", "sixty": "60", "seventy": "70",
	"eighty": "80", "ninety": "90", "hundred": "100", "thousand": "1000",
}

var subtitleLangMap = map[string]string{
	"ar": "ar", "ara": "ar", "arab": "ar", "arabic": "ar", "عربي": "ar", "العربية": "ar",
	"en": "en", "eng": "en", "english": "en",
	"fr": "fr", "fre": "fr", "fra": "fr", "french": "fr", "francais": "fr", "français": "fr",
	"es": "es", "spa": "es", "spanish": "es", "espanol": "es", "español": "es",
	"de": "de", "ger": "de", "deu": "de", "german": "de", "deutsch": "de",
	"it": "it", "ita": "it", "italian": "it", "italiano": "it",
	"pt": "pt", "por": "pt", "portuguese": "pt", "pob": "pt", "ptbr": "pt",
	"ru": "ru", "rus": "ru", "russian": "ru",
	"tr": "tr", "tur": "tr", "turkish": "tr",
	"ja": "ja", "jpn": "ja", "japanese": "ja",
	"ko": "ko", "kor": "ko", "korean": "ko",
	"zh": "zh", "chi": "zh", "zho": "zh", "chs": "zh", "cht": "zh", "chinese": "zh",
	"hi": "hi", "hin": "hi", "hindi": "hi",
	"ur": "ur", "urd": "ur", "urdu": "ur",
	"fa": "fa", "per": "fa", "fas": "fa", "persian": "fa", "farsi": "fa",
	"nl": "nl", "dut": "nl", "nld": "nl", "dutch": "nl",
	"sv": "sv", "swe": "sv", "swedish": "sv",
	"no": "no", "nor": "no", "norwegian": "no",
	"da": "da", "dan": "da", "danish": "da",
	"fi": "fi", "fin": "fi", "finnish": "fi",
	"pl": "pl", "pol": "pl", "polish": "pl",
	"cs": "cs", "cze": "cs", "ces": "cs", "czech": "cs",
	"el": "el", "gre": "el", "ell": "el", "greek": "el",
	"he": "he", "heb": "he", "hebrew": "he",
	"ro": "ro", "rum": "ro", "ron": "ro", "romanian": "ro",
	"hu": "hu", "hun": "hu", "hungarian": "hu",
	"id": "id", "ind": "id", "indonesian": "id",
	"ms": "ms", "may": "ms", "msa": "ms", "malay": "ms",
	"vi": "vi", "vie": "vi", "vietnamese": "vi",
	"th": "th", "tha": "th", "thai": "th",
	"et": "et", "est": "et", "estonian": "et",
	"uk": "uk", "ukr": "uk", "ukrainian": "uk",
	"bg": "bg", "bul": "bg", "bulgarian": "bg",
	"hr": "hr", "hrv": "hr", "croatian": "hr",
	"sr": "sr", "srp": "sr", "serbian": "sr",
}

// CleanTitle strips dot-separators, website tags, quality/codec/release tags, brackets, and Windows-illegal
// path characters WITHOUT speculatively translating the title language, while capitalizing lowercase Latin titles cleanly.
func CleanTitle(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errEmptyTitle
	}
	for leadingWebsiteBracketRe.MatchString(s) {
		s = leadingWebsiteBracketRe.ReplaceAllString(s, "")
	}
	s = inlineWebsiteDomainRe.ReplaceAllString(s, " ")
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
	s = normalizeLatinTitleCaseIfAllLower(s)
	return s, nil
}

func normalizeLatinTitleCaseIfAllLower(s string) string {
	hasLatinLower := false
	hasLatinUpper := false
	hasArabic := false
	for _, r := range s {
		if unicode.In(r, unicode.Arabic) {
			hasArabic = true
		}
		if r >= 'a' && r <= 'z' {
			hasLatinLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasLatinUpper = true
		}
	}
	if hasArabic || !hasLatinLower || hasLatinUpper {
		return s
	}
	smallWords := map[string]bool{
		"a": true, "an": true, "and": true, "as": true, "at": true, "but": true,
		"by": true, "for": true, "from": true, "in": true, "into": true, "nor": true,
		"of": true, "on": true, "or": true, "over": true, "the": true, "to": true, "with": true,
	}
	words := strings.Fields(s)
	for i, w := range words {
		if i > 0 && i < len(words)-1 && smallWords[w] {
			continue
		}
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

// NormalizeWorkIdentityKey builds a deterministic title+year comparison key so alternate encodings
// like `gone in 60 seconds 2000` and `Gone in Sixty Seconds (2000) 1080p` unite under ONE verified movie work.
func NormalizeWorkIdentityKey(cleanOrRawTitle string, year int) string {
	cleaned, err := CleanTitle(cleanOrRawTitle)
	if err != nil {
		cleaned = strings.TrimSpace(cleanOrRawTitle)
	}
	lower := strings.ToLower(cleaned)
	if year >= 1888 && year <= 2100 {
		yrStr := fmt.Sprintf("%d", year)
		if strings.HasSuffix(lower, " "+yrStr) {
			lower = strings.TrimSpace(strings.TrimSuffix(lower, " "+yrStr))
		}
	}
	lower = nonAlphaNumIdentityRe.ReplaceAllString(lower, " ")
	tokens := strings.Fields(lower)
	normTokens := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if numStr, ok := englishNumberWords[tok]; ok {
			normTokens = append(normTokens, numStr)
			continue
		}
		normTokens = append(normTokens, tok)
	}
	base := strings.Join(normTokens, " ")
	if year >= 1888 && year <= 2100 {
		return fmt.Sprintf("%s|%d", base, year)
	}
	return base
}

func FormatMovieName(cleanOrRawTitle string, productionYear int) (string, error) {
	if productionYear < 1888 || productionYear > 2100 {
		return "", errInvalidYear
	}
	title, err := CleanTitle(cleanOrRawTitle)
	if err != nil {
		return "", err
	}
	yearSuffix := fmt.Sprintf(" %d", productionYear)
	if strings.HasSuffix(title, yearSuffix) {
		title = strings.TrimSpace(strings.TrimSuffix(title, yearSuffix))
	}
	return fmt.Sprintf("%s %d", title, productionYear), nil
}

func FormatSeriesFolder(cleanOrRawTitle string) (string, error) {
	title, err := CleanTitle(cleanOrRawTitle)
	if err != nil {
		return "", err
	}
	return title, nil
}

func FormatSeasonFolder(seasonNumber int) (string, error) {
	if seasonNumber < 0 {
		return "", errors.New("naming: season number cannot be negative")
	}
	return fmt.Sprintf("S%02d", seasonNumber), nil
}

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

func FormatFranchiseFolder(franchiseName string) (string, error) {
	return CleanTitle(franchiseName)
}

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

type ProgramHandlingPlan struct {
	FolderName         string
	AllowAutoSeasons   bool
	RouteToReviewQueue bool
}

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

type CompanionFilePlan struct {
	OriginalFilename string `json:"originalFilename"`
	OriginalRelPath  string `json:"originalRelPath,omitempty"`
	Extension        string `json:"extension"`
	FileKind         string `json:"fileKind"`
	LanguageHint     string `json:"languageHint,omitempty"`
	IsPrimaryPoster  bool   `json:"isPrimaryPoster,omitempty"`
	VersionTag       string `json:"versionTag,omitempty"`
	ProposedFilename string `json:"proposedFilename"`
	ProtectedShorts  bool   `json:"protectedShorts"`
}

func DetectSubtitleLanguageCode(filename string) string {
	base := strings.TrimSpace(filepath.Base(filename))
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	trimmedBase := strings.TrimSpace(base)
	if trimmedBase == "" {
		return ""
	}
	if code, ok := subtitleLangMap[strings.ToLower(trimmedBase)]; ok {
		return code
	}
	tokens := subtitleTokenSplitRe.Split(strings.ToLower(trimmedBase), -1)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t == "" || t == "sdh" || t == "hi" || t == "cc" || t == "forced" || t == "utf8" || t == "utf" {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) > 0 {
		last := filtered[len(filtered)-1]
		if code, ok := subtitleLangMap[last]; ok {
			return code
		}
		first := filtered[0]
		if len(first) >= 3 {
			if code, ok := subtitleLangMap[first]; ok {
				return code
			}
		}
	}
	return ""
}

func ExtractLanguageSuffix(filename string) string {
	code := DetectSubtitleLanguageCode(filename)
	if code == "" {
		return ""
	}
	return "." + code
}

func IsShortsExtension(filenameOrExt string) bool {
	s := strings.ToLower(strings.TrimSpace(filenameOrExt))
	return s == ".shorts" || strings.HasSuffix(s, ".shorts")
}

func PlanMovieBundle(title string, year int, companions []CompanionFilePlan, multiSubtitle bool) (string, []CompanionFilePlan, error) {
	stem, err := FormatMovieName(title, year)
	if err != nil {
		return "", nil, err
	}

	usedNames := map[string]bool{}
	reserveUnique := func(desiredStem, extWithDot string, dotStyle bool) string {
		cand := desiredStem + extWithDot
		if !usedNames[strings.ToLower(cand)] {
			usedNames[strings.ToLower(cand)] = true
			return cand
		}
		for n := 2; n < 10000; n++ {
			var next string
			if dotStyle {
				next = fmt.Sprintf("%s.%d%s", desiredStem, n, extWithDot)
			} else {
				next = fmt.Sprintf("%s (%d)%s", desiredStem, n, extWithDot)
			}
			if !usedNames[strings.ToLower(next)] {
				usedNames[strings.ToLower(next)] = true
				return next
			}
		}
		return cand
	}

	hasExplicitPrimaryPoster := false
	for _, c := range companions {
		if c.FileKind == "image" && c.IsPrimaryPoster {
			hasExplicitPrimaryPoster = true
		}
	}

	vobSubAssignedStem := map[string]string{}
	videoIdx := 0
	extraArtIdx := 1
	assignedPrimaryPoster := false

	out := make([]CompanionFilePlan, 0, len(companions))
	for _, c := range companions {
		ext := strings.ToLower(strings.TrimSpace(c.Extension))
		if IsShortsExtension(c.OriginalFilename) || IsShortsExtension(ext) {
			proposed := reserveUnique(stem, ".shorts", true)
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ".shorts",
				FileKind:         "shorts",
				ProposedFilename: proposed,
				ProtectedShorts:  true,
			})
			continue
		}
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(c.OriginalFilename))
		}

		switch c.FileKind {
		case "video":
			videoIdx++
			desiredStem := stem
			if videoIdx > 1 {
				tag := strings.TrimSpace(c.VersionTag)
				if tag == "" {
					tag = fmt.Sprintf("v%d", videoIdx)
				}
				desiredStem = fmt.Sprintf("%s [%s]", stem, tag)
			}
			proposed := reserveUnique(desiredStem, ext, false)
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ext,
				FileKind:         c.FileKind,
				VersionTag:       c.VersionTag,
				ProposedFilename: proposed,
			})

		case "subtitle":
			lang := strings.TrimSpace(c.LanguageHint)
			if lang == "" {
				lang = DetectSubtitleLanguageCode(c.OriginalFilename)
			}
			pairKey := ""
			if ext == ".sub" || ext == ".idx" {
				srcBase := strings.TrimSuffix(c.OriginalFilename, filepath.Ext(c.OriginalFilename))
				srcDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(c.OriginalRelPath)))
				pairKey = strings.ToLower(srcDir + "|" + srcBase)
			}
			if pairKey != "" {
				if pairedStem, ok := vobSubAssignedStem[pairKey]; ok {
					proposed := reserveUnique(pairedStem, ext, true)
					out = append(out, CompanionFilePlan{
						OriginalFilename: c.OriginalFilename,
						OriginalRelPath:  c.OriginalRelPath,
						Extension:        ext,
						FileKind:         c.FileKind,
						LanguageHint:     lang,
						ProposedFilename: proposed,
					})
					continue
				}
			}

			desiredStem := stem
			if lang != "" {
				desiredStem = stem + "." + lang
			} else if multiSubtitle {
				desiredStem = stem + ".und"
			}
			proposed := reserveUnique(desiredStem, ext, true)
			if pairKey != "" {
				vobSubAssignedStem[pairKey] = strings.TrimSuffix(proposed, ext)
			}
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ext,
				FileKind:         c.FileKind,
				LanguageHint:     lang,
				ProposedFilename: proposed,
			})

		case "image":
			isPrimary := false
			if hasExplicitPrimaryPoster && c.IsPrimaryPoster && !assignedPrimaryPoster {
				isPrimary = true
				assignedPrimaryPoster = true
			}
			var proposed string
			if isPrimary {
				proposed = reserveUnique("poster", ext, false)
			} else if hasExplicitPrimaryPoster {
				extraArtIdx++
				proposed = reserveUnique(fmt.Sprintf("%s - artwork-%d", stem, extraArtIdx), ext, false)
			} else {
				proposed = reserveUnique(stem, ext, false)
			}
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ext,
				FileKind:         c.FileKind,
				IsPrimaryPoster:  isPrimary,
				ProposedFilename: proposed,
			})

		default:
			proposed := reserveUnique(stem, ext, false)
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ext,
				FileKind:         c.FileKind,
				ProposedFilename: proposed,
			})
		}
	}
	return stem, out, nil
}

func PlanEpisodeBundle(seriesTitle string, seasonNum, episodeNum int, companions []CompanionFilePlan, multiSubtitle bool) (string, string, []CompanionFilePlan, error) {
	seriesFolder, err := FormatSeriesFolder(seriesTitle)
	if err != nil {
		return "", "", nil, err
	}
	seasonFolder, err := FormatSeasonFolder(seasonNum)
	if err != nil {
		return "", "", nil, err
	}
	used := map[string]bool{}
	out := make([]CompanionFilePlan, 0, len(companions))
	for _, c := range companions {
		ext := strings.ToLower(strings.TrimSpace(c.Extension))
		if IsShortsExtension(c.OriginalFilename) || IsShortsExtension(ext) {
			epShorts, err := FormatEpisodeFile(episodeNum, ".shorts")
			if err != nil {
				return "", "", nil, err
			}
			cand := epShorts
			for n := 2; used[strings.ToLower(cand)]; n++ {
				cand = fmt.Sprintf("%02d.%d.shorts", episodeNum, n)
			}
			used[strings.ToLower(cand)] = true
			out = append(out, CompanionFilePlan{
				OriginalFilename: c.OriginalFilename,
				OriginalRelPath:  c.OriginalRelPath,
				Extension:        ".shorts",
				FileKind:         "shorts",
				ProposedFilename: cand,
				ProtectedShorts:  true,
			})
			continue
		}
		if ext == "" {
			ext = strings.ToLower(filepath.Ext(c.OriginalFilename))
		}
		epFile, err := FormatEpisodeFile(episodeNum, ext)
		if err != nil {
			return "", "", nil, err
		}
		if c.FileKind == "subtitle" {
			lang := strings.TrimSpace(c.LanguageHint)
			if lang == "" {
				lang = DetectSubtitleLanguageCode(c.OriginalFilename)
			}
			if lang != "" && multiSubtitle {
				epFile = fmt.Sprintf("%02d.%s%s", episodeNum, lang, ext)
			}
		}
		baseNoExt := strings.TrimSuffix(epFile, ext)
		cand := epFile
		for n := 2; used[strings.ToLower(cand)]; n++ {
			cand = fmt.Sprintf("%s.%d%s", baseNoExt, n, ext)
		}
		used[strings.ToLower(cand)] = true
		out = append(out, CompanionFilePlan{
			OriginalFilename: c.OriginalFilename,
			OriginalRelPath:  c.OriginalRelPath,
			Extension:        ext,
			FileKind:         c.FileKind,
			ProposedFilename: cand,
			ProtectedShorts:  false,
		})
	}
	return seriesFolder, seasonFolder, out, nil
}
