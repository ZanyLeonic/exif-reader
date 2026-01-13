package exif

import (
	"fmt"
	"log/slog"
	"time"
)

// GPS Sub-IFD Tags
const (
	GPSVersionID     Tag = 0x0
	LatitudeRef      Tag = 0x1
	Latitude         Tag = 0x2
	LongitudeRef     Tag = 0x3
	Longitude        Tag = 0x4
	AltitudeRef      Tag = 0x5
	Altitude         Tag = 0x6
	Timestamp        Tag = 0x7
	SpeedRef         Tag = 0x0c
	Speed            Tag = 0x0d
	ImgDirectionRef  Tag = 0x10
	ImgDirection     Tag = 0x11
	MapDatum         Tag = 0x12
	DestLatitudeRef  Tag = 0x13
	DestLatitude     Tag = 0x14
	DestLongitudeRef Tag = 0x15
	DestLongitude    Tag = 0x16
	DestBearingRef   Tag = 0x17
	DestBearing      Tag = 0x18
	DestDistanceRef  Tag = 0x19
	DestDistance     Tag = 0x1a
	ProcessingMethod Tag = 0x1b
	Datestamp        Tag = 0x1d
	Differential     Tag = 0x1e
)

func ExtractGPSIFD(exifIfdOffset int, metadata *PhotoExifEvidence, helper *ValueExtractor) {
	entryCount := helper.Endian.Uint16(helper.Data[exifIfdOffset : exifIfdOffset+2])

	var hours, minutes int
	var seconds, speed, imgDir, destBearing, destDistance float64
	var latRef, longRef, imgDirRef, destLatRef, destLongRef, destBearingRef, destDistanceRef, dateStr, speedMetric string
	var hasLat, hasLong, hasDestLat, hasDestLong, hasTime, hasSpeed, hasImgDir, hasDestBearing, hasDestDistance, underSeaLevel bool

	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)
		entry := parseIFDEntry(helper.Data, entryOffset, helper.Endian)

		slog.Info("GPS IFD Entry",
			"tag", fmt.Sprintf("%#x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		switch entry.Tag {
		case GPSVersionID:
			rawVersion := helper.GetUint8Array(entryOffset, 4)
			metadata.GPS.Version = fmt.Sprintf("%d.%d.%d.%d", rawVersion[0], rawVersion[1], rawVersion[2], rawVersion[3])
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
		case AltitudeRef:
			ref := helper.GetUint8(entryOffset)
			if ref == 1 || ref == 3 {
				underSeaLevel = true
			}
		case Altitude:
			metadata.GPS.Altitude = helper.GetRational(entry, 0, false)
		case Timestamp:
			hours = int(helper.GetRational(entry, 0, false))
			minutes = int(helper.GetRational(entry, 8, false))
			seconds = helper.GetRational(entry, 16, false)
			hasTime = true
		case SpeedRef:
			speedMetric = helper.GetString(entry, entryOffset)
			hasSpeed = true
		case Speed:
			speed = helper.GetRational(entry, 0, false)
		case ImgDirectionRef:
			imgDirRef = helper.GetString(entry, entryOffset)
		case ImgDirection:
			imgDir = helper.GetRational(entry, 0, false)
			hasImgDir = true
		case MapDatum:
			metadata.GPS.MapDatum = helper.GetString(entry, entryOffset)
		case DestLatitudeRef:
			destLatRef = helper.GetString(entry, entryOffset)
		case DestLatitude:
			metadata.GPS.DestinationLatitude = helper.GetGPSCoord(entry)
			hasDestLat = true
		case DestLongitudeRef:
			destLongRef = helper.GetString(entry, entryOffset)
		case DestLongitude:
			metadata.GPS.DestinationLongitude = helper.GetGPSCoord(entry)
			hasDestLong = true
		case DestBearingRef:
			destBearingRef = helper.GetString(entry, entryOffset)
		case DestBearing:
			destBearing = helper.GetRational(entry, 0, false)
			hasDestBearing = true
		case DestDistanceRef:
			destDistanceRef = helper.GetString(entry, entryOffset)
		case DestDistance:
			destDistance = helper.GetRational(entry, 0, false)
			hasDestDistance = true
		case ProcessingMethod:
			metadata.GPS.ProcessingMethod = helper.GetString(entry, entryOffset)
		case Datestamp:
			dateStr = helper.GetString(entry, entryOffset)
		case Differential:
			rawVal := helper.GetUint16(entryOffset)
			if rawVal == 0x1 {
				metadata.GPS.Differential = "Differential Corrected"
			} else {
				metadata.GPS.Differential = "No Correction"
			}
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

	if underSeaLevel {
		metadata.GPS.Altitude *= -1
	}
	if metadata.GPS.Altitude < -11000 || metadata.GPS.Altitude > 9000 {
		slog.Warn("GPS altitude out of valid range", "alt", metadata.GPS.Altitude)
	}

	if hasSpeed && speedMetric != "" {
		metadata.GPS.Speed = fmt.Sprintf("%.2f%s", speed, speedMetric)
	}

	if hasImgDir && imgDirRef != "" {
		metadata.GPS.Direction = fmt.Sprintf("%f%s", imgDir, imgDirRef)
	}

	if hasDestLat && destLatRef == "S" {
		metadata.GPS.DestinationLatitude *= -1
	}
	if hasDestLat && (metadata.GPS.DestinationLatitude < -90 || metadata.GPS.DestinationLatitude > 90) {
		slog.Warn("GPS Destination latitude out of valid range", "lat", metadata.GPS.DestinationLatitude)
	}

	if hasDestLong && destLongRef == "W" {
		metadata.GPS.DestinationLongitude *= -1
	}
	if hasDestLong && (metadata.GPS.DestinationLongitude < -180 || metadata.GPS.DestinationLongitude > 180) {
		slog.Warn("GPS destination longitude out of valid range", "long", metadata.GPS.DestinationLongitude)
	}

	if hasDestBearing && destBearingRef != "" {
		metadata.GPS.DestinationBearing = fmt.Sprintf("%f%s", destBearing, destBearingRef)
	}
	if hasDestDistance && destDistanceRef != "" {
		metadata.GPS.DestinationDistance = fmt.Sprintf("%f%s", destDistance, destDistanceRef)
	}

	if hasTime && dateStr != "" {
		date, err := time.Parse("2006:01:02", dateStr)
		if err == nil {
			metadata.GPS.Timestamp = time.Date(
				date.Year(), date.Month(), date.Day(),
				hours, minutes, int(seconds),
				int((seconds-float64(int(seconds)))*1e9), time.UTC)
		}
	}
}
