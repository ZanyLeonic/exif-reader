package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

type EXIFTag uint16

// APP1 IFD Tags
const (
	ProcessingSoftware EXIFTag = 0x000b
	ImageWidth         EXIFTag = 0x0100
	ImageHeight        EXIFTag = 0x0101
	ImageDescription   EXIFTag = 0x010e
	Make               EXIFTag = 0x010f
	Model              EXIFTag = 0x0110
	Orientation        EXIFTag = 0x0112
	Software           EXIFTag = 0x0131
	ModifyDate         EXIFTag = 0x0132
	Artist             EXIFTag = 0x013b
	Copyright          EXIFTag = 0x8298
	EXIFSubIFD         EXIFTag = 0x8769
	GPSSubIFD          EXIFTag = 0x8825
	XPTitle            EXIFTag = 0x9c9b
	XPComment          EXIFTag = 0x9c9c
	XPAuthor           EXIFTag = 0x9c9d
	XPKeywords         EXIFTag = 0x9c9e
	XPSubject          EXIFTag = 0x9c9f
)

// EXIF Sub-IFD Tags
const (
	ExposureTime            EXIFTag = 0x829a
	FNumber                 EXIFTag = 0x829d
	ExposureProgram         EXIFTag = 0x8822
	ISO                     EXIFTag = 0x8827
	ExifVersion             EXIFTag = 0x9000
	DateCaptured            EXIFTag = 0x9003
	CreateDate              EXIFTag = 0x9004
	OffsetTime              EXIFTag = 0x9010
	OffsetTimeOriginal      EXIFTag = 0x9011
	OffsetTimeDigitized     EXIFTag = 0x9012
	ComponentsConfiguration EXIFTag = 0x9101
	MeteringMode            EXIFTag = 0x9207
	LightSource             EXIFTag = 0x9208
	FlashFired              EXIFTag = 0x9209
	FocalLength             EXIFTag = 0x920a
	MakerNote               EXIFTag = 0x927c
	UserComment             EXIFTag = 0x9286
	SubSecTime              EXIFTag = 0x9290
	SubSecTimeOriginal      EXIFTag = 0x9291
	SubSecTimeDigitized     EXIFTag = 0x9292
	FlashpixVersion         EXIFTag = 0xa000
	ColorSpace              EXIFTag = 0xa001
	PixelXDimension         EXIFTag = 0xa002
	PixelYDimension         EXIFTag = 0xa003
	RelatedSoundFile        EXIFTag = 0xa004
	FileSource              EXIFTag = 0xa300
	SceneType               EXIFTag = 0xa301
	WhiteBalance            EXIFTag = 0xa403
	DigitalZoomRatio        EXIFTag = 0xa404
	SceneCaptureType        EXIFTag = 0xa406
	Contrast                EXIFTag = 0xa408
	Saturation              EXIFTag = 0xa409
	Sharpness               EXIFTag = 0xa40a
	SubjectDistanceRange    EXIFTag = 0xa40c
	ImageUniqueID           EXIFTag = 0xa420
	BodySerialNumber        EXIFTag = 0xa431
	LensInfo                EXIFTag = 0xa432
	LensMake                EXIFTag = 0xa433
	LensModel               EXIFTag = 0xa434
	LensSerialNumber        EXIFTag = 0xa435
	ImageEditor             EXIFTag = 0xa438
	CameraFirmware          EXIFTag = 0xa439
	CompositeImage          EXIFTag = 0xa460
	CompositeImageCount     EXIFTag = 0xa461
	SerialNumber            EXIFTag = 0xfde9
)

// GPS Sub-IFD Tags
const (
	GPSVersionID     EXIFTag = 0x0
	LatitudeRef      EXIFTag = 0x1
	Latitude         EXIFTag = 0x2
	LongitudeRef     EXIFTag = 0x3
	Longitude        EXIFTag = 0x4
	AltitudeRef      EXIFTag = 0x5
	Altitude         EXIFTag = 0x6
	Timestamp        EXIFTag = 0x7
	SpeedRef         EXIFTag = 0x0c
	Speed            EXIFTag = 0x0d
	ImgDirectionRef  EXIFTag = 0x10
	ImgDirection     EXIFTag = 0x11
	MapDatum         EXIFTag = 0x12
	DestLatitudeRef  EXIFTag = 0x13
	DestLatitude     EXIFTag = 0x14
	DestLongitudeRef EXIFTag = 0x15
	DestLongitude    EXIFTag = 0x16
	DestBearingRef   EXIFTag = 0x17
	DestBearing      EXIFTag = 0x18
	DestDistanceRef  EXIFTag = 0x19
	DestDistance     EXIFTag = 0x1a
	ProcessingMethod EXIFTag = 0x1b
	Datestamp        EXIFTag = 0x1d
	Differential     EXIFTag = 0x1e
)

