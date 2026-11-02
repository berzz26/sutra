package transcript

import (
	"errors"
	"testing"
)

func TestParseVTT(t *testing.T) {
	data := []byte("WEBVTT\r\n\r\nNOTE\r\nthis --> is metadata\r\n\r\nintro\r\n00:00:01.200 --> 00:00:04.500\r\nHello &amp; <i>world</i>.\r\n\r\n00:01:05.000 --> 01:02:08.250 align:start\r\nline one\r\nline two\r\n")
	cues, err := Parse(FormatVTT, data)
	if err != nil || len(cues) != 2 {
		t.Fatalf("cues=%+v err=%v", cues, err)
	}
	if cues[0].Seq != 1 || cues[0].StartMs != 1200 || cues[0].Text != "Hello & world." {
		t.Fatalf("unexpected first cue: %+v", cues[0])
	}
	if cues[1].EndMs != 3728250 || cues[1].Text != "line one line two" {
		t.Fatalf("unexpected second cue: %+v", cues[1])
	}
}

func TestParseSRTAndRejectInvalidTimestamp(t *testing.T) {
	data := []byte("1\n00:00:01,200 --> 00:00:04,500\nWelcome.\n\n2\n00:00:05,000 --> 00:00:08,300\nToday we're\ndiscussing.\n")
	cues, err := Parse(FormatSRT, data)
	if err != nil || len(cues) != 2 || cues[1].Text != "Today we're discussing." {
		t.Fatalf("cues=%+v err=%v", cues, err)
	}
	_, err = Parse(FormatVTT, []byte("WEBVTT\n\n00:00:02.000 --> 00:00:02.000\nempty timing\n"))
	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("expected invalid timestamp, got %v", err)
	}
}

func TestParseDropsEmptyCuesAndRequiresHeader(t *testing.T) {
	cues, err := Parse(FormatVTT, []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\n\n00:00:03.000 --> 00:00:04.000\nkept\n"))
	if err != nil || len(cues) != 1 || cues[0].Seq != 1 {
		t.Fatalf("cues=%+v err=%v", cues, err)
	}
	if _, err := Parse(FormatVTT, []byte("not vtt")); err == nil {
		t.Fatal("expected invalid VTT header")
	}
}
