package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// --- synthetic images -------------------------------------------------------------------
//
// These are structurally valid containers with nonsense pixel data. Every parser in imagemeta.go
// only walks segment and chunk headers, so that is exactly what needs exercising — and building
// the bytes by hand keeps the test readable about which byte means what.

func imgPNGChunk(typ string, payload []byte) []byte {
	out := make([]byte, 4, len(payload)+12)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(payload)))
	out = append(out, typ...)
	out = append(out, payload...)
	return append(out, 0, 0, 0, 0) // CRC placeholder: the stripper does not verify checksums
}

func imgPNG(w, h int, extra ...[]byte) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(h))
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA

	out := append([]byte{}, pngSignature...)
	out = append(out, imgPNGChunk("IHDR", ihdr)...)
	for _, c := range extra {
		out = append(out, c...)
	}
	out = append(out, imgPNGChunk("IDAT", []byte("compressed pixels"))...)
	return append(out, imgPNGChunk("IEND", nil)...)
}

func imgJPEGSegment(marker byte, payload []byte) []byte {
	seg := []byte{0xFF, marker, byte((len(payload) + 2) >> 8), byte((len(payload) + 2) & 0xFF)}
	return append(seg, payload...)
}

func imgJPEG(w, h int, extra ...[]byte) []byte {
	out := []byte{0xFF, 0xD8} // SOI
	out = append(out, imgJPEGSegment(0xE0, append([]byte("JFIF\x00"), 1, 2, 0, 0, 1, 0, 1, 0, 0))...)
	for _, s := range extra {
		out = append(out, s...)
	}
	sof := []byte{8, byte(h >> 8), byte(h), byte(w >> 8), byte(w), 3, 1, 0x22, 0, 2, 0x11, 1, 3, 0x11, 1}
	out = append(out, imgJPEGSegment(0xC0, sof)...)
	out = append(out, imgJPEGSegment(0xDA, []byte{1, 1, 0, 0, 63, 0})...) // SOS
	out = append(out, 0x12, 0x34, 0x56, 0x78)                            // entropy-coded data
	return append(out, 0xFF, 0xD9)                                       // EOI
}

func imgWebPChunk(fourcc string, payload []byte) []byte {
	out := make([]byte, 0, len(payload)+9)
	out = append(out, fourcc...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(payload)))
	out = append(out, size[:]...)
	out = append(out, payload...)
	if len(payload)%2 == 1 {
		out = append(out, 0) // RIFF chunks are padded to an even length
	}
	return out
}

func imgVP8X(w, h int, flags byte) []byte {
	p := make([]byte, 10)
	p[0] = flags
	cw, ch := w-1, h-1 // the canvas fields are stored minus one
	p[4], p[5], p[6] = byte(cw), byte(cw>>8), byte(cw>>16)
	p[7], p[8], p[9] = byte(ch), byte(ch>>8), byte(ch>>16)
	return imgWebPChunk("VP8X", p)
}

func imgWebP(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := make([]byte, 0, len(body)+8)
	out = append(out, "RIFF"...)
	var size [4]byte
	binary.LittleEndian.PutUint32(size[:], uint32(len(body)))
	out = append(out, size[:]...)
	return append(out, body...)
}

// --- JPEG -------------------------------------------------------------------------------