type MakerNoteData struct {
	Raw          []byte                 `json:"raw"`
	Manufacturer string                 `json:"manufacturer"`
	Parsed       map[string]interface{} `json:"parsed"`
}

type GPSExif struct {
	Version              string    `json:"version"`
	Altitude             float64   `json:"altitude"`
	Latitude             float64   `json:"latitude"`
	Longitude            float64   `json:"longitude"`
	Timestamp            time.Time `json:"timestamp"`
	Speed                string    `json:"speed"`
	Direction            string    `json:"direction"`
	MapDatum             string    `json:"mapDatum"`
	DestinationLatitude  float64   `json:"destinationLatitude"`
	DestinationLongitude float64   `json:"destinationLongitude"`
	DestinationBearing   string    `json:"destinationBearing"`
	DestinationDistance  string    `json:"destinationDistance"`
	ProcessingMethod     string    `json:"processingMethod"`
	Differential         string    `json:"differential"`
}

// TemporalData Temporal evidence with full precision
type TemporalData struct {
	DateCaptured        time.Time `json:"dateCaptured"`
	CreateDate          time.Time `json:"createDate"`
	ModifyDate          time.Time `json:"modifyDate"`
	SubSecTime          string    `json:"subSecTime"`
	SubSecTimeOriginal  string    `json:"subSecTimeOriginal"`
	SubSecTimeDigitized string    `json:"subSecTimeDigitized"`
	OffsetTime          string    `json:"offsetTime"`
	OffsetTimeOriginal  string    `json:"offsetTimeOriginal"`
	OffsetTimeDigitized string    `json:"offsetTimeDigitized"`
}

// DeviceData Device identification data
type DeviceData struct {
	Make             string `json:"make"`
	Model            string `json:"model"`
	BodySerialNumber string `json:"bodySerialNumber"`
	SerialNumber     string `json:"serialNumber"`
	CameraFirmware   string `json:"cameraFirmware"`
	LensInfo         string `json:"lensInfo"`
	LensMake         string `json:"lensMake"`
	LensModel        string `json:"lensModel"`
	LensSerialNumber string `json:"lensSerialNumber"`
}

// ImageProperties Image dimensions and properties
type ImageProperties struct {
	Width            int           `json:"width"`
	Height           int           `json:"height"`
	PixelXDimension  float64       `json:"pixelXDimension"`
	PixelYDimension  float64       `json:"pixelYDimension"`
	Orientation      string        `json:"orientation"`
	ColorSpace       string        `json:"colorSpace"`
	ComponentsConfig string        `json:"componentsConfiguration"`
	FileSource       string        `json:"fileSource"`
	SceneType        string        `json:"sceneType"`
	ExifVersion      string        `json:"exifVersion"`
	FlashpixVersion  string        `json:"flashpixVersion"`
	MakersNote       MakerNoteData `json:"makersNote"`
}

// CameraSettings Camera settings used during capture
type CameraSettings struct {
	ExposureTime         string  `json:"exposureTime"`
	FNumber              float64 `json:"fNumber"`
	ExposureProgram      string  `json:"exposureProgram"`
	ISO                  int     `json:"iso"`
	FocalLength          float64 `json:"focalLength"`
	MeteringMode         string  `json:"meteringMode"`
	LightSource          string  `json:"lightSource"`
	FlashFired           string  `json:"flashFired"`
	WhiteBalance         string  `json:"whiteBalance"`
	SceneCaptureType     string  `json:"sceneCaptureType"`
	SubjectDistanceRange string  `json:"subjectDistanceRange"`
}

