package main

// detectMedia recognizes image input by content signature. Full signature
// detection (PNG, JPEG, GIF, WebP, BMP) lands with the media read ticket; the
// stub keeps the read JSON path decoupled until then.
func detectMedia(source []byte) (mediaJSON, bool) {
	return mediaJSON{}, false
}
