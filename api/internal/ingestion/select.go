package ingestion

import (
	"log/slog"
	"net/url"
	"sort"
	"strings"
)

// subSelection preserves the exact key returned by yt-dlp, including en-orig.

// selectSubtitleTrack uses explicit tiers so translated automatic tracks do
// not win over YouTube's original-language track. No unrelated language is
// ever selected for an explicit request.
func selectSubtitleTrack(meta *Metadata, requested string) (subSelection, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	slog.Debug("subtitle track discovery", "requested_language", requested,
		"manual_languages", sortedKeys(meta.Subtitles),
		"automatic_languages", sortedKeys(meta.AutomaticCaptions))
	if !meta.hasAnyTrack() {
		return subSelection{}, NewError(ErrCodeNoTranscript, "video has no available subtitles")
	}

	if requested != "" {
		if selection, ok := selectRequested(meta, requested); ok {
			slog.Debug("subtitle track selected", "requested_language", requested,
				"selected_language", selection.language, "source", selectionSource(selection))
			return selection, nil
		}
		return subSelection{}, Errorf(ErrCodeLanguageUnavailable, "language %q is not available on this video", requested)
	}
	for _, preferred := range []string{"en", "en-us", "en-gb"} {
		if selection, ok := selectRequested(meta, preferred); ok {
			slog.Debug("subtitle track selected", "requested_language", "",
				"selected_language", selection.language, "source", selectionSource(selection))
			return selection, nil
		}
	}
	for _, key := range sortedKeys(meta.Subtitles) {
		if selection, ok := manualTrack(meta, key); ok {
			return selection, nil
		}
	}
	for _, key := range sortedKeys(meta.AutomaticCaptions) {
		if !automaticTrackIsOriginal(meta, key) {
			continue
		}
		if selection, ok := autoTrack(meta, key); ok {
			return selection, nil
		}
	}
	return subSelection{}, NewError(ErrCodeNoTranscript, "video has no available subtitles")
}

func selectRequested(meta *Metadata, requested string) (subSelection, bool) {
	base := languageBase(requested)
	regional := requested != base

	if selection, ok := manualTrack(meta, requested); ok {
		return selection, true
	}
	// An exact regional automatic track is more specific than generic en-orig.
	if regional {
		if selection, ok := autoTrack(meta, requested); ok {
			return selection, true
		}
	}
	// Manual family matches always precede automatic tracks.
	for _, key := range matchingKeys(meta.Subtitles, base, requested) {
		if selection, ok := manualTrack(meta, key); ok {
			return selection, true
		}
	}
	// For a generic request, original automatic captions precede translated
	// automatic captions. For regional requests they follow the exact match.
	for _, key := range originalAutomaticKeys(meta, base) {
		if selection, ok := autoTrack(meta, key); ok {
			return selection, true
		}
	}
	if automaticTrackIsOriginal(meta, base) {
		if selection, ok := autoTrack(meta, base); ok {
			return selection, true
		}
	}
	for _, key := range matchingKeys(meta.AutomaticCaptions, base, requested) {
		if !automaticTrackIsOriginal(meta, key) {
			continue
		}
		if selection, ok := autoTrack(meta, key); ok {
			return selection, true
		}
	}
	return subSelection{}, false
}

func selectExactSubtitleTrack(meta *Metadata, requested, source string) (subSelection, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return subSelection{}, Errorf(ErrCodeLanguageUnavailable, "subtitle language is required")
	}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "manual":
		if selection, ok := manualTrack(meta, requested); ok {
			return selection, nil
		}
		if strings.EqualFold(source, "manual") {
			return subSelection{}, Errorf(ErrCodeLanguageUnavailable, "manual subtitle %q is not available", requested)
		}
		if selection, ok := autoTrack(meta, requested); ok {
			return selection, nil
		}
	case "automatic":
		if selection, ok := autoTrack(meta, requested); ok {
			return selection, nil
		}
	default:
		return subSelection{}, Errorf(ErrCodeLanguageUnavailable, "unsupported subtitle source %q", source)
	}
	return subSelection{}, Errorf(ErrCodeLanguageUnavailable, "subtitle %q is not available", requested)
}

func subtitleOptions(meta *Metadata) []SubtitleOption {
	options := make([]SubtitleOption, 0)
	for _, key := range sortedKeys(meta.Subtitles) {
		if option, ok := subtitleOption(key, "manual", meta.Subtitles[key]); ok {
			options = append(options, option)
		}
	}
	for _, key := range sortedKeys(meta.AutomaticCaptions) {
		if option, ok := subtitleOption(key, "automatic", meta.AutomaticCaptions[key]); ok {
			options = append(options, option)
		}
	}
	return options
}

func manualSubtitleOptions(meta *Metadata) []SubtitleOption {
	options := make([]SubtitleOption, 0)
	for _, key := range sortedKeys(meta.Subtitles) {
		if option, ok := subtitleOption(key, "manual", meta.Subtitles[key]); ok {
			options = append(options, option)
		}
	}
	return options
}