// ProcessingData Post-processing and manipulation indicators
type ProcessingData struct {
	Software            string  `json:"software"`
	ProcessingSoftware  string  `json:"processingSoftware"`
	ImageEditor         string  `json:"imageEditor"`
	DigitalZoomRatio    float64 `json:"digitalZoomRatio"`
	Contrast            string  `json:"contrast"`
	Saturation          string  `json:"saturation"`
	Sharpness           string  `json:"sharpness"`
	CompositeImage      string  `json:"compositeImage"`
	CompositeImageCount string  `json:"compositeImageCount"`
}

// AuthorshipData Authorship and chain of custody
type AuthorshipData struct {
	Artist           string `json:"artist"`
	Copyright        string `json:"copyright"`
	ImageDescription string `json:"imageDescription"`
	XPTitle          string `json:"xpTitle"`
	XPComment        string `json:"xpComment"`
	XPAuthor         string `json:"xpAuthor"`
	XPKeywords       string `json:"xpKeywords"`
	XPSubject        string `json:"xpSubject"`
	UserComment      string `json:"userComment"`
}

// AuthenticityData Authenticity and integrity markers
type AuthenticityData struct {
	ImageUniqueID    string `json:"imageUniqueID"`
	MakerNote        string `json:"makerNote"`
	RelatedSoundFile string `json:"relatedSoundFile"`
}

type PhotoExifEvidence struct {
	Temporal     TemporalData     `json:"temporal"`
	GPS          GPSExif          `json:"gps"`
	Device       DeviceData       `json:"device"`
	Image        ImageProperties  `json:"image"`
	Camera       CameraSettings   `json:"camera"`
	Processing   ProcessingData   `json:"processing"`
	Authorship   AuthorshipData   `json:"authorship"`
	Authenticity AuthenticityData `json:"authenticity"`
}

type IFDEntry struct {
	Tag         EXIFTag
	DataType    uint16
	Count       uint32
	ValueOffset uint32
}

func parseIFDEntry(data []byte, offset int, endian binary.ByteOrder) IFDEntry {
	return IFDEntry{
		Tag:         EXIFTag(endian.Uint16(data[offset : offset+2])),
		DataType:    endian.Uint16(data[offset+2 : offset+4]),
		Count:       endian.Uint32(data[offset+4 : offset+8]),
		ValueOffset: endian.Uint32(data[offset+8 : offset+12]),
	}
}

type ExifValueExtractor struct {
	data      []byte
	tiffStart int
	endian    binary.ByteOrder
}

// getString extracts a string value from EXIF data
func (e *ExifValueExtractor) getString(entryOffset, offset, count int) string {
	if count <= 4 {
		return strings.TrimRight(string(e.data[entryOffset+8:entryOffset+8+count]), "\x00")
	}
	return strings.TrimRight(string(e.data[offset:offset+count]), "\x00")
}

// getRational extracts a rational value (numerator/denominator) from EXIF data
func (e *ExifValueExtractor) getRational(offset int) float64 {
	if offset < 0 || offset+8 > len(e.data) {
		return 0
	}

	numerator := e.endian.Uint32(e.data[offset : offset+4])
	denominator := e.endian.Uint32(e.data[offset+4 : offset+8])

	if denominator == 0 {
		return 0
	}

	return float64(numerator) / float64(denominator)
}

// getRationalParts extracts the raw numerator and denominator from EXIF data
func (e *ExifValueExtractor) getRationalParts(offset int) (uint32, uint32) {
	if offset < 0 || offset+8 > len(e.data) {
		return 0, 0
	}

	numerator := e.endian.Uint32(e.data[offset : offset+4])
	denominator := e.endian.Uint32(e.data[offset+4 : offset+8])

	return numerator, denominator
}

// getGPSCoordinate calculates GPS coordinates from degrees, minutes, seconds
func (e *ExifValueExtractor) getGPSCoordinate(offset int) float64 {
	degrees := e.getRational(offset)
	minutes := e.getRational(offset + 8)
	seconds := e.getRational(offset + 16)

	return degrees + (minutes / 60.0) + (seconds / 3600.0)
}

func (e *ExifValueExtractor) GetString(entry IFDEntry, entryOffset int) string {
	offset := e.tiffStart + int(entry.ValueOffset)
	return e.getString(entryOffset, offset, int(entry.Count))
}

