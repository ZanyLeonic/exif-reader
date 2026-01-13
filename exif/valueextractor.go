package exif

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf16"
)

type ValueExtractor struct {
	Data      []byte
	TiffStart int
	Endian    binary.ByteOrder
}

// getString extracts a string value from EXIF data
func (e *ValueExtractor) getString(entryOffset, offset, count int) string {
	if count <= 4 {
		return strings.TrimRight(string(e.Data[entryOffset+8:entryOffset+8+count]), "\x00")
	}
	return strings.TrimRight(string(e.Data[offset:offset+count]), "\x00")
}

// getRational extracts a rational value (numerator/denominator) from EXIF data
func (e *ValueExtractor) getRational(offset int, signed bool) float64 {
	if offset < 0 || offset+8 > len(e.Data) {
		return 0
	}

	var numerator float64
	var denominator float64

	if signed {
		numerator = float64(int32(e.Endian.Uint32(e.Data[offset : offset+4])))
		denominator = float64(int32(e.Endian.Uint32(e.Data[offset+4 : offset+8])))

	} else {
		numerator = float64(e.Endian.Uint32(e.Data[offset : offset+4]))
		denominator = float64(e.Endian.Uint32(e.Data[offset+4 : offset+8]))
	}

	if denominator == 0 {
		return 0
	}

	return numerator / denominator
}

// getRationalParts extracts the raw numerator and denominator from EXIF data
func (e *ValueExtractor) getRationalParts(offset int) (uint32, uint32) {
	if offset < 0 || offset+8 > len(e.Data) {
		return 0, 0
	}

	numerator := e.Endian.Uint32(e.Data[offset : offset+4])
	denominator := e.Endian.Uint32(e.Data[offset+4 : offset+8])

	return numerator, denominator
}

// getGPSCoordinate calculates GPS coordinates from degrees, minutes, seconds
func (e *ValueExtractor) getGPSCoordinate(offset int) float64 {
	degrees := e.getRational(offset, false)
	minutes := e.getRational(offset+8, false)
	seconds := e.getRational(offset+16, false)

	return degrees + (minutes / 60.0) + (seconds / 3600.0)
}

func (e *ValueExtractor) GetString(entry IFDEntry, entryOffset int) string {
	offset := e.TiffStart + int(entry.ValueOffset)
	return e.getString(entryOffset, offset, int(entry.Count))
}

func (e *ValueExtractor) GetUint32(entryOffset int) uint32 {
	if entryOffset < 0 || entryOffset+12 > len(e.Data) {
		return 0
	}
	return e.Endian.Uint32(e.Data[entryOffset+8 : entryOffset+12])
}

func (e *ValueExtractor) GetUint32Array(entry IFDEntry, count int) []uint32 {
	offset := e.TiffStart + int(entry.ValueOffset)

	if offset < 0 || offset+(count*4) > len(e.Data) {
		return nil
	}

	result := make([]uint32, count)
	for i := 0; i < count; i++ {
		result[i] = e.Endian.Uint32(e.Data[offset+(i*4) : offset+(i*4)+4])
	}
	return result
}

func (e *ValueExtractor) GetUint16(entryOffset int) uint16 {
	if entryOffset < 0 || entryOffset+10 > len(e.Data) {
		return 0
	}
	return e.Endian.Uint16(e.Data[entryOffset+8 : entryOffset+10])
}

func (e *ValueExtractor) GetUint8(entryOffset int) uint8 {
	if entryOffset < 0 || entryOffset+8 >= len(e.Data) {
		return 0
	}
	return e.Data[entryOffset+8]
}

func (e *ValueExtractor) GetUint8Array(entryOffset, numSlices int) []uint8 {
	val := make([]uint8, numSlices)
	copy(val, e.Data[entryOffset+8:entryOffset+8+numSlices])
	return val
}

func (e *ValueExtractor) GetRational(entry IFDEntry, nestedOffset int, signed bool) float64 {
	offset := e.TiffStart + int(entry.ValueOffset) + nestedOffset
	return e.getRational(offset, signed)
}

