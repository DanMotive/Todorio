package api

// Metadata stripping and dimension checks for uploaded images.
//
// Uploads are already restricted to four raster formats by sniffing the real bytes (see
// uploadAttachment), so an SVG or an HTML polyglot cannot get in through the extension or a
// forged Content-Type. Two problems were left untouched:
//
//  1. Metadata rides along with the file. A photo taken on a phone carries EXIF with GPS
//     coordinates, the device serial number and the exact capture time. Attaching a photo to a
//     task therefore hands the uploader's home address to everyone with access to the list, and
//     nothing in the UI ever shows those fields, so the leak is invisible to the person
//     uploading. Screenshots are not exempt either: editors write the author's name into PNG
//     tEXt chunks.
//  2. Pixel dimensions were never looked at. A "decompression bomb" — a 200 KB PNG that decodes
//     to 40000x40000 pixels, i.e. ~6 GB in memory — passes the byte-size limit untouched and
//     then freezes or kills the browser tab of every teammate who opens the task.
//
// Both are handled by walking the container structure only. Nothing is re-encoded: image data is
// copied through byte for byte, so quality, animation and colour profiles survive exactly as the
// user made them. Every parser bails out to "return the original bytes" the moment anything looks
// wrong — a file we cannot confidently parse is stored as it arrived rather than mangled into
// something that no longer opens.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// maxImagePixels is the largest canvas (width x height) an upload may declare. 50 megapixels is
// well past any real photo — a 50 MP phone camera sits right at it, a 4K screenshot uses a sixth
// of it — while still ruling out the bombs, which run to the billions of pixels.
const maxImagePixels = 50_000_000

// stripImageMetadata removes metadata blocks (EXIF, XMP, IPTC, textual comments) from an image,
// returning the cleaned bytes. Formats it does not understand, and files that fail to parse, come
// back unchanged.
func stripImageMetadata(mime string, data []byte) []byte {
	switch mime {
	case "image/jpeg":
		return stripJPEGMetadata(data)
	case "image/png":
		return stripPNGMetadata(data)
	case "image/webp":
		return stripWebPMetadata(data)
	default:
		// GIF has no EXIF container; its extension blocks carry animation control (the
		// NETSCAPE looping block especially), so editing them would break playback for no
		// privacy gain.
		return data
	}
}

// imageTooLarge reports whether the image declares a canvas above maxImagePixels. ok is false
// when the dimensions could not be read, in which case the caller should let the file through:
// refusing everything we cannot parse would reject valid uploads for no security benefit, as the
// byte-size limit still applies.
func imageTooLarge(mime string, data []byte) (pixels int64, tooLarge bool) {
	w, h, ok := imageDimensions(mime, data)
	if !ok {
		return 0, false
	}
	pixels = int64(w) * int64(h)
	return pixels, pixels > maxImagePixels
}

func imageDimensions(mime string, data []byte) (w, h int, ok bool) {
	switch mime {
	case "image/jpeg":
		return jpegDimensions(data)
	case "image/png":
		return pngDimensions(data)
	case "image/gif":
		return gifDimensions(data)
	case "image/webp":
		return webpDimensions(data)
	}
	return 0, 0, false
}

// ---------- JPEG ----------

// JPEG is a chain of segments: 0xFF, a marker byte, then (for most markers) a big-endian length
// covering the payload and the length field itself. Scanning stops at SOS, after which the rest
// of the file is entropy-coded image data with no further segment structure.
func stripJPEGMetadata(data []byte) []byte {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return data
	}
	out := make([]byte, 0, len(data))
	out = append(out, 0xFF, 0xD8)
	for i := 2; ; {
		if i+2 > len(data) || data[i] != 0xFF {
			return data // not where a marker should be: leave the file alone
		}
		marker := data[i+1]
		switch {
		case marker == 0xD9: // EOI
			return append(out, data[i:]...)
		case marker == 0xDA: // SOS — image data follows to the end
			return append(out, data[i:]...)
		case marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8):
			// Standalone markers: no length field.
			out = append(out, data[i], data[i+1])
			i += 2
			continue
		}
		if i+4 > len(data) {
			return data
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 || i+2+segLen > len(data) {
			return data
		}
		// Dropped: APP1 (EXIF and XMP — the GPS carrier), APP13 (Photoshop/IPTC credits) and COM
		// (free-text comments). Kept deliberately: APP0 (JFIF density), APP2 (ICC colour
		// profile) and APP14 (Adobe colour transform), because dropping those changes how the
		// image is displayed rather than what it says about its author.
		drop := marker == 0xE1 || marker == 0xED || marker == 0xFE
		if !drop {
			out = append(out, data[i:i+2+segLen]...)
		}
		i += 2 + segLen
	}
}

