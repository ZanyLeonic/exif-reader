package main

import (
	"errors"
	"log/slog"
	"os"
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

	for i := range data {
		if data[i] == 0xFF && data[i+1] == 0xE1 {
			slog.Info("Found APP1 segment")
			return &PhotoExifEvidence{}, nil
		}
	}

	return nil, errors.New("unable to find EXIF Block")
}
