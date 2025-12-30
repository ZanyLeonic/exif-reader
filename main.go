package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

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
			metadata.Device.Make = helper.GetString(entry, entryOffset)
		case Model:
			metadata.Device.Model = helper.GetString(entry, entryOffset)
		case Software:
			metadata.Processing.Software = helper.GetString(entry, entryOffset)
		case Orientation:
			metadata.Image.Orientation = parseOrientationValue(helper.GetUint16(entryOffset))
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
			metadata.Temporal.DateCaptured = captured
		case PixelXDimension:
			metadata.Image.PixelXDimension = float64(helper.GetUint16(entryOffset))
		case PixelYDimension:
			metadata.Image.PixelYDimension = float64(helper.GetUint16(entryOffset))
		case FlashFired:
			metadata.Camera.Flash = parseFlashValue(helper.GetUint16(entryOffset))
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
		case LatitudeRef:
			latRef = helper.GetString(entry, entryOffset)
		case Latitude:
			metadata.GPS.Latitude = helper.GetGPSCoord(entry)
			hasLat = true
		case LongitudeRef:
			longRef = helper.GetString(entry, entryOffset)
		case Longitude:
			metadata.GPS.Longitude = helper.GetGPSCoord(entry)
			hasLong = true
		}
	}

	if hasLat && latRef == "S" {
		metadata.GPS.Latitude *= -1
	}
	if hasLat && (metadata.GPS.Latitude < -90 || metadata.GPS.Latitude > 90) {
		slog.Warn("GPS latitude out of valid range", "lat", metadata.GPS.Latitude)
	}

	if hasLong && longRef == "W" {
		metadata.GPS.Longitude *= -1
	}
	if hasLong && (metadata.GPS.Longitude < -180 || metadata.GPS.Longitude > 180) {
		slog.Warn("GPS longitude out of valid range", "long", metadata.GPS.Longitude)
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