func jpegDimensions(data []byte) (int, int, bool) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0, false
	}
	for i := 2; ; {
		if i+4 > len(data) || data[i] != 0xFF {
			return 0, 0, false
		}
		marker := data[i+1]
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD8) {
			i += 2
			continue
		}
		if marker == 0xDA || marker == 0xD9 {
			return 0, 0, false // reached image data without a frame header
		}
		segLen := int(binary.BigEndian.Uint16(data[i+2 : i+4]))
		if segLen < 2 || i+2+segLen > len(data) {
			return 0, 0, false
		}
		// SOF0..SOF15 carry the frame size. 0xC4 (DHT), 0xC8 (JPG) and 0xCC (DAC) share the
		// range but are not frame headers.
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if segLen < 7 {
				return 0, 0, false
			}
			h := int(binary.BigEndian.Uint16(data[i+5 : i+7]))
			w := int(binary.BigEndian.Uint16(data[i+7 : i+9]))
			return w, h, w > 0 && h > 0
		}
		i += 2 + segLen
	}
}

// ---------- PNG ----------

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

// PNG is a signature followed by chunks of {length, type, payload, CRC}. Because each chunk
// carries its own CRC, whole chunks can be dropped without recomputing anything.
func stripPNGMetadata(data []byte) []byte {
	if len(data) < 8 || !bytes.Equal(data[:8], pngSignature) {
		return data
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:8]...)
	for i := 8; i+8 <= len(data); {
		length := int64(binary.BigEndian.Uint32(data[i : i+4]))
		typ := string(data[i+4 : i+8])
		end := int64(i) + 12 + length // 4 length + 4 type + payload + 4 CRC
		if end > int64(len(data)) {
			return data
		}
		switch typ {
		case "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
			// Author, software, capture time, and a full EXIF block. iCCP (colour profile) is
			// deliberately not in this list: it affects rendering, not privacy.
		default:
			out = append(out, data[i:end]...)
		}
		i = int(end)
		if typ == "IEND" {
			break // anything appended past IEND is not part of the image
		}
	}
	return out
}

func pngDimensions(data []byte) (int, int, bool) {
	// IHDR is required to be the first chunk: signature (8) + length/type (8) + width, height.
	if len(data) < 24 || !bytes.Equal(data[:8], pngSignature) || string(data[12:16]) != "IHDR" {
		return 0, 0, false
	}
	w := int(binary.BigEndian.Uint32(data[16:20]))
	h := int(binary.BigEndian.Uint32(data[20:24]))
	return w, h, w > 0 && h > 0
}

// ---------- GIF ----------

func gifDimensions(data []byte) (int, int, bool) {
	if len(data) < 10 || string(data[:3]) != "GIF" {
		return 0, 0, false
	}
	w := int(binary.LittleEndian.Uint16(data[6:8]))
	h := int(binary.LittleEndian.Uint16(data[8:10]))
	return w, h, w > 0 && h > 0
}

// ---------- WebP ----------