func (e *ExifValueExtractor) GetUint32(entryOffset int) uint32 {
	if entryOffset < 0 || entryOffset+12 > len(e.data) {
		return 0
	}
	return e.endian.Uint32(e.data[entryOffset+8 : entryOffset+12])
}

func (e *ExifValueExtractor) GetUint16(entryOffset int) uint16 {
	if entryOffset < 0 || entryOffset+10 > len(e.data) {
		return 0
	}
	return e.endian.Uint16(e.data[entryOffset+8 : entryOffset+10])
}

func (e *ExifValueExtractor) GetUint8(entryOffset int) uint8 {
	if entryOffset < 0 || entryOffset+8 >= len(e.data) {
		return 0
	}
	return e.data[entryOffset+8]
}

func (e *ExifValueExtractor) GetUint8Array(entryOffset, numSlices int) []uint8 {
	val := make([]uint8, numSlices)
	copy(val, e.data[entryOffset+8:entryOffset+8+numSlices])
	return val
}

func (e *ExifValueExtractor) GetRational(entry IFDEntry, nestedOffset int) float64 {
	offset := e.tiffStart + int(entry.ValueOffset) + nestedOffset
	return e.getRational(offset)
}

func (e *ExifValueExtractor) GetRationalParts(entry IFDEntry, nestedOffset int) (uint32, uint32) {
	offset := e.tiffStart + int(entry.ValueOffset) + nestedOffset
	return e.getRationalParts(offset)
}

func (e *ExifValueExtractor) GetGPSCoord(entry IFDEntry) float64 {
	offset := e.tiffStart + int(entry.ValueOffset)
	return e.getGPSCoordinate(offset)
}

func (e *ExifValueExtractor) GetByteArray(entry IFDEntry, entryOffset int) []byte {
	var offset int
	if entry.Count <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.tiffStart + int(entry.ValueOffset)
	}

	if offset < 0 || offset+int(entry.Count) > len(e.data) {
		return nil
	}

	result := make([]byte, entry.Count)
	copy(result, e.data[offset:offset+int(entry.Count)])
	return result
}

func (e *ExifValueExtractor) GetUserComment(entry IFDEntry, entryOffset int) string {
	raw := e.GetByteArray(entry, entryOffset)
	if len(raw) <= 8 {
		return ""
	}
	// Skip the 8-byte character code prefix
	return strings.TrimRight(string(raw[8:]), "\x00")
}

func (e *ExifValueExtractor) GetVersion(entry IFDEntry, entryOffset int) string {
	if entry.Count != 4 || entryOffset+12 > len(e.data) {
		return ""
	}
	raw := e.data[entryOffset+8 : entryOffset+12]
	// Convert "0232" → "2.32"
	return fmt.Sprintf("%c.%c%c", raw[1], raw[2], raw[3])
}

func (e *ExifValueExtractor) GetCompositeImageCount(entry IFDEntry, entryOffset int) (uint16, uint16) {
	if entry.Count < 2 {
		return 0, 0
	}

	var offset int
	if entry.Count*2 <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.tiffStart + int(entry.ValueOffset)
	}

	if offset+4 > len(e.data) {
		return 0, 0
	}

	sourceNum := e.endian.Uint16(e.data[offset : offset+2])
	usedNum := e.endian.Uint16(e.data[offset+2 : offset+4])

	return sourceNum, usedNum
}

func (e *ExifValueExtractor) GetUTF16LEString(entry IFDEntry, entryOffset int) string {
	var offset int
	if entry.Count*2 <= 4 {
		offset = entryOffset + 8
	} else {
		offset = e.tiffStart + int(entry.ValueOffset)
	}

	if offset < 0 || offset+int(entry.Count) > len(e.data) {
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
		if offset+i*2+2 > len(e.data) {
			break
		}
		utf16Data[i] = binary.LittleEndian.Uint16(e.data[offset+i*2 : offset+i*2+2])
	}

	// Decode UTF-16 to UTF-8 string
	runes := utf16.Decode(utf16Data)
	result := string(runes)

	// Trim null terminators and any trailing whitespace
	result = strings.TrimRight(result, "\x00")
	return strings.TrimSpace(result)
}