func (e *ValueExtractor) GetRationalParts(entry IFDEntry, nestedOffset int) (uint32, uint32) {
	offset := e.TiffStart + int(entry.ValueOffset) + nestedOffset
	return e.getRationalParts(offset)
}

func (e *ValueExtractor) GetGPSCoord(entry IFDEntry) float64 {
	offset := e.TiffStart + int(entry.ValueOffset)
	return e.getGPSCoordinate(offset)
}

func (e *ValueExtractor) GetByteArray(entry IFDEntry, entryOffset int) []byte {
	var offset int
	if entry.Count <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.TiffStart + int(entry.ValueOffset)
	}

	if offset < 0 || offset+int(entry.Count) > len(e.Data) {
		return nil
	}

	result := make([]byte, entry.Count)
	copy(result, e.Data[offset:offset+int(entry.Count)])
	return result
}

func (e *ValueExtractor) GetUserComment(entry IFDEntry, entryOffset int) string {
	raw := e.GetByteArray(entry, entryOffset)
	if len(raw) <= 8 {
		return ""
	}
	// Skip the 8-byte character code prefix
	return strings.TrimRight(string(raw[8:]), "\x00")
}

func (e *ValueExtractor) GetVersion(entry IFDEntry, entryOffset int) string {
	if entry.Count != 4 || entryOffset+12 > len(e.Data) {
		return ""
	}
	raw := e.Data[entryOffset+8 : entryOffset+12]
	// Convert "0232" → "2.32"
	return fmt.Sprintf("%c.%c%c", raw[1], raw[2], raw[3])
}

func (e *ValueExtractor) GetCompositeImageCount(entry IFDEntry, entryOffset int) (uint16, uint16) {
	if entry.Count < 2 {
		return 0, 0
	}

	var offset int
	if entry.Count*2 <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.TiffStart + int(entry.ValueOffset)
	}

	if offset+4 > len(e.Data) {
		return 0, 0
	}

	sourceNum := e.Endian.Uint16(e.Data[offset : offset+2])
	usedNum := e.Endian.Uint16(e.Data[offset+2 : offset+4])

	return sourceNum, usedNum
}

func (e *ValueExtractor) GetUTF16LEString(entry IFDEntry, entryOffset int) string {
	var offset int
	if entry.Count*2 <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.TiffStart + int(entry.ValueOffset)
	}

	if offset < 0 || offset+int(entry.Count) > len(e.Data) {
		return ""
	}

	// Convert byte count to uint16 count
	charCount := int(entry.Count) / 2
	if charCount == 0 {
		return ""
	}

	// Read UTF-16LE encoded data
	utf16Data := make([]uint16, charCount)
	for i := 0; i < charCount; i++ {
		if offset+i*2+2 > len(e.Data) {
			break
		}
		utf16Data[i] = binary.LittleEndian.Uint16(e.Data[offset+i*2 : offset+i*2+2])
	}

	// Decode UTF-16 to UTF-8 string
	runes := utf16.Decode(utf16Data)
	result := string(runes)

	// Trim null terminators and any trailing whitespace
	result = strings.TrimRight(result, "\x00")
	return strings.TrimSpace(result)
}

func (e *ValueExtractor) DecodeXMPMeta(inXml []byte) XmpMeta {
	var xmp XmpMeta

	err := xml.Unmarshal(inXml, &xmp)
	if err != nil {
		slog.Error("Cannot unmarshal XMP", "error", err)
		return xmp
	}

	// First, extract HdrPlusMakernote attribute BEFORE XML parsing
	// to avoid XML entity decoding corrupting the base64 data
	xmp.RDF.Description.HdrPlusMakerNote = extractRawXMLAttribute(string(inXml), "HdrPlusMakernote")

	return xmp
}

// extractRawXMLAttribute extracts an attribute value without XML entity decoding
func extractRawXMLAttribute(xmlStr, attrName string) string {
	// Look for the attribute pattern with optional namespace prefix
	// Pattern: (namespace:)?attrName="value"
	pattern := `[a-zA-Z]*:?` + attrName + `="([^"]*)"`

	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(xmlStr)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}