func TestStripJPEGRemovesExif(t *testing.T) {
	// A photo taken on a phone carries GPS coordinates in APP1. Posting one as an avatar or a
	// task attachment published the photographer's home address to everyone in the space.
	exif := imgJPEGSegment(0xE1, []byte("Exif\x00\x00GPS 55.7558 37.6173 iPhone 15"))
	comment := imgJPEGSegment(0xFE, []byte("internal draft, do not share"))
	iptc := imgJPEGSegment(0xED, []byte("Photoshop 3.0\x00byline: Vlad"))
	icc := imgJPEGSegment(0xE2, []byte("ICC_PROFILE\x00colour data"))

	src := imgJPEG(800, 600, exif, comment, iptc, icc)
	got := stripJPEGMetadata(src)

	for _, secret := range []string{"GPS 55.7558", "iPhone 15", "internal draft", "byline: Vlad"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Errorf("metadata survived stripping: %q", secret)
		}
	}
	// Colour management and the JFIF header stay: dropping them changes how the image renders.
	if !bytes.Contains(got, []byte("ICC_PROFILE")) {
		t.Error("the ICC colour profile was dropped")
	}
	if !bytes.Contains(got, []byte("JFIF")) {
		t.Error("the JFIF header was dropped")
	}
	if !bytes.Contains(got, []byte{0x12, 0x34, 0x56, 0x78}) {
		t.Error("the scan data was altered")
	}
	if !bytes.HasSuffix(got, []byte{0xFF, 0xD9}) {
		t.Error("the end-of-image marker is missing")
	}
	if len(got) >= len(src) {
		t.Errorf("nothing was removed: %d bytes in, %d out", len(src), len(got))
	}
	if w, h, ok := jpegDimensions(got); !ok || w != 800 || h != 600 {
		t.Errorf("dimensions after stripping = %dx%d (ok=%v), want 800x600", w, h, ok)
	}
}

func TestStripJPEGKeepsCleanFileIntact(t *testing.T) {
	src := imgJPEG(64, 48)
	if got := stripJPEGMetadata(src); !bytes.Equal(got, src) {
		t.Error("a file with no metadata was modified anyway")
	}
}

func TestJPEGDimensions(t *testing.T) {
	if w, h, ok := jpegDimensions(imgJPEG(1920, 1080)); !ok || w != 1920 || h != 1080 {
		t.Errorf("jpegDimensions = %dx%d (ok=%v), want 1920x1080", w, h, ok)
	}
}

// --- PNG --------------------------------------------------------------------------------

func TestStripPNGRemovesTextChunks(t *testing.T) {
	src := imgPNG(1024, 768,
		imgPNGChunk("tEXt", []byte("Comment\x00captured on a work laptop")),
		imgPNGChunk("zTXt", []byte("Software\x00\x00compressed note")),
		imgPNGChunk("iTXt", []byte("Author\x00\x00\x00\x00Vlad")),
		imgPNGChunk("eXIf", []byte("II*\x00exif payload")),
		imgPNGChunk("tIME", []byte{7, 0xE9, 7, 25, 12, 0, 0}),
		imgPNGChunk("iCCP", []byte("sRGB\x00\x00profile")),
	)
	got := stripPNGMetadata(src)

	for _, chunk := range []string{"tEXt", "zTXt", "iTXt", "eXIf", "tIME"} {
		if bytes.Contains(got, []byte(chunk)) {
			t.Errorf("%s chunk survived", chunk)
		}
	}
	if bytes.Contains(got, []byte("captured on a work laptop")) {
		t.Error("chunk contents survived")
	}
	if !bytes.Contains(got, []byte("iCCP")) {
		t.Error("the colour profile was dropped")
	}
	if !bytes.HasPrefix(got, pngSignature) {
		t.Error("the PNG signature was damaged")
	}
	if !bytes.Contains(got, []byte("IDAT")) || !bytes.Contains(got, []byte("IEND")) {
		t.Error("required chunks were removed")
	}
	if w, h, ok := pngDimensions(got); !ok || w != 1024 || h != 768 {
		t.Errorf("dimensions after stripping = %dx%d (ok=%v), want 1024x768", w, h, ok)
	}
}

func TestStripPNGKeepsCleanFileIntact(t *testing.T) {
	src := imgPNG(32, 32)
	if got := stripPNGMetadata(src); !bytes.Equal(got, src) {
		t.Error("a file with no metadata chunks was modified anyway")
	}
}

// --- WebP -------------------------------------------------------------------------------