func (e *ExifValueExtractor) DecodeMakerNote(entry IFDEntry) MakerNoteData {
	raw := e.GetByteArray(entry, e.tiffStart+int(entry.ValueOffset))
	slog.Info("MakerNote ID", "ID", string(raw[0:10]))

	return MakerNoteData{}
}

func (e *ExifValueExtractor) DecodeXMPMeta(inXml []byte) XmpMeta {
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

func parseOrientationValue(raw uint16) string {
	switch raw {
	case 1:
		return "Horizontal"
	case 2:
		return "Mirror horizontal"
	case 3:
		return "Rotate 180"
	case 4:
		return "Mirror vertical"
	case 5:
		return "Mirror horizontal and rotate 270 CW"
	case 6:
		return "Rotate 90 CW"
	case 7:
		return "Mirror horizontal and rotate 90 CW"
	case 8:
		return "Rotate 270 CW"
	default:
		return "Unknown"
	}
}

func parseExposureProgram(raw uint16) string {
	switch raw {
	case 0:
		return "Not Defined"
	case 1:
		return "Manual"
	case 2:
		return "Program AE"
	case 3:
		return "Aperture-priority AE"
	case 4:
		return "Shutter speed priority AE"
	case 5:
		return "Creative (Slow speed)"
	case 6:
		return "Action (High speed)"
	case 7:
		return "Portrait"
	case 8:
		return "Landscape"
	case 9:
		return "Bulb"
	default:
		return "Unknown"
	}
}

func parseComponentsConfiguration(components []uint8) string {
	var names []string
	for _, comp := range components {
		switch comp {
		case 0:
			names = append(names, "-")
		case 1:
			names = append(names, "Y")
		case 2:
			names = append(names, "Cb")
		case 3:
			names = append(names, "Cr")
		case 4:
			names = append(names, "R")
		case 5:
			names = append(names, "G")
		case 6:
			names = append(names, "B")
		default:
			names = append(names, "?")
		}
	}
	return strings.Join(names, "")
}

func parseMeteringMode(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Average"
	case 2:
		return "Center-weighted average"
	case 3:
		return "Spot"
	case 4:
		return "Multi-spot"
	case 5:
		return "Multi-segment"
	case 6:
		return "Partial"
	case 255:
		return "Other"
	default:
		return "Not Defined"
	}
}

func parseLightSource(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Daylight"
	case 2:
		return "Fluorescent"
	case 3:
		return "Tungsten (Incandescent)"
	case 4:
		return "Flash"
	case 9:
		return "Fine Weather"
	case 10:
		return "Cloudy"
	case 11:
		return "Shade"
	case 12:
		return "Daylight Fluorescent"
	case 13:
		return "Day White Fluorescent"
	case 14:
		return "Cool White Fluorescent"
	case 15:
		return "White Fluorescent"
	case 16:
		return "Warm White Fluorescent"
	case 17:
		return "Standard Light A"
	case 18:
		return "Standard Light B"
	case 19:
		return "Standard Light C"
	case 20:
		return "D55"
	case 21:
		return "D65"
	case 22:
		return "D75"
	case 23:
		return "D50"
	case 24:
		return "ISO Studio Tungsten"
	case 255:
		return "Other"
	default:
		return "Not Defined"
	}
}

func parseColourSpace(raw uint16) string {
	switch raw {
	case 0x1:
		return "sRGB"
	case 0x2:
		return "Adobe RGB"
	case 0xfffd:
		return "Wide Gamut RGB"
	case 0xfffe:
		return "ICC Profile"
	case 0xffff:
		return "Uncalibrated"
	default:
		return "None"
	}
}

