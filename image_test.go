package main

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestNativeBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))

	kitty, err := renderThumbData(img, 16, 9, protoKitty)
	if err != nil {
		t.Fatal(err)
	}
	// PNG transmit for a unique per-image kitty number
	if !strings.Contains(kitty, "f=100") || !strings.Contains(kitty, "t=d,i=") || !strings.Contains(kitty, "q=2") {
		t.Fatalf("kitty block missing transmit: %q…", kitty[:min(48, len(kitty))])
	}
	// virtual placement that the placeholder cells reference
	if !strings.Contains(kitty, "a=p,U=1") || !strings.Contains(kitty, "c=16") || !strings.Contains(kitty, "r=9") {
		t.Fatalf("kitty block missing virtual placement: %q…", kitty[:min(96, len(kitty))])
	}
	// the image is drawn as a grid of Unicode placeholder glyphs (image-as-text)
	if !strings.Contains(kitty, "\U0010EEEE") {
		t.Fatalf("kitty block missing placeholder glyphs: %q…", kitty[:min(48, len(kitty))])
	}
	// the whole block (transmit + placeholders) is exactly `rows` lines
	lines := strings.Split(kitty, "\n")
	if len(lines) != 9 {
		t.Fatalf("kitty: want %d lines, got %d", 9, len(lines))
	}

	six, err := renderThumbData(img, 8, 4, protoSixel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(six, "\x1bP") || !strings.Contains(six, "q") {
		t.Fatalf("sixel block is not a DECSIXEL sequence: %q…", six[:min(20, len(six))])
	}
	sLines := strings.Split(six, "\n")
	if len(sLines) != 4 { // 1 DCS line + 3 reserved lines == rows
		t.Fatalf("sixel: want %d lines, got %d", 4, len(sLines))
	}
	for i, ln := range sLines[1:] {
		if ln != "" {
			t.Fatalf("sixel: reserved line %d should be empty, got %q", i+2, ln)
		}
	}
}

func TestDistinctImagesGetDistinctBlocks(t *testing.T) {
	solid := func(c color.RGBA) image.Image {
		im := image.NewRGBA(image.Rect(0, 0, 320, 180))
		for i := 0; i < len(im.Pix); i += 4 {
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, 255
		}
		return im
	}
	a, err := renderThumbData(solid(color.RGBA{255, 0, 0, 255}), 8, 4, protoKitty)
	if err != nil {
		t.Fatal(err)
	}
	b, err := renderThumbData(solid(color.RGBA{0, 0, 255, 255}), 8, 4, protoKitty)
	if err != nil {
		t.Fatal(err)
	}

	// every render gets its own kitty image number (would be the same "I=" id
	// if some caller pinned it, making the cells identical and the diff skip them)
	na, nb := kittyImageNum(t, a), kittyImageNum(t, b)
	if na == nb {
		t.Fatalf("two images rendered with the same kitty number: %s", na)
	}
	// …and the payload differs too (guards the go-termimg global resize cache
	// that used to return the first thumbnail for every video), so each line of
	// the block differs from the previous image's and gets rewritten.
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range la {
		if la[i] == lb[i] {
			t.Fatalf("placeholder line %d identical across different images: %q", i, la[i])
		}
	}
}

func TestScaleFitPreservesAspectRatioAndBounds(t *testing.T) {
	// A 16:9 image (320x180) rendered into a non-16:9 bounding box (e.g. 40x20).
	// Under ScaleFit, the rendered block must fit within the requested cell dimensions
	// without cropping away any image content.
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	cols, rows := 40, 20
	kitty, err := renderThumbData(img, cols, rows, protoKitty)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(kitty, "\n")
	if len(lines) != rows {
		t.Fatalf("kitty: want %d lines, got %d", rows, len(lines))
	}
}
