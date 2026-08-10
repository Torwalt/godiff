package gitx

import (
	"reflect"
	"testing"
)

func TestParseLog(t *testing.T) {
	in := "aaaaaaaa\x00aaaaaaa\x00newest subject\x00bbbbbbbb\x00bbbbbbb\x00older subject\x00"
	want := []Commit{
		{SHA: "aaaaaaaa", ShortSHA: "aaaaaaa", Subject: "newest subject"},
		{SHA: "bbbbbbbb", ShortSHA: "bbbbbbb", Subject: "older subject"},
	}

	got, err := ParseLog([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseLogMalformed(t *testing.T) {
	for _, in := range []string{
		"sha\x00short\x00",
		"\x00short\x00subject\x00",
	} {
		if _, err := ParseLog([]byte(in)); err == nil {
			t.Errorf("ParseLog(%q): expected error", in)
		}
	}
}

func TestLogPageRejectsInvalidBounds(t *testing.T) {
	repo := &Repo{}
	for _, bounds := range [][2]int{{-1, 10}, {0, 0}} {
		if _, _, err := repo.LogPage("master", bounds[0], bounds[1]); err == nil {
			t.Errorf("LogPage offset=%d limit=%d: expected error", bounds[0], bounds[1])
		}
	}
}