func TestStripWebPRemovesExifAndXmp(t *testing.T) {
	src := imgWebP(
		imgVP8X(2048, 1536, 0x0C), // EXIF and XMP flags set
		imgWebPChunk("EXIF", []byte("II*\x00 gps and camera model")),
		imgWebPChunk("XMP ", []byte("<x:xmpmeta>author</x:xmpmeta>")),
		imgWebPChunk("VP8 ", []byte("lossy pixel data")),
	)
	got := stripWebPMetadata(src)

	if bytes.Contains(got, []byte("gps and camera model")) || bytes.Contains(got, []byte("xmpmeta")) {
		t.Error("metadata chunk contents survived")
	}
	if !bytes.Contains(got, []byte("lossy pixel data")) {
		t.Error("the image data chunk was removed")
	}
	// The RIFF header carries the total size; leaving the old value there produces a file that
	// decoders treat as truncated.
	if want := uint32(len(got) - 8); binary.LittleEndian.Uint32(got[4:8]) != want {
		t.Errorf("RIFF size = %d, want %d", binary.LittleEndian.Uint32(got[4:8]), want)
	}
	// The VP8X flags must stop advertising chunks that are no longer present.
	if i := bytes.Index(got, []byte("VP8X")); i < 0 {
		t.Fatal("the VP8X header chunk disappeared")
	} else if flags := got[i+8]; flags&0x0C != 0 {
		t.Errorf("VP8X still advertises EXIF/XMP: flags = %#02x", flags)
	}
	if w, h, ok := webpDimensions(got); !ok || w != 2048 || h != 1536 {
		t.Errorf("dimensions after stripping = %dx%d (ok=%v), want 2048x1536", w, h, ok)
	}
}

func TestWebPDimensions(t *testing.T) {
	src := imgWebP(imgVP8X(4000, 3000, 0), imgWebPChunk("VP8 ", []byte("data")))
	if w, h, ok := webpDimensions(src); !ok || w != 4000 || h != 3000 {
		t.Errorf("webpDimensions = %dx%d (ok=%v), want 4000x3000", w, h, ok)
	}
	if !isWebP(src) {
		t.Error("isWebP did not recognise a RIFF/WEBP container")
	}
	if isWebP(imgPNG(8, 8)) {
		t.Error("isWebP accepted a PNG")
	}
}

// --- damaged input ----------------------------------------------------------------------

// Nothing here parses hostile input for fun: a file that does not match its declared type, or is
// simply corrupt, must come back byte for byte rather than mangled or panicking. The upload is
// rejected later on its own merits; the stripper's job is to not make things worse.
func TestStripImageMetadataReturnsDamagedInputUnchanged(t *testing.T) {
	cases := map[string][]byte{
		"empty":              {},
		"garbage":            []byte("this is not an image at all"),
		"png signature only": append([]byte{}, pngSignature...),
		"truncated png":      imgPNG(10, 10)[:20],
		"jpeg soi only":      {0xFF, 0xD8},
		"truncated jpeg":     imgJPEG(10, 10)[:12],
		"riff header only":   []byte("RIFF\x04\x00\x00\x00WEBP"),
		"png chunk claiming a huge length": append(append([]byte{}, pngSignature...),
			0xFF, 0xFF, 0xFF, 0xFF, 't', 'E', 'X', 't'),
	}
	for _, mime := range []string{"image/jpeg", "image/png", "image/webp", "image/gif"} {
		for name, data := range cases {
			t.Run(mime+"/"+name, func(t *testing.T) {
				if got := stripImageMetadata(mime, data); !bytes.Equal(got, data) {
					t.Errorf("damaged input was rewritten: %d bytes in, %d out", len(data), len(got))
				}
			})
		}
	}
}

func TestStripImageMetadataLeavesGifAlone(t *testing.T) {
	// GIF extension blocks carry animation control (NETSCAPE looping in particular), so editing
	// them would break playback for no privacy gain.
	gif := append([]byte("GIF89a"), 0x40, 0x00, 0x30, 0x00, 0xF7, 0x00, 0x00)
	if got := stripImageMetadata("image/gif", gif); !bytes.Equal(got, gif) {
		t.Error("a GIF was modified")
	}
	if w, h, ok := gifDimensions(gif); !ok || w != 64 || h != 48 {
		t.Errorf("gifDimensions = %dx%d (ok=%v), want 64x48", w, h, ok)
	}
}

