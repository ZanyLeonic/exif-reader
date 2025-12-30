package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

type EXIFTag uint16

// APP1 IFD Tags
const (
	Make        EXIFTag = 0x010f
	Model       EXIFTag = 0x0110
	Orientation EXIFTag = 0x0112
	Software    EXIFTag = 0x0131
	EXIFSubIFD  EXIFTag = 0x8769
	GPSSubIFD   EXIFTag = 0x8825
)

// EXIF Sub-IFD Tags
const (
	DateCaptured    EXIFTag = 0x9003
	PixelXDimension EXIFTag = 0xa002
	PixelYDimension EXIFTag = 0xa003
	FlashFired      EXIFTag = 0x9209
)

// GPS Sub-IFD Tags
const (
	LatitudeDir  EXIFTag = 0x1
	Latitude     EXIFTag = 0x2
	LongitudeDir EXIFTag = 0x3
	Longitude    EXIFTag = 0x4
)

type PhotoExifEvidence struct {
	DateCaptured    time.Time `json:"dateCaptured"`
	GPSLatitude     float64   `json:"gpsLatitude"`
	GPSLongitude    float64   `json:"gpsLongitude"`
	Make            string    `json:"make"`
	Model           string    `json:"model"`
	PixelXDimension float64   `json:"pixelXDimension"`
	PixelYDimension float64   `json:"pixelYDimension"`
	Software        string    `json:"software"`
	Orientation     string    `json:"orientation"`
	Flash           string    `json:"flash"`
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

func (e *ExifValueExtractor) GetString(entry IFDEntry, entryOffset int) string {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getEXIFString(e.data, entryOffset, offset, int(entry.Count))
}

func (e *ExifValueExtractor) GetUint16(entryOffset int) uint16 {
	return getEXIFuInt16(e.data, entryOffset, e.endian)
}

func (e *ExifValueExtractor) GetRational(entry IFDEntry) float64 {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getEXIFRational(e.data, offset, e.endian)
}

func (e *ExifValueExtractor) GetGPSCoord(entry IFDEntry) float64 {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getGPSCoord(e.data, offset, e.endian)
}

func getEXIFString(data []byte, entryOffset, offset, count int) string {
	if count <= 4 {
		return strings.TrimRight(string(data[entryOffset+8:entryOffset+8+count]), "\x00")
	}
	return strings.TrimRight(string(data[offset:offset+count]), "\x00")
}

func getEXIFRational(data []byte, offset int, endian binary.ByteOrder) float64 {
	numerator := endian.Uint32(data[offset : offset+4])
	denominator := endian.Uint32(data[offset+4 : offset+8])

	if denominator == 0 {
		return 0
	}

	return float64(numerator) / float64(denominator)
}

func getEXIFuInt16(data []byte, offset int, endian binary.ByteOrder) uint16 {
	return endian.Uint16(data[offset+8 : offset+10])
}

func getGPSCoord(data []byte, offset int, endian binary.ByteOrder) float64 {
	degrees := getEXIFRational(data, offset, endian)
	minutes := getEXIFRational(data, offset+8, endian)
	seconds := getEXIFRational(data, offset+16, endian)

	return degrees + (minutes / 60.0) + (seconds / 3600.0)
}
func findAPP1Segment(data []byte) (int, error) {
	// does the file have the JPEG Magic Number
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, errors.New("file is not a JPEG")
	}
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			slog.Info("Found APP1 segment")
			return i, nil
		}
	}

	return 0, errors.New("cannot find EXIF block")
}