func parseFlashValue(raw uint16) string {
	switch raw {
	case 0x0:
		return "No Flash"
	case 0x1:
		return "Fired"
	case 0x5:
		return "Fired, Return no detected"
	case 0x7:
		return "Fired, Return detected"
	case 0x8:
		return "On, Did not fire"
	case 0x9:
		return "On, Fired"
	case 0xd:
		return "On, Return not detected"
	case 0xf:
		return "On, Return detected"
	case 0x10:
		return "Off, Did not fire"
	case 0x14:
		return "Off, Did not fire, Return not detected"
	case 0x18:
		return "Auto, Did not fire"
	case 0x19:
		return "Auto, Fired"
	case 0x1d:
		return "Auto, Fired, Return not detected"
	case 0x1f:
		return "Auto, Fired, Return detected"
	case 0x20:
		return "No flash function"
	case 0x30:
		return "Off, No flash function"
	case 0x41:
		return "Fired, Red-eye reduction"
	case 0x45:
		return "Fired, Red-eye reduction, Return not detected"
	case 0x47:
		return "Fired, Red-eye reduction, Return detected"
	case 0x49:
		return "On, Red-eye reduction"
	case 0x4d:
		return "On, Red-eye reduction, Return not detected"
	case 0x4f:
		return "On, Red-eye reduction, Return detected"
	case 0x50:
		return "Off, Red-eye reduction"
	case 0x58:
		return "Auto, Did not fire, Red-eye reduction"
	case 0x59:
		return "Auto, Fired, Red-eye reduction"
	case 0x5d:
		return "Auto, Fired, Red-eye reduction, Return not detected"
	case 0x5f:
		return "Auto, Fired, Red-eye reduction, Return detected"
	default:
		return "Unknown"
	}
}

func formatExposureTime(num, den uint32) string {
	if den == 0 {
		return "Invalid"
	}

	// For exposure >= 1 second, show a decimal
	if num >= den {
		seconds := float64(num) / float64(den)
		if seconds == float64(int(seconds)) {
			return fmt.Sprintf("%ds", int(seconds))
		}
		return fmt.Sprintf("%.1fs", seconds)
	}

	reciprocal := int((float64(den) / float64(num)) + 0.5)

	return fmt.Sprintf("1/%d", reciprocal)
}

func parseFileSource(raw uint8) string {
	switch raw {
	case 0x1:
		return "Film Scanner (Transparent Scanner)"
	case 0x2:
		return "Film Scanner (Relection Print Scanner)"
	case 0x3:
		return "Digital Camera"
	default:
		return "Unknown"
	}
}

func parseSceneType(raw uint16) string {
	switch raw {
	case 0:
		return "Standard"
	case 1:
		return "Landscape"
	case 2:
		return "Portrait"
	case 3:
		return "Night"
	case 4:
		return "Other"
	default:
		return "Unknown"
	}
}

// parseProcessing for Contrast, Saturation, and Sharpness
func parseProcessing(raw uint16) string {
	switch raw {
	case 0:
		return "Normal"
	case 1:
		return "Low"
	case 2:
		return "High"
	default:
		return "Unknown or not set"
	}
}

func parseSubjectDistanceRange(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Macro"
	case 2:
		return "Close"
	case 3:
		return "Distant"
	default:
		return "Not defined"
	}
}

func parseCompositeImage(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Not a Composite Image"
	case 2:
		return "General Composite Image"
	case 3:
		return "Composite Image Captured While Shooting"
	default:
		return "Not defined"
	}
}

