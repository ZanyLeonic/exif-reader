package exif

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

type ContainerItem struct {
	Mime     string `xml:"Mime,attr"`
	Semantic string `xml:"Semantic,attr"`
	Length   int    `xml:"Length,attr,omitempty"`
}

type XmpMeta struct {
	XMLName xml.Name `xml:"xmpmeta"`
	RDF     struct {
		XMLName     xml.Name `xml:"RDF"`
		Description struct {
			XMLName          xml.Name `xml:"Description"`
			Version          string   `xml:"Version,attr"`
			HasExtendedXMP   string   `xml:"HasExtendedXMP,attr"`
			HdrPlusMakerNote string   `xml:"HdrPlusMakernote,attr"`
			Directory        struct {
				XMLName  xml.Name `xml:"Directory"`
				Sequence struct {
					XMLName xml.Name `xml:"Seq"`
					Items   []struct {
						XMLName       xml.Name      `xml:"li"`
						ParseType     string        `xml:"parseType,attr"`
						ContainerItem ContainerItem `xml:"Item"`
					} `xml:"li"`
				} `xml:"Seq"`
			} `xml:"Directory"`
		} `xml:"Description"`
	} `xml:"RDF"`
}

func ExtractXMPData(data []byte) (string, error) {
	xmpHeader := "http://ns.adobe.com/xap/1.0/\x00"
	for i := 0; i < len(data)-len(xmpHeader); i++ {
		start := 0
		if string(data[i:i+len(xmpHeader)]) == xmpHeader {
			start = i
		} else {
			continue
		}
		end := start
		for end < len(data)-11 {
			if string(data[end:end+12]) == "</x:xmpmeta>" {
				end += 12
				return strings.TrimLeft(string(data[start:end]), xmpHeader), nil
			}
			end++
		}
		return "", errors.New("XMP end tag not found")
	}
	return "", errors.New("XMP block not found")
}

func ExtractExtXMPData(data []byte, extId string) (string, error) {
	extHeader := fmt.Sprintf("http://ns.adobe.com/xmp/extension/\x00%s\x00", extId)
	for i := 0; i < len(data)-len(extHeader); i++ {
		start := 0
		if string(data[i:i+len(extHeader)]) == extHeader {
			start = i
		} else {
			continue
		}
		end := start
		for end < len(data)-11 {
			if string(data[end:end+12]) == "</x:xmpmeta>" {
				end += 12
				// Skip past the header by moving start position
				start = start + len(extHeader)
				xmlString := string(data[start:end])

				// Find the actual XML start
				tagStart := strings.Index(xmlString, "<x:xmpmeta")
				if tagStart != -1 {
					xmlString = xmlString[tagStart:]
				}

				var b strings.Builder
				for _, c := range xmlString {
					if c == '\uFFFD' {
						continue
					}
					b.WriteRune(c)
				}

				return SanitizeXMLString(b.String()), nil
			}
			end++
		}
		return "", errors.New("XMP end tag not found")
	}
	return "", errors.New("extended XMP data not found")
}

func SanitizeXMLString(s string) string {
	s = strings.ToValidUTF8(s, "")
	re := regexp.MustCompile(`http://ns\.adobe\.com/xmp/extension/\x00[A-F0-9]+`)
	s = re.ReplaceAllString(s, "")

	// Remove illegal XML control characters
	// XML spec allows only: tab (0x09), newline (0x0A), carriage return (0x0D)
	// XML forbids: 0x00-0x08, 0x0B-0x0C, 0x0E-0x1F
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		// Keep if outside control char range OR is allowed whitespace
		if r >= 0x20 || r == 0x09 || r == 0x0A || r == 0x0D {
			result.WriteRune(r)
		}
		// Otherwise skip (illegal XML character like U+0008)
	}

	return result.String()
}

func SanitizeBase64String(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	var removedChars []rune
	var controlCharCounts = make(map[string]int)

	for _, r := range s {
		// Keep only valid base64 characters (ASCII range only)
		if (r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '+' || r == '/' || r == '=' {
			result.WriteRune(r)
		} else {
			// Track what types of characters we're removing
			if len(removedChars) < 100 {
				removedChars = append(removedChars, r)
			}

			// Count specific control characters
			switch r {
			case 0x00:
				controlCharCounts["NULL(0x00)"]++
			case 0x08:
				controlCharCounts["BACKSPACE(0x08)"]++
			case 0x09:
				controlCharCounts["TAB(0x09)"]++
			case 0x0A:
				controlCharCounts["LF(0x0A)"]++
			case 0x0D:
				controlCharCounts["CR(0x0D)"]++
			case 0x1B:
				controlCharCounts["ESC(0x1B)"]++
			case 0x20:
				controlCharCounts["SPACE(0x20)"]++
			default:
				if r < 0x20 {
					controlCharCounts[fmt.Sprintf("CTRL(0x%02X)", r)]++
				} else if r >= 0x7F {
					controlCharCounts[fmt.Sprintf("UNICODE(U+%04X)", r)]++
				} else {
					controlCharCounts[fmt.Sprintf("OTHER('%c'/0x%02X)", r, r)]++
				}
			}
		}
	}

	// Log what was removed if there were invalid characters
	if len(removedChars) > 0 {
		var charDetails []string
		for i, r := range removedChars {
			if i >= 10 { // Only show first 10 for brevity
				charDetails = append(charDetails, fmt.Sprintf("... and %d more", len(removedChars)-10))
				break
			}

			// Give descriptive names to control characters
			var desc string
			switch r {
			case 0x00:
				desc = "NULL(0x00)"
			case 0x08:
				desc = "BACKSPACE(0x08)"
			case 0x09:
				desc = "TAB(0x09)"
			case 0x0A:
				desc = "LF(0x0A)"
			case 0x0D:
				desc = "CR(0x0D)"
			case 0x1B:
				desc = "ESC(0x1B)"
			case 0x20:
				desc = "SPACE(0x20)"
			default:
				if r < 0x20 {
					desc = fmt.Sprintf("CTRL(0x%02X)", r)
				} else if r < 0x7F {
					desc = fmt.Sprintf("'%c'(0x%02X)", r, r)
				} else {
					desc = fmt.Sprintf("U+%04X", r)
				}
			}
			charDetails = append(charDetails, desc)
		}

		// Create summary of control character counts
		var countSummary []string
		for name, count := range controlCharCounts {
			countSummary = append(countSummary, fmt.Sprintf("%s:%d", name, count))
		}

		slog.Info("Removed invalid characters from base64",
			"totalRemoved", len(removedChars),
			"first10", strings.Join(charDetails, ", "),
			"summary", strings.Join(countSummary, ", "))
	}

	cleaned := result.String()

	// Fix padding: base64 strings should have length divisible by 4
	// Remove any existing padding first, then add correct padding
	cleaned = strings.TrimRight(cleaned, "=")

	// Calculate how many padding characters we need
	mod := len(cleaned) % 4
	if mod > 0 {
		paddingNeeded := 4 - mod
		slog.Info("Fixing base64 padding",
			"originalLength", len(cleaned),
			"mod4", mod,
			"paddingAdded", paddingNeeded)
		cleaned += strings.Repeat("=", paddingNeeded)
	}

	return cleaned
}
