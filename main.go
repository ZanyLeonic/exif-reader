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

				tag := endian.Uint16(data[entryOffset : entryOffset+2])
				dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
				count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
				valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

				slog.Info("IFD01 Entry",
					"tag", fmt.Sprintf("%#x", tag),
					"type", dataType,
					"count", count,
					"valueOffset", valueOffset)

				switch tag {
				case 0x010F:
					offset := tiffStart + int(valueOffset)
					metadata.Make = getEXIFString(data, entryOffset, offset, int(count))
				case 0x0110:
					offset := tiffStart + int(valueOffset)
					metadata.Model = getEXIFString(data, entryOffset, offset, int(count))
				case 0x0131:
					offset := tiffStart + int(valueOffset)
					metadata.Software = getEXIFString(data, entryOffset, offset, int(count))

				case 0x0112:
					orientationRaw := getEXIFuInt16(data, entryOffset, endian)
					switch orientationRaw {
					case 1:
						metadata.Orientation = "Horizontal"
					case 2:
						metadata.Orientation = "Mirror horizontal"
					case 3:
						metadata.Orientation = "Rotate 180"
					case 4:
						metadata.Orientation = "Mirror vertical"
					case 5:
						metadata.Orientation = "Mirror horizontal and rotate 270 CW"
					case 6:
						metadata.Orientation = "Rotate 90 CW"
					case 7:
						metadata.Orientation = "Mirror horizontal and rotate 90 CW"
					case 8:
						metadata.Orientation = "Rotate 270 CW"
					default:
						metadata.Orientation = "Unknown"
					}
				case 0x8769:
					exifSubIfdPointer := endian.Uint32(data[entryOffset+8 : entryOffset+12])
					exifIfdOffset := tiffStart + int(exifSubIfdPointer)
					extractExifSubIFD(data, exifIfdOffset, tiffStart, endian, &metadata)
				case 0x8825:
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

func extractExifSubIFD(data []byte, exifIfdOffset int, tiffStart int, endian binary.ByteOrder, metadata *PhotoExifEvidence) {
	entryCount := endian.Uint16(data[exifIfdOffset : exifIfdOffset+2])
	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)

		tag := endian.Uint16(data[entryOffset : entryOffset+2])
		dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
		count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
		valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

		slog.Info("ExifIFD Entry",
			"tag", fmt.Sprintf("%#x", tag),
			"type", dataType,
			"count", count,
			"valueOffset", valueOffset)

		switch tag {
		case 0x0002:
			metadata.GPSLatitude = getEXIFRational(data, tiffStart+int(valueOffset), endian)
		case 0x0004:
			metadata.GPSLongitude = getEXIFRational(data, tiffStart+int(valueOffset), endian)
		case 0x9003:
			offset := tiffStart + int(valueOffset)
			dateStr := getEXIFString(data, entryOffset, offset, int(count))
			captured, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found capture timestamp, but it is an invalid format!", "captureDate", dateStr, "error", err)
				continue
			}
			metadata.DateCaptured = captured
		case 0xa002:
			metadata.PixelXDimension = float64(getEXIFuInt16(data, entryOffset, endian))
		case 0xa003:
			metadata.PixelYDimension = float64(getEXIFuInt16(data, entryOffset, endian))
		case 0x9209:
			flashRaw := getEXIFuInt16(data, entryOffset, endian)
			if flashRaw&0x01 != 0 {
				metadata.Flash = "Flash Fired"
			} else {
				metadata.Flash = "Flash did not fire"
			}
		}
	}
}

func extractGPSIFD(data []byte, exifIfdOffset int, tiffStart int, endian binary.ByteOrder, metadata *PhotoExifEvidence) {
	entryCount := endian.Uint16(data[exifIfdOffset : exifIfdOffset+2])

	var latRef string
	var longRef string

	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)

		tag := endian.Uint16(data[entryOffset : entryOffset+2])
		dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
		count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
		valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

		slog.Info("GPS IFD Entry",
			"tag", fmt.Sprintf("%#x", tag),
			"type", dataType,
			"count", count,
			"valueOffset", valueOffset)

		switch tag {
		case 0x1:
			offset := tiffStart + int(valueOffset)
			latRef = getEXIFString(data, entryOffset, offset, int(count))
		case 0x2:
			metadata.GPSLatitude = getGPSCoord(data, tiffStart+int(valueOffset), endian)
		case 0x3:
			offset := tiffStart + int(valueOffset)
			longRef = getEXIFString(data, entryOffset, offset, int(count))
		case 0x4:
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