// WebP is a RIFF container: "RIFF", a little-endian size, "WEBP", then chunks of {fourCC, size,
// payload} where an odd-sized payload is padded to even.
func stripWebPMetadata(data []byte) []byte {
	if !isWebP(data) {
		return data
	}
	out := make([]byte, 0, len(data))
	out = append(out, data[:12]...)
	for i := 12; i+8 <= len(data); {
		typ := string(data[i : i+4])
		size := int64(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		end := int64(i) + 8 + size + size%2
		if end > int64(len(data)) {
			return data
		}
		switch typ {
		case "EXIF", "XMP ":
			// dropped
		default:
			start := len(out)
			out = append(out, data[i:end]...)
			if typ == "VP8X" && size >= 1 {
				// The extended-format header advertises which optional chunks exist. Now
				// that EXIF (0x08) and XMP (0x04) are gone, leaving their flags set would
				// describe a file that no longer matches itself, which strict decoders
				// reject.
				out[start+8] &^= 0x0C
			}
		}
		i = int(end)
	}
	// The RIFF size field counts everything after "RIFF" and the field itself.
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out
}

// ---------- upload entry point ----------

// maxInMemoryImage bounds how much of an upload is buffered in order to clean it. Metadata
// stripping needs the whole file in memory, and doing that for an unbounded upload would trade
// one memory problem for another — particularly since limits.uploads.max_file_size_mb can be set
// to 0, meaning "no limit". Anything larger than this streams straight to disk uncleaned, exactly
// as it did before: a 32 MB photo is far past what a phone camera produces, so in practice every
// real upload takes the cleaning path.
const maxInMemoryImage = 32 << 20

// ErrImageTooLarge is returned when an image's declared canvas exceeds maxImagePixels.
var ErrImageTooLarge = errors.New("image canvas is too large")

// sanitizeUpload reads an image, rejects decompression bombs, and returns a reader over the
// metadata-stripped bytes ready to be written to disk.
//
// The second return value is the exact byte count when the file was small enough to be buffered,
// or -1 when it is being streamed through unmodified. Callers use it to charge the storage quota
// the real size instead of the size the client claimed in the multipart header, which is
// self-reported and can be a lie in either direction.
func sanitizeUpload(mime string, src io.Reader) (io.Reader, int64, error) {
	buf, err := io.ReadAll(io.LimitReader(src, maxInMemoryImage+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(buf)) > maxInMemoryImage {
		// Too big to clean: hand back the bytes already read followed by the rest of the stream.
		return io.MultiReader(newBytesReader(buf), src), -1, nil
	}
	if pixels, tooLarge := imageTooLarge(mime, buf); tooLarge {
		return nil, 0, fmt.Errorf("%w: %d megapixels, the limit is %d",
			ErrImageTooLarge, pixels/1_000_000, maxImagePixels/1_000_000)
	}
	cleaned := stripImageMetadata(mime, buf)
	return newBytesReader(cleaned), int64(len(cleaned)), nil
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func webpDimensions(data []byte) (int, int, bool) {
	if !isWebP(data) {
		return 0, 0, false
	}
	for i := 12; i+8 <= len(data); {
		typ := string(data[i : i+4])
		size := int64(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		payload := int64(i) + 8
		end := payload + size + size%2
		if end > int64(len(data)) {
			return 0, 0, false
		}
		switch typ {
		case "VP8X":
			// Canvas size: two 24-bit little-endian values, each stored as size-1.
			if size < 10 {
				return 0, 0, false
			}
			p := data[payload:]
			w := (int(p[4]) | int(p[5])<<8 | int(p[6])<<16) + 1
			h := (int(p[7]) | int(p[8])<<8 | int(p[9])<<16) + 1
			return w, h, true
		case "VP8 ":
			// Lossy: 3-byte frame tag, the 3-byte start code 9D 01 2A, then 14-bit width and
			// height (the top two bits of each 16-bit value are a scale factor).
			if size < 10 || data[payload+3] != 0x9D || data[payload+4] != 0x01 || data[payload+5] != 0x2A {
				return 0, 0, false
			}
			w := int(binary.LittleEndian.Uint16(data[payload+6:payload+8])) & 0x3FFF
			h := int(binary.LittleEndian.Uint16(data[payload+8:payload+10])) & 0x3FFF
			return w, h, w > 0 && h > 0
		case "VP8L":
			// Lossless: signature byte 0x2F, then 14 bits of width-1 and 14 bits of height-1.
			if size < 5 || data[payload] != 0x2F {
				return 0, 0, false
			}
			bits := binary.LittleEndian.Uint32(data[payload+1 : payload+5])
			w := int(bits&0x3FFF) + 1
			h := int((bits>>14)&0x3FFF) + 1
			return w, h, true
		}
		i = int(end)
	}
	return 0, 0, false
}
