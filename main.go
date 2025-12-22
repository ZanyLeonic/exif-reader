package main

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"
)

type PhotoExifEvidence struct {
	DateCaptured    time.Time `json:"dateCaptured"`
	GPSLatitude     float64   `json:"gpsLatitude"`
	GPSLongitude    float64   `json:"gpsLongitude"`
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

			for j := 0; j < int(entryCount); j++ {
				entryOffset := firstIfdIndex + 2 + (j * 12)

				tag := endian.Uint16(data[entryOffset : entryOffset+2])
				dataType := endian.Uint16(data[entryOffset+2 : entryOffset+4])
				count := endian.Uint32(data[entryOffset+4 : entryOffset+8])
				valueOffset := endian.Uint32(data[entryOffset+8 : entryOffset+12])

				slog.Info("IFD Entry",
					"tag", tag,
					"type", dataType,
					"count", count,
					"valueOffset", valueOffset)

				if tag == 0x010F {
					cameraMake := ""
					if count <= 4 {
						cameraMake = string(data[entryOffset+8 : entryOffset+8+int(count)])
					} else {
						offset := tiffStart + int(valueOffset)
						cameraMake = string(data[offset : offset+int(count)])
					}

					slog.Info("Found Make tag", "make", strings.TrimRight(cameraMake, "\x00"))
				} else if tag == 0x0110 {
					cameraModel := ""
					if count <= 4 {
						cameraModel = string(data[entryOffset+8 : entryOffset+8+int(count)])
					} else {
						offset := tiffStart + int(valueOffset)
						cameraModel = string(data[offset : offset+int(count)])
					}

					slog.Info("Found Model tag", "model", strings.TrimRight(cameraModel, "\x00"))
				}
			}

			return &PhotoExifEvidence{}, nil
		}
	}

	return nil, errors.New("unable to find EXIF Block")
}
