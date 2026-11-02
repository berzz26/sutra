// Package ingestion orchestrates transcript ingestion from YouTube URLs and
// uploaded VTT/SRT files. It composes URL validation, yt-dlp extraction,
// subtitle parsing, artifact storage and persistence.
package ingestion

import (
	"fmt"
	"net/url"
	"strings"
)

// YouTubeRef is a canonical YouTube video reference.
type YouTubeRef struct {
	VideoID      string
	CanonicalURL string
}

// youtubeHosts lists the only hostnames (with optional leading "www." and
// "m." subdomains) that are accepted for ingestion.
var youtubeHosts = map[string]bool{
	"youtube.com":       true,
	"m.youtube.com":     true,
	"music.youtube.com": true,
	"www.youtube.com":   true,
	"youtu.be":          true,
	"www.youtu.be":      true,
}

// ParseYouTubeURL validates a URL, extracts the canonical video ID and
// produces a deterministic canonical form of the URL. It rejects playlists
// (and any other non-video path), non-HTTPS URLs and foreign domains.
func ParseYouTubeURL(rawURL string) (*YouTubeRef, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("%w: empty URL", ErrInvalidYouTubeURL)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed URL: %v", ErrInvalidYouTubeURL, err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: only https URLs are accepted", ErrInvalidYouTubeURL)
	}
	if u.User != nil || u.Port() != "" {
		return nil, fmt.Errorf("%w: credentials and custom ports are not accepted", ErrInvalidYouTubeURL)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidYouTubeURL)
	}
	if !youtubeHosts[host] {
		return nil, fmt.Errorf("%w: unsupported domain %q", ErrUnsupportedURL, host)
	}

	videoID, source, err := videoIDFromURL(u, host)
	if err != nil {
		return nil, err
	}
	if err := validateVideoID(videoID); err != nil {
		return nil, err
	}

	canonical := canonicalURL(u, source)
	if canonical == "" {
		canonical = fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)
	}
	return &YouTubeRef{VideoID: videoID, CanonicalURL: canonical}, nil
}

// videoIDFromURL extracts the ID from watch/shorts/live/youtu.be paths and
// rejects playlists and unknown paths.
func videoIDFromURL(u *url.URL, host string) (id, source string, err error) {
	const invalidID = ""
	switch {
	case host == "youtu.be" || host == "www.youtu.be":
		p := strings.Trim(u.Path, "/")
		if p == "" || strings.Contains(p, "/") {
			return invalidID, "", fmt.Errorf("%w: no video ID in youtu.be URL", ErrInvalidYouTubeURL)
		}
		return p, "short", nil
	case u.Path == "/watch":
		q := u.Query()
		v := q.Get("v")
		if v == "" {
			return invalidID, "", fmt.Errorf("%w: missing video ID in watch URL", ErrInvalidYouTubeURL)
		}
		if q.Get("list") != "" {
			return invalidID, "", fmt.Errorf("%w: playlists are not supported; submit a single watch URL", ErrPlaylistURL)
		}
		return v, "watch", nil
	case u.Path == "/playlist":
		return invalidID, "", fmt.Errorf("%w: playlists are not supported; submit a single watch URL", ErrPlaylistURL)
	case strings.HasPrefix(u.Path, "/shorts/"),
		strings.HasPrefix(u.Path, "/live/"),
		strings.HasPrefix(u.Path, "/embed/"):
		source = strings.Split(u.Path, "/")[1]
		p := strings.Trim(strings.TrimPrefix(u.Path, "/"+source+"/"), "/")
		if p == "" || strings.Contains(p, "/") {
			return invalidID, "", fmt.Errorf("%w: no video ID in %s URL", ErrInvalidYouTubeURL, source)
		}
		return p, source, nil
	default:
		return invalidID, "", fmt.Errorf("%w: unsupported YouTube path %q", ErrUnsupportedURL, u.Path)
	}
}

func validateVideoID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: missing video ID", ErrInvalidYouTubeURL)
	}
	if len(id) > 32 {
		return fmt.Errorf("%w: video ID too long", ErrInvalidYouTubeURL)
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case c == '-' || c == '_':
		default:
			return fmt.Errorf("%w: invalid characters in video ID %q", ErrInvalidYouTubeURL, id)
		}
	}
	return nil
}

// canonicalURL strips tracking and irrelevant query parameters. Watch URLs
// keep only the video id parameter.
func canonicalURL(u *url.URL, source string) string {
	switch source {
	case "watch":
		return "https://www.youtube.com/watch?v=" + url.QueryEscape(u.Query().Get("v"))
	default:
		return ""
	}
}
