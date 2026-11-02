package ingestion

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func vttTrack() []subtitleEntry { return []subtitleEntry{{Ext: "vtt"}} }

func TestSelectSubtitleTrack(t *testing.T) {
	tests := []struct {
		name      string
		manual    trackList
		automatic trackList
		requested string
		language  string
		manualOut bool
	}{
		{"original English automatic", nil, trackList{"en": vttTrack(), "en-orig": vttTrack()}, "en", "en-orig", false},
		{"regular English fallback", nil, trackList{"en": vttTrack()}, "en", "en", false},
		{"manual wins", trackList{"en": vttTrack()}, trackList{"en-orig": vttTrack()}, "en", "en", true},
		{"original only", nil, trackList{"en-orig": vttTrack()}, "en", "en-orig", false},
		{"exact regional wins", nil, trackList{"en-US": vttTrack(), "en-GB": vttTrack(), "en-orig": vttTrack()}, "en-US", "en-US", false},
		{"regional manual family", trackList{"en-GB": vttTrack()}, trackList{"en-orig": vttTrack()}, "en-US", "en-GB", true},
		{"other language original absent", nil, trackList{"hi": vttTrack()}, "hi", "hi", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selection, err := selectSubtitleTrack(&Metadata{Subtitles: tt.manual, AutomaticCaptions: tt.automatic}, tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if selection.language != tt.language || selection.manual != tt.manualOut {
				t.Fatalf("got language=%q manual=%v, want language=%q manual=%v", selection.language, selection.manual, tt.language, tt.manualOut)
			}
		})
	}
}

func TestSelectSubtitleTrackLanguageUnavailable(t *testing.T) {
	_, err := selectSubtitleTrack(&Metadata{AutomaticCaptions: trackList{"hi-orig": vttTrack()}}, "en")
	var ingestionErr *Error
	if !errors.As(err, &ingestionErr) || ingestionErr.Code != ErrCodeLanguageUnavailable {
		t.Fatalf("got %v, want %s", err, ErrCodeLanguageUnavailable)
	}
}

func TestSelectSubtitleTrackRegionalOrderIsDeterministic(t *testing.T) {
	meta := &Metadata{AutomaticCaptions: trackList{"en-ZA": vttTrack(), "en-GB": vttTrack(), "en-US": vttTrack()}}
	for i := 0; i < 20; i++ {
		selection, err := selectSubtitleTrack(meta, "en")
		if err != nil || selection.language != "en-GB" {
			t.Fatalf("iteration %d: got %+v, err=%v", i, selection, err)
		}
	}
}

func TestSubtitleDownloadArgsPreserveOriginalAutomaticTrack(t *testing.T) {
	args := subtitleDownloadArgs(subSelection{language: "en-orig", manual: false}, "/tmp/subtitles", "https://www.youtube.com/watch?v=test")
	joined := strings.Join(args, "\x00")
	if !hasArg(args, "--write-auto-subs") || !strings.Contains(joined, "--sub-langs\x00en-orig") {
		t.Fatalf("args=%v", args)
	}
	if hasArg(args, "--write-subs") {
		t.Fatalf("automatic command unexpectedly contains --write-subs: %v", args)
	}
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func TestClassifyYTDLPRateLimit(t *testing.T) {
	err := classifyYTDLPEnd(context.Background(), "ERROR: Unable to download subtitles: HTTP Error 429: Too Many Requests")
	if err.Code != ErrCodeYTDLPRateLimited {
		t.Fatalf("got %s, want %s", err.Code, ErrCodeYTDLPRateLimited)
	}
}
