package goheif

import (
	"bytes"
	"image"
	"io/ioutil"
	"testing"
)

func TestFormatRegistered(t *testing.T) {
	b, err := ioutil.ReadFile("testdata/camel.heic")
	if err != nil {
		t.Fatal(err)
	}

	img, dec, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("unable to decode heic image: %s", err)
	}

	if got, want := dec, "heic"; got != want {
		t.Errorf("unexpected decoder: got %s, want %s", got, want)
	}

	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 1596 || h != 1064 {
		t.Errorf("unexpected decoded image size: got %dx%d, want 1596x1064", w, h)
	}
}
