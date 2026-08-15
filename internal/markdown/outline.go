package markdown

import "bytes"

// Heading describes one Markdown heading and the section it begins.
type Heading struct {
	ID         string
	Name       string
	Parent     string
	Level      int
	StartByte  int
	EndByte    int
	StartLine  int
	EndLine    int
	Targetable bool
}

// Outline returns an optional first-H1 title and every targetable heading in
// document order. The title is display metadata; its H1 remains an addressable
// section with the same stable ID used by section read and edit operations.
func Outline(source []byte) (string, []Heading, error) {
	headings, err := parseHeadings(source)
	if err != nil {
		return "", nil, err
	}
	assignIDs(headings)

	title := ""
	for _, heading := range headings {
		if heading.level == 1 {
			title = heading.text
			break
		}
	}
	result := make([]Heading, 0, len(headings))
	for i, heading := range headings {
		end := len(source)
		for _, next := range headings[i+1:] {
			if next.level <= heading.level {
				end = next.offset
				break
			}
		}
		parent := ""
		for previous := i - 1; previous >= 0; previous-- {
			if headings[previous].level < heading.level {
				parent = headings[previous].text
				break
			}
		}
		result = append(result, Heading{
			ID: heading.id, Name: heading.text, Parent: parent, Level: heading.level,
			StartByte: heading.offset, EndByte: end,
			StartLine: lineAt(source, heading.offset), EndLine: sectionEndLine(source, heading.offset, end),
			Targetable: heading.id != "",
		})
	}
	return title, result, nil
}

func lineAt(source []byte, offset int) int {
	return bytes.Count(source[:offset], []byte{'\n'}) + 1
}

func sectionEndLine(source []byte, start, end int) int {
	if end <= start {
		return lineAt(source, start)
	}
	return lineAt(source, end-1)
}