func sanitizeXMLString(s string) string {
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

func sanitizeBase64String(s string) string {
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

// decryptHDRPBytes implements the custom 64-bit XOR cipher used by Google, encrypting their MakerNote (ported from Exiftool)
func decryptHDRPBytes(data []byte) ([]byte, error) {
	// Pad to 8-byte alignment
	pad := (8 - (len(data) % 8)) & 0x07
	if pad > 0 {
		padded := make([]byte, len(data)+pad)
		copy(padded, data)
		data = padded
	}

	// Initial key
	// my $key = 0x2515606b4a7791cd;
	hi := uint32(0x2515606b)
	lo := uint32(0x4a7791cd)

	// Convert to 32-bit words for processing
	wordCount := len(data) / 4
	words := make([]uint32, wordCount)
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &words); err != nil {
		return nil, err
	}

	// Process each 64-bit (8-byte) block
	for i := 0; i < len(words); i += 2 {
		// Transform the key
		// $key ^= $key >> 12;
		lo ^= lo>>12 | (hi&0xfff)<<20
		hi ^= hi >> 12

		// $key ^= ($key << 25) & 0xffffffffffffffff;
		hi ^= (hi&0x7f)<<25 | lo>>7
		lo ^= (lo & 0x7f) << 25

		// $key ^= ($key >> 27) & 0xffffffffffffffff;
		lo ^= lo>>27 | (hi&0x7ffffff)<<5
		hi ^= hi >> 27

		// $key = ($key * 0x2545f4914f6cdd1d) & 0xffffffffffffffff;
		// Multiply using 32-bit arithmetic
		hi, lo = multiply64(hi, lo)

		// XOR the words with the key
		words[i] ^= lo
		words[i+1] ^= hi
	}

	// Convert back to bytes
	result := new(bytes.Buffer)
	if err := binary.Write(result, binary.LittleEndian, words); err != nil {
		return nil, err
	}

	// Remove padding from the END
	decrypted := result.Bytes()
	if pad > 0 {
		decrypted = decrypted[:len(decrypted)-pad]
	}

	return decrypted, nil
}

// multiply64 multiplies a 64-bit number (hi:lo) by 0x2545f4914f6cdd1d
// Returns the low 64 bits as (hi, lo) (ported from the exiftool project)
func multiply64(hi, lo uint32) (uint32, uint32) {
	// Pack as big-endian 32-bit, then unpack as big-endian 16-bit
	// Perl: my @a = unpack('n*', pack('N*', $hi, $lo));
	a := []uint32{
		(hi >> 16) & 0xffff, // high 16 bits of hi
		hi & 0xffff,         // low 16 bits of hi
		(lo >> 16) & 0xffff, // high 16 bits of lo
		lo & 0xffff,         // low 16 bits of lo
	}

	// Multiplier: 0x2545f4914f6cdd1d split into 16-bit parts
	b := []uint32{0x2545, 0xf491, 0x4f6c, 0xdd1d}

	// Multiply (school multiplication method)
	c := make([]uint64, 7)
	for j := 0; j < 4; j++ {
		for k := 0; k < 4; k++ {
			c[j+k] += uint64(a[j]) * uint64(b[k])
		}
	}

	// Propagate carries - match Perl's exact logic
	for j := 6; j >= 3; j-- {
		for c[j] > 0xffffffff {
			c[j-2]++
			c[j] -= 4294967296
		}
		c[j-1] += c[j] >> 16
		c[j] &= 0xffff
	}

	// Extract the low 64 bits
	// Perl: $hi = ($c[3] << 16) + $c[4];
	// Perl: $lo = ($c[5] << 16) + $c[6];
	newHi := uint32((c[3] << 16) + c[4])
	newLo := uint32((c[5] << 16) + c[6])

	return newHi, newLo
}

func readGzipContent(decrypted []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		// if the gunzip header is corrupted, attempt to raw inflate
		slog.Warn("gzip.NewReader failed, attempting raw inflate", "error", err)

		protoBytes, err := tryRawInflate(decrypted)
		if err != nil || len(protoBytes) == 0 {
			return nil, fmt.Errorf("both gzip and raw inflate failed: %w", err)
		}
		slog.Info("Successfully inflated using raw deflate", "size", len(protoBytes))

		return protoBytes, nil
	}
	defer reader.Close()

	// Read all available data, even if we hit EOF early
	protoBytes, err := io.ReadAll(reader)

	// Check if we got usable data despite errors
	if len(protoBytes) == 0 && err != nil {
		return nil, fmt.Errorf("failed to read gzip data: %w", err)
	}

	// Like ExifTool, treat EOF-related errors as warnings if we got data
	if err != nil && err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
		slog.Warn("gzip ReadAll encountered error, using partial data", "error", err, "bytesRead", len(protoBytes))
	} else if errors.Is(err, io.ErrUnexpectedEOF) {
		slog.Warn("gzip stream truncated (unexpected EOF), using available data", "bytesRead", len(protoBytes))
	}

	slog.Info("Decompressed protobuf data", "size", len(protoBytes))

	return protoBytes, nil
}

// tryRawInflate attempts to decompress data using raw DEFLATE format
// This is more permissive than gzip and can handle truncated streams
// Similar to how Compress::Raw::Zlib handles partial data in Perl
func tryRawInflate(data []byte) ([]byte, error) {
	// Try flate (raw DEFLATE) reader
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()

	// Read as much as possible, even if we hit EOF
	var result bytes.Buffer
	_, err := io.Copy(&result, reader)

	if err != nil && err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("raw inflate failed: %w", err)
	}

	// Return whatever we managed to decompress, even if incomplete
	return result.Bytes(), nil
}

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
