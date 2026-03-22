package markdown

import "fmt"

// HeadingTree returns a rendered heading tree string for the given markdown source.
func HeadingTree(source []byte) (string, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return "", err
	}
	assignIDs(headings)
	return renderTree(headings, source), nil
}

// ReadSection extracts the content of a section identified by sectionID.
func ReadSection(source []byte, sectionID string) (string, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return "", err
	}
	assignIDs(headings)
	return extractSection(source, headings, sectionID)
}

// ReplaceSection replaces the content of a section (heading line through end of section)
// with newContent. Returns the modified document.
func ReplaceSection(source []byte, sectionID string, newContent []byte) ([]byte, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return nil, err
	}
	assignIDs(headings)

	start, end, err := sectionBounds(source, headings, sectionID)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, start+len(newContent)+len(source)-end+1)
	result = append(result, source[:start]...)
	result = append(result, newContent...)
	if len(newContent) > 0 && newContent[len(newContent)-1] != '\n' {
		result = append(result, '\n')
	}
	result = append(result, source[end:]...)
	return result, nil
}

// InsertBeforeSection inserts newContent immediately before the target section's heading line.
func InsertBeforeSection(source []byte, sectionID string, newContent []byte) ([]byte, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return nil, err
	}
	assignIDs(headings)

	start, _, err := sectionBounds(source, headings, sectionID)
	if err != nil {
		return nil, err
	}

	var result []byte
	result = append(result, source[:start]...)
	result = append(result, newContent...)
	if len(newContent) > 0 && newContent[len(newContent)-1] != '\n' {
		result = append(result, '\n')
	}
	result = append(result, source[start:]...)
	return result, nil
}

// InsertAfterSection inserts newContent after the last byte of the target section
// (before the next section or end of document).
func InsertAfterSection(source []byte, sectionID string, newContent []byte) ([]byte, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return nil, err
	}
	assignIDs(headings)

	_, end, err := sectionBounds(source, headings, sectionID)
	if err != nil {
		return nil, err
	}

	var result []byte
	result = append(result, source[:end]...)
	if end > 0 && source[end-1] != '\n' {
		result = append(result, '\n')
	}
	result = append(result, newContent...)
	if len(newContent) > 0 && newContent[len(newContent)-1] != '\n' {
		result = append(result, '\n')
	}
	result = append(result, source[end:]...)
	return result, nil
}

// DeleteSection removes a section (heading line through end of section) from the document.
func DeleteSection(source []byte, sectionID string) ([]byte, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return nil, err
	}
	assignIDs(headings)

	start, end, err := sectionBounds(source, headings, sectionID)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 0, start+len(source)-end)
	result = append(result, source[:start]...)
	result = append(result, source[end:]...)
	return result, nil
}

// sectionBounds returns the [start, end) byte range of a section identified by sectionID.
// start is the byte offset of the heading line; end is the byte offset where the next
// same-or-higher-level heading begins (or len(source) for the last section).
func sectionBounds(source []byte, headings []mdHeading, sectionID string) (start, end int, err error) {
	targetIdx := -1
	for i, h := range headings {
		if h.id == sectionID {
			targetIdx = i
			break
		}
	}
	if targetIdx < 0 {
		var ids []string
		for _, h := range headings {
			if h.id != "" {
				ids = append(ids, fmt.Sprintf("%q (%s)", h.id, h.text))
			}
		}
		return 0, 0, fmt.Errorf("section %q not found; available: %v", sectionID, ids)
	}

	target := headings[targetIdx]
	start = target.offset
	end = len(source)
	for _, h := range headings[targetIdx+1:] {
		if h.level <= target.level {
			end = h.offset
			break
		}
	}
	return start, end, nil
}
