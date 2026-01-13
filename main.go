package main

import (
	"log/slog"
	"os"

	"github.com/ZanyLeonic/exif-reader/exif"
)

func main() {
	if len(os.Args) < 2 {
		slog.Error("Usage: exif-reader <image-file>")
		os.Exit(1)
	}

	filename := os.Args[1]
	data, err := os.ReadFile(filename)
	if err != nil {
		slog.Error("Error reading file", "error", err, "file", filename)
		os.Exit(1)
	}

	metadata, err := exif.ExtractExifData(data)
	if metadata != nil && err != nil {
		slog.Warn("Extracted metadata with warnings", "warning", err)
	} else if err != nil {
		slog.Error("Error extracting exif metadata", "error", err)
		os.Exit(1)
	}

	slog.Info("Metadata search successful", "metadata", metadata)

	os.Exit(0)
}
