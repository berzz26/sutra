package ingestion

import "testing"

func TestSubtitleOptionsExposeSourcesAndFormats(t *testing.T) {
	options := subtitleOptions(&Metadata{
		Subtitles: trackList{"en": {{Ext: "vtt", Name: "English"}}},
		AutomaticCaptions: trackList{
			"en":      {{Ext: "vtt"}, {Ext: "srt"}, {Ext: "json3"}},
			"en-orig": {{Ext: "vtt", Name: "English (Original)"}},
		},
	})
	if len(options) != 3 {
		t.Fatalf("got %d options: %+v", len(options), options)
	}
	if options[0].Language != "en" || options[0].Source != "manual" || options[0].Name != "English" {
		t.Fatalf("unexpected manual option: %+v", options[0])
	}
	if options[1].Language != "en" || options[1].Source != "automatic" || len(options[1].Formats) != 3 {
		t.Fatalf("unexpected automatic option: %+v", options[1])
	}
}

func TestSubtitleOptionsIncludeAutomaticTracks(t *testing.T) {
	options := subtitleOptions(&Metadata{
		Subtitles:         trackList{"en": vttTrack()},
		AutomaticCaptions: trackList{"en": vttTrack(), "en-orig": vttTrack()},
	})
	if len(options) != 3 || options[1].Language != "en" || options[1].Source != "automatic" {
		t.Fatalf("got %+v", options)
	}
}

func TestSelectExactSubtitleTrackHonorsSource(t *testing.T) {
	meta := &Metadata{
		Subtitles:         trackList{"en": vttTrack()},
		AutomaticCaptions: trackList{"en": vttTrack(), "en-orig": vttTrack()},
	}
	selection, err := selectExactSubtitleTrack(meta, "en", "automatic")
	if err != nil || selection.language != "en" || selection.manual {
		t.Fatalf("got %+v, err=%v", selection, err)
	}
	selection, err = selectExactSubtitleTrack(meta, "en", "manual")
	if err != nil || selection.language != "en" || !selection.manual {
		t.Fatalf("got %+v, err=%v", selection, err)
	}
}