func determineEndianess(data []byte, offset int) (binary.ByteOrder, error) {
	if data[offset+10] == 0x49 && data[offset+11] == 0x49 {
		return binary.LittleEndian, nil
	} else if data[offset+10] == 0x4D && data[offset+11] == 0x4D {
		return binary.BigEndian, nil
	}
	return nil, errors.New("unsupported byte order")
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

func extractExifData(data []byte) (*PhotoExifEvidence, error) {
	// Determine if we are working with a JPEG with EXIF data
	offset, err := findAPP1Segment(data)
	if err != nil {
		return nil, err
	}

	endian, err := determineEndianess(data, offset)
	if err != nil {
		return nil, err
	}

	slog.Info("detected photo endianess from TIFF header", "endian", endian)

	tiffStart := offset + 10
	ifdOffset := endian.Uint32(data[tiffStart+4 : tiffStart+8])
	firstIfdIndex := tiffStart + int(ifdOffset)

	slog.Info("First IFD Index", "first", firstIfdIndex)

	entryCount := endian.Uint16(data[firstIfdIndex : firstIfdIndex+2])
	slog.Info("IFD entry count", "count", entryCount)

	metadata := PhotoExifEvidence{}
	helper := ExifValueExtractor{
		data:      data,
		tiffStart: tiffStart,
		endian:    endian,
	}

	for j := 0; j < int(entryCount); j++ {
		entryOffset := firstIfdIndex + 2 + (j * 12)
		entry := parseIFDEntry(data, entryOffset, endian)

		slog.Info("IFD01 Entry",
			"tag", fmt.Sprintf("%#x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		switch entry.Tag {
		case Make:
			metadata.Make = helper.GetString(entry, entryOffset)
		case Model:
			metadata.Model = helper.GetString(entry, entryOffset)
		case Software:
			metadata.Software = helper.GetString(entry, entryOffset)
		case Orientation:
			metadata.Orientation = parseOrientationValue(helper.GetUint16(entryOffset))
		case EXIFSubIFD:
			exifSubIfdPointer := endian.Uint32(data[entryOffset+8 : entryOffset+12])
			exifIfdOffset := tiffStart + int(exifSubIfdPointer)
			extractExifSubIFD(exifIfdOffset, &metadata, &helper)
		case GPSSubIFD:
			gpsSubIfdPointer := endian.Uint32(data[entryOffset+8 : entryOffset+12])
			gpsIfdOffset := tiffStart + int(gpsSubIfdPointer)
			extractGPSIFD(gpsIfdOffset, &metadata, &helper)
		}
	}

	return &metadata, nil
}

func extractExifSubIFD(exifIfdOffset int, metadata *PhotoExifEvidence, helper *ExifValueExtractor) {
	entryCount := helper.endian.Uint16(helper.data[exifIfdOffset : exifIfdOffset+2])
	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)
		entry := parseIFDEntry(helper.data, entryOffset, helper.endian)

		slog.Info("ExifIFD Entry",
			"tag", fmt.Sprintf("%#x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		switch entry.Tag {
		case DateCaptured:
			dateStr := helper.GetString(entry, entryOffset)
			captured, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found capture timestamp, but it is an invalid format!", "captureDate", dateStr, "error", err)
				continue
			}
			metadata.DateCaptured = captured
		case PixelXDimension:
			metadata.PixelXDimension = float64(helper.GetUint16(entryOffset))
		case PixelYDimension:
			metadata.PixelYDimension = float64(helper.GetUint16(entryOffset))
		case FlashFired:
			metadata.Flash = parseFlashValue(helper.GetUint16(entryOffset))
		}
	}
}

func extractGPSIFD(exifIfdOffset int, metadata *PhotoExifEvidence, helper *ExifValueExtractor) {
	entryCount := helper.endian.Uint16(helper.data[exifIfdOffset : exifIfdOffset+2])

	var latRef, longRef string
	var hasLat, hasLong bool

	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)
		entry := parseIFDEntry(helper.data, entryOffset, helper.endian)

		slog.Info("GPS IFD Entry",
			"tag", fmt.Sprintf("%#x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		switch entry.Tag {
		case LatitudeDir:
			latRef = helper.GetString(entry, entryOffset)
		case Latitude:
			metadata.GPSLatitude = helper.GetGPSCoord(entry)
			hasLat = true
		case LongitudeDir:
			longRef = helper.GetString(entry, entryOffset)
		case Longitude:
			metadata.GPSLongitude = helper.GetGPSCoord(entry)
			hasLong = true
		}
	}

	if hasLat && latRef == "S" {
		metadata.GPSLatitude *= -1
	}
	if hasLat && (metadata.GPSLatitude < -90 || metadata.GPSLatitude > 90) {
		slog.Warn("GPS latitude out of valid range", "lat", metadata.GPSLatitude)
	}

	if hasLong && longRef == "W" {
		metadata.GPSLongitude *= -1
	}
	if hasLong && (metadata.GPSLongitude < -180 || metadata.GPSLongitude > 180) {
		slog.Warn("GPS longitude out of valid range", "long", metadata.GPSLongitude)
	}
}

func main() {
	data, err := os.ReadFile("example.jpg")
	if err != nil {
		slog.Error("Error reading file", "error", err)
		os.Exit(1)
	}

	metadata, err := extractExifData(data)
	if err != nil {
		slog.Error("Error extracting exif metadata", "error", err)
		os.Exit(1)
	}

	slog.Info("Metadata search successful", "metadata", metadata)

	os.Exit(0)
}
