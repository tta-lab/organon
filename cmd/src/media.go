package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// errUnsupportedMedia reports a recognized-but-unsupported image variant or an
// unsupported binary input so the caller fails visibly instead of producing
// corrupted text.
var errUnsupportedMedia = errors.New("unsupported media input")

// mediaKind identifies a recognized image for the Pi adapter.
type mediaKind string

const (
	mediaImage = "image"
)

// detectMedia recognizes image input by content signature using the same
// signatures as the Pi built-in read: non-animated PNG, JPEG, GIF, WebP, and
// BMP. It returns (media, ok) when the input is a supported image. When the
// input is a recognized-but-unsupported image (for example an animated PNG) or
// another binary format, it returns an errUnsupportedMedia so the caller can
// fail visibly rather than decode the bytes as UTF-8 text.
func detectMedia(source []byte) (mediaJSON, bool) {
	kind, mime, ok := detectMediaType(source)
	if !ok {
		return mediaJSON{}, false
	}
	return mediaJSON{
		Kind:       kind,
		Mime:       mime,
		DataBase64: base64.StdEncoding.EncodeToString(source),
	}, true
}

// detectMediaType inspects content signatures, mirroring Pi's mime detection.
// The bool result reports whether the bytes are a supported image.
func detectMediaType(source []byte) (string, string, bool) {
	if isJPEG(source) {
		return mediaImage, "image/jpeg", true
	}
	if isPNG(source) {
		return mediaImage, "image/png", true
	}
	if hasPrefix(source, []byte("GIF")) {
		return mediaImage, "image/gif", true
	}
	if hasPrefix(source, []byte("RIFF")) && hasPrefixAt(source, 8, []byte("WEBP")) {
		return mediaImage, "image/webp", true
	}
	if isBMP(source) {
		return mediaImage, "image/bmp", true
	}
	return "", "", false
}

// mediaErrorFor produces a visible failure for unsupported media input.
func mediaErrorFor(source []byte, filename string) error {
	if looksLikeImageButUnsupported(source) {
		return fmt.Errorf("%s is a recognized image variant that is not supported (animated PNG or similar); "+
			"use a supported image format: non-animated PNG, JPEG, GIF, WebP, or BMP", filename)
	}
	return fmt.Errorf("%s is a binary file and cannot be read as text", filename)
}

// looksLikeImageButUnsupported reports image-like signatures that detectMedia
// deliberately rejects, chiefly animated PNGs and JPEG 2000.
func looksLikeImageButUnsupported(source []byte) bool {
	if isPNGHeader(source) && isAnimatedPNG(source) {
		return true
	}
	if isJPEGHeader(source) && len(source) > 3 && source[3] == 0xF7 {
		return true
	}
	return false
}

func isJPEG(source []byte) bool {
	return isJPEGHeader(source) && !(len(source) > 3 && source[3] == 0xF7)
}

func isJPEGHeader(source []byte) bool {
	return hasPrefix(source, []byte{0xFF, 0xD8, 0xFF})
}

func isPNG(source []byte) bool {
	if !isPNGHeader(source) {
		return false
	}
	// IHDR must follow the 8-byte signature with length 13.
	if len(source) < 16 || readUint32BE(source, 8) != 13 || !hasPrefixAt(source, 12, []byte("IHDR")) {
		return false
	}
	return !isAnimatedPNG(source)
}

func isPNGHeader(source []byte) bool {
	return hasPrefix(source, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
}

// isAnimatedPNG scans PNG chunks before IDAT for an animation control chunk.
func isAnimatedPNG(source []byte) bool {
	offset := 8
	for offset+8 <= len(source) {
		chunkLength := int(readUint32BE(source, offset))
		chunkType := offset + 4
		if hasPrefixAt(source, chunkType, []byte("acTL")) {
			return true
		}
		if hasPrefixAt(source, chunkType, []byte("IDAT")) {
			return false
		}
		next := offset + 8 + chunkLength + 4
		if next <= offset || next > len(source) {
			return false
		}
		offset = next
	}
	return false
}

func isBMP(source []byte) bool {
	if !hasPrefix(source, []byte("BM")) || len(source) < 26 {
		return false
	}
	declaredFileSize := readUint32LE(source, 2)
	pixelDataOffset := readUint32LE(source, 10)
	dibHeaderSize := readUint32LE(source, 14)
	if declaredFileSize != 0 && declaredFileSize < 26 {
		return false
	}
	if pixelDataOffset < 14+dibHeaderSize {
		return false
	}
	if declaredFileSize != 0 && pixelDataOffset >= declaredFileSize {
		return false
	}
	var colorPlanes, bitsPerPixel uint16
	switch {
	case dibHeaderSize == 12:
		colorPlanes = readUint16LE(source, 22)
		bitsPerPixel = readUint16LE(source, 24)
	case dibHeaderSize >= 40 && dibHeaderSize <= 124:
		if len(source) < 30 {
			return false
		}
		colorPlanes = readUint16LE(source, 26)
		bitsPerPixel = readUint16LE(source, 28)
	default:
		return false
	}
	if colorPlanes != 1 {
		return false
	}
	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
		return true
	}
	return false
}

func hasPrefix(source, prefix []byte) bool {
	return len(source) >= len(prefix) && string(source[:len(prefix)]) == string(prefix)
}

func hasPrefixAt(source []byte, offset int, prefix []byte) bool {
	if offset < 0 || offset+len(prefix) > len(source) {
		return false
	}
	return string(source[offset:offset+len(prefix)]) == string(prefix)
}

func readUint32BE(source []byte, offset int) uint32 {
	if offset+4 > len(source) {
		return 0
	}
	return uint32(source[offset])<<24 | uint32(source[offset+1])<<16 | uint32(source[offset+2])<<8 | uint32(source[offset+3])
}

func readUint32LE(source []byte, offset int) uint32 {
	if offset+4 > len(source) {
		return 0
	}
	return uint32(source[offset]) | uint32(source[offset+1])<<8 | uint32(source[offset+2])<<16 | uint32(source[offset+3])<<24
}

func readUint16LE(source []byte, offset int) uint16 {
	if offset+2 > len(source) {
		return 0
	}
	return uint16(source[offset]) | uint16(source[offset+1])<<8
}

var _ = strings.TrimSpace