func subtitleOption(language, source string, entries []subtitleEntry) (SubtitleOption, bool) {
	formats := make([]string, 0, len(entries))
	seen := make(map[string]bool)
	name := ""
	compatible := false
	for _, entry := range entries {
		ext := strings.ToLower(strings.TrimSpace(entry.Ext))
		if ext == "" {
			continue
		}
		if !seen[ext] {
			formats = append(formats, ext)
			seen[ext] = true
		}
		if name == "" {
			name = entry.Name
		}
		if ext == "vtt" || ext == "srt" {
			compatible = true
		}
	}
	if len(formats) == 0 || !compatible {
		return SubtitleOption{}, false
	}
	sort.Strings(formats)
	translated := source == "automatic" && automaticTrackIsTranslated(entries)
	return SubtitleOption{
		Language:   language,
		Name:       name,
		Source:     source,
		Formats:    formats,
		Original:   source == "automatic" && !translated,
		Translated: translated,
	}, true
}

// yt-dlp exposes the translation choice in the track URL as tlang. This uses
// the extractor metadata itself instead of inferring meaning from a language
// key or constructing a synthetic subtitle identifier.
func automaticTrackIsTranslated(entries []subtitleEntry) bool {
	for _, entry := range entries {
		parsed, err := url.Parse(entry.URL)
		if err != nil {
			continue
		}
		if parsed.Query().Get("tlang") != "" {
			return true
		}
	}
	return false
}

func automaticTrackIsOriginal(meta *Metadata, key string) bool {
	entries, ok := meta.AutomaticCaptions[key]
	if !ok {
		actual, _, found := lookupTrack(meta.AutomaticCaptions, key)
		if !found {
			return false
		}
		entries = meta.AutomaticCaptions[actual]
	}
	return !automaticTrackIsTranslated(entries)
}

func languageBase(language string) string {
	if index := strings.IndexByte(language, '-'); index >= 0 {
		return language[:index]
	}
	return language
}

func matchingKeys(tracks trackList, base, exact string) []string {
	keys := sortedKeys(tracks)
	ordered := make([]string, 0, len(keys))
	for _, key := range keys {
		lower := strings.ToLower(key)
		if lower == exact {
			continue
		}
		if lower == base || languageBase(lower) == base {
			ordered = append(ordered, key)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		iBase := strings.EqualFold(ordered[i], base)
		jBase := strings.EqualFold(ordered[j], base)
		if iBase != jBase {
			return iBase
		}
		return strings.ToLower(ordered[i]) < strings.ToLower(ordered[j])
	})
	return ordered
}

func originalAutomaticKeys(meta *Metadata, base string) []string {
	keys := sortedKeys(meta.AutomaticCaptions)
	ordered := make([]string, 0, 2)
	for _, key := range keys {
		lower := strings.ToLower(key)
		if languageBase(lower) == base && automaticTrackIsOriginal(meta, key) {
			ordered = append(ordered, key)
		}
	}
	return ordered
}

func selectionSource(selection subSelection) string {
	if selection.manual {
		return "manual"
	}
	return "automatic"
}

func manualTrack(meta *Metadata, key string) (subSelection, bool) {
	actual, entries, ok := lookupTrack(meta.Subtitles, key)
	if !ok || !hasCompatibleEntry(entries) {
		return subSelection{}, false
	}
	return subSelection{language: actual, manual: true}, true
}

func autoTrack(meta *Metadata, key string) (subSelection, bool) {
	actual, entries, ok := lookupTrack(meta.AutomaticCaptions, key)
	if !ok || !hasCompatibleEntry(entries) {
		return subSelection{}, false
	}
	return subSelection{language: actual, manual: false}, true
}

func lookupTrack(tracks trackList, requested string) (string, []subtitleEntry, bool) {
	if entries, ok := tracks[requested]; ok {
		return requested, entries, true
	}
	for key, entries := range tracks {
		if strings.EqualFold(key, requested) {
			return key, entries, true
		}
	}
	return "", nil, false
}

func hasCompatibleEntry(entries []subtitleEntry) bool {
	for _, entry := range entries {
		ext := strings.ToLower(entry.Ext)
		if ext == "vtt" || ext == "srt" {
			return true
		}
	}
	return false
}

func sortedKeys(tracks trackList) []string {
	keys := make([]string, 0, len(tracks))
	for key := range tracks {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := strings.ToLower(keys[i]), strings.ToLower(keys[j])
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	return keys
}

func sortedTrackKeys(meta *Metadata) []string {
	seen := make(trackList)
	for key := range meta.Subtitles {
		seen[key] = nil
	}
	for key := range meta.AutomaticCaptions {
		seen[key] = nil
	}
	return sortedKeys(seen)
}

func (m *Metadata) hasAnyTrack() bool {
	return len(m.Subtitles) > 0 || len(m.AutomaticCaptions) > 0
}