// --- decompression bombs ----------------------------------------------------------------

func TestImageTooLarge(t *testing.T) {
	// A 40000x40000 PNG of one flat colour compresses to a few kilobytes and expands to 6.4 GB
	// once decoded. The header is checked before anything decodes it.
	pixels, tooLarge := imageTooLarge("image/png", imgPNG(40000, 40000))
	if !tooLarge {
		t.Error("a 1.6-gigapixel canvas was accepted")
	}
	if pixels != 40000*40000 {
		t.Errorf("pixel count = %d", pixels)
	}

	if _, tooLarge := imageTooLarge("image/png", imgPNG(4000, 3000)); tooLarge {
		t.Error("an ordinary 12-megapixel photo was rejected")
	}
	// Unreadable headers give no answer rather than a false accusation.
	if _, tooLarge := imageTooLarge("image/png", []byte("garbage")); tooLarge {
		t.Error("unparseable data was reported as too large")
	}
}

// --- sanitizeUpload ---------------------------------------------------------------------

func TestSanitizeUploadCleansAndReportsSize(t *testing.T) {
	src := imgPNG(640, 480, imgPNGChunk("tEXt", []byte("Comment\x00location data")))

	payload, size, err := sanitizeUpload("image/png", bytes.NewReader(src))
	if err != nil {
		t.Fatalf("sanitizeUpload: %v", err)
	}
	got, err := io.ReadAll(payload)
	if err != nil {
		t.Fatalf("reading the sanitized payload: %v", err)
	}
	if bytes.Contains(got, []byte("location data")) {
		t.Error("metadata reached the returned payload")
	}
	// The caller bills storage quota against this number, so it has to describe the bytes that
	// actually get written — not the size the client claimed in the multipart header.
	if size != int64(len(got)) {
		t.Errorf("reported size %d does not match the %d bytes returned", size, len(got))
	}
	if size >= int64(len(src)) {
		t.Errorf("reported size %d is not smaller than the %d bytes uploaded", size, len(src))
	}
}

func TestSanitizeUploadRejectsBombs(t *testing.T) {
	_, _, err := sanitizeUpload("image/png", bytes.NewReader(imgPNG(40000, 40000)))
	if !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("error = %v, want ErrImageTooLarge", err)
	}
}

func TestSanitizeUploadPassesThroughNonImages(t *testing.T) {
	// Attachments are not restricted to images; anything else must arrive at the disk untouched.
	src := []byte("%PDF-1.7\nnot an image\n")
	payload, size, err := sanitizeUpload("application/pdf", bytes.NewReader(src))
	if err != nil {
		t.Fatalf("sanitizeUpload: %v", err)
	}
	got, err := io.ReadAll(payload)
	if err != nil {
		t.Fatalf("reading the payload: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Error("a non-image was altered")
	}
	if size != int64(len(src)) {
		t.Errorf("reported size = %d, want %d", size, len(src))
	}
}

func TestSanitizeUploadStreamsOversizedFiles(t *testing.T) {
	// Past the in-memory ceiling the file is streamed through unread rather than buffered, and
	// the size is reported as unknown so the caller falls back to counting bytes as it copies.
	src := bytes.Repeat([]byte{'a'}, maxInMemoryImage+1024)
	payload, size, err := sanitizeUpload("image/jpeg", bytes.NewReader(src))
	if err != nil {
		t.Fatalf("sanitizeUpload: %v", err)
	}
	if size != -1 {
		t.Errorf("size = %d, want -1 for the streaming path", size)
	}
	n, err := io.Copy(io.Discard, payload)
	if err != nil {
		t.Fatalf("draining the payload: %v", err)
	}
	if n != int64(len(src)) {
		t.Errorf("streamed %d bytes, want %d — the file was truncated", n, len(src))
	}
}
