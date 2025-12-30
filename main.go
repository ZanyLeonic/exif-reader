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

func extractExifData(data []byte) (*PhotoExifEvidence, error) {
	if !(len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8) {
		return nil, errors.New("file is not a JPEG")
	}

	var endian binary.ByteOrder
	for i := range data {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			slog.Info("Found APP1 segment")

			if data[i+10] == 0x49 && data[i+11] == 0x49 {
				endian = binary.LittleEndian
			} else if data[i+10] == 0x4D && data[i+11] == 0x4D {
				endian = binary.BigEndian
			} else {
				return nil, errors.New("unsupported byte order")
			}
			slog.Info("detected photo endianess from TIFF header", "endian", endian)

			tiffStart := i + 10
			ifdOffset := endian.Uint32(data[tiffStart+4 : tiffStart+8])
			firstIfdIndex := tiffStart + int(ifdOffset)

			slog.Info("First IFD Index", "first", firstIfdIndex)

			entryCount := endian.Uint16(data[firstIfdIndex : firstIfdIndex+2])
			slog.Info("IFD entry count", "count", entryCount)

			metadata := PhotoExifEvidence{}

			for j := 0; j < int(entryCount); j++ {
				entryOffset := firstIfdIndex + 2 + (j * 12)

				tag := EXIFTag(endian.Uint16(data[entryOffset : entryOffset+2]))
				dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
				count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
				valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

				slog.Info("IFD01 Entry",
					"tag", fmt.Sprintf("%#x", tag),
					"type", dataType,
					"count", count,
					"valueOffset", valueOffset)

				switch tag {
				case Make:
					offset := tiffStart + int(valueOffset)
					metadata.Make = getEXIFString(data, entryOffset, offset, int(count))
				case Model:
					offset := tiffStart + int(valueOffset)
					metadata.Model = getEXIFString(data, entryOffset, offset, int(count))
				case Software:
					offset := tiffStart + int(valueOffset)
					metadata.Software = getEXIFString(data, entryOffset, offset, int(count))
				case Orientation:
					orientationRaw := getEXIFuInt16(data, entryOffset, endian)
					metadata.Orientation = parseOrientationValue(orientationRaw)
				case EXIFSubIFD:
					exifSubIfdPointer := endian.Uint32(data[entryOffset+8 : entryOffset+12])
					exifIfdOffset := tiffStart + int(exifSubIfdPointer)
					extractExifSubIFD(data, exifIfdOffset, tiffStart, endian, &metadata)
				case GPSSubIFD:
					gpsSubIfdPointer := endian.Uint32(data[entryOffset+8 : entryOffset+12])
					gpsIfdOffset := tiffStart + int(gpsSubIfdPointer)
					extractGPSIFD(data, gpsIfdOffset, tiffStart, endian, &metadata)
				}
			}

			return &metadata, nil
		}
	}

	return nil, errors.New("unable to find EXIF Block")
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

func extractExifSubIFD(data []byte, exifIfdOffset int, tiffStart int, endian binary.ByteOrder, metadata *PhotoExifEvidence) {
	entryCount := endian.Uint16(data[exifIfdOffset : exifIfdOffset+2])
	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)

		tag := EXIFTag(endian.Uint16(data[entryOffset : entryOffset+2]))
		dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
		count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
		valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

		slog.Info("ExifIFD Entry",
			"tag", fmt.Sprintf("%#x", tag),
			"type", dataType,
			"count", count,
			"valueOffset", valueOffset)

		switch tag {
		case DateCaptured:
			offset := tiffStart + int(valueOffset)
			dateStr := getEXIFString(data, entryOffset, offset, int(count))
			captured, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found capture timestamp, but it is an invalid format!", "captureDate", dateStr, "error", err)
				continue
			}
			metadata.DateCaptured = captured
		case PixelXDimension:
			metadata.PixelXDimension = float64(getEXIFuInt16(data, entryOffset, endian))
		case PixelYDimension:
			metadata.PixelYDimension = float64(getEXIFuInt16(data, entryOffset, endian))
		case FlashFired:
			metadata.Flash = parseFlashValue(getEXIFuInt16(data, entryOffset, endian))
		}
	}
}

func extractGPSIFD(data []byte, exifIfdOffset int, tiffStart int, endian binary.ByteOrder, metadata *PhotoExifEvidence) {
	entryCount := endian.Uint16(data[exifIfdOffset : exifIfdOffset+2])

	var latRef string
	var longRef string

	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)

		tag := EXIFTag(endian.Uint16(data[entryOffset : entryOffset+2]))
		dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
		count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
		valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

		slog.Info("GPS IFD Entry",
			"tag", fmt.Sprintf("%#x", tag),
			"type", dataType,
			"count", count,
			"valueOffset", valueOffset)

		switch tag {
		case LatitudeDir:
			offset := tiffStart + int(valueOffset)
			latRef = getEXIFString(data, entryOffset, offset, int(count))
		case Latitude:
			metadata.GPSLatitude = getGPSCoord(data, tiffStart+int(valueOffset), endian)
		case LongitudeDir:
			offset := tiffStart + int(valueOffset)
			longRef = getEXIFString(data, entryOffset, offset, int(count))
		case Longitude:
			metadata.GPSLongitude = getGPSCoord(data, tiffStart+int(valueOffset), endian)
		}
	}

	if latRef == "S" {
		metadata.GPSLatitude *= -1
	}

	if longRef == "W" {
		metadata.GPSLongitude *= -1
	}
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
