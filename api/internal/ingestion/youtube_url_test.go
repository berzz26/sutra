package ingestion

import "testing"

func TestParseYouTubeURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		id   string
		code error
	}{
		{"watch", "https://www.youtube.com/watch?v=abc_123", "abc_123", nil},
		{"short", "https://youtu.be/abc-123", "abc-123", nil},
		{"shorts", "https://www.youtube.com/shorts/abc123", "abc123", nil},
		{"live", "https://youtube.com/live/abc123", "abc123", nil},
		{"domain", "https://example.com/watch?v=abc123", "", ErrUnsupportedURL},
		{"http", "http://youtube.com/watch?v=abc123", "", ErrInvalidYouTubeURL},
		{"missing", "https://youtube.com/watch", "", ErrInvalidYouTubeURL},
		{"playlist", "https://youtube.com/watch?v=abc123&list=PL1", "", ErrPlaylistURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, err := ParseYouTubeURL(tt.url)
			if tt.code != nil {
				if err == nil || !containsError(err, tt.code.Error()) {
					t.Fatalf("expected %v, got %v", tt.code, err)
				}
				return
			}
			if err != nil || ref.VideoID != tt.id {
				t.Fatalf("got ref=%+v err=%v", ref, err)
			}
		})
	}
}

func containsError(err error, text string) bool {
	for err != nil {
		if err.Error() == text || len(err.Error()) >= len(text) && err.Error()[:len(text)] == text {
			return true
		}
		err = unwrap(err)
	}
	return false
}

func unwrap(err error) error {
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return wrapped.Unwrap()
	}
	return nil
}
