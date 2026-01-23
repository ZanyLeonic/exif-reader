package makernotes

import (
	"fmt"
	"log/slog"

	"github.com/ZanyLeonic/exif-reader/exif/helpers"
)

type AppleParser struct{}

func (p *AppleParser) Manufacturer() string {
	return "Apple"
}

func (p *AppleParser) Parse(e *helpers.ValueExtractor, entry helpers.IFDEntry) (*map[string]interface{}, error) {
	raw := e.GetByteArray(entry, e.TiffStart+int(entry.ValueOffset))

	// Minimum size check: 12-byte prefix + 2 endian + 2 magic + 4 offset + 2 count = 22 bytes
	if len(raw) < 22 {
		slog.Warn("Apple MakerNote data too short", "length", len(raw), "minimum", 22)
		return nil, fmt.Errorf("apple makernote too short length: %d, minimum: 22", len(raw))
	}

	if string(raw[0:12]) != "Apple iOS\x00\x00\x01" {
		slog.Warn("Apple MakerNote prefix mismatch", "prefix", fmt.Sprintf("%q", raw[0:12]))
		return nil, fmt.Errorf("incorrect prefix, expected Apple iOS, got: %s", fmt.Sprintf("%q", raw[0:12]))
	}

	// Apple MakerNote uses TIFF structure starting at byte 12
	// Parse the TIFF header using the standard function
	mnEndian, err := helpers.DetermineEndianess(raw, 2)
	if err != nil {
		slog.Warn("Error parsing Apple MakerNote TIFF header", "err", err)
		return nil, err
	}

	// Unlike standard TIFF, there's no magic number or IFD offset pointer.
	// The IFD just starts at byte 14.
	// Offsets within IFD entries are relative to byte 0 (start of MakerNote data)

	mnTiffStart := 0 // Offsets are relative to byte 0 of MakerNote
	mnFirstIfd := 14 // IFD starts at byte 14

	slog.Debug("Apple MakerNote IFD location",
		"ifdPosition", mnFirstIfd)

	if mnFirstIfd+2 > len(raw) {
		slog.Warn("IFD position out of bounds",
			"ifdPos", mnFirstIfd,
			"dataLen", len(raw))
		return nil, fmt.Errorf("IFD position out of bounds, pos: %d, dataLength: %d", mnFirstIfd, len(raw))
	}

	entryCount := mnEndian.Uint16(raw[mnFirstIfd : mnFirstIfd+2])

	// Validate we have enough data for all entries
	entriesStart := mnFirstIfd + 2
	totalEntriesSize := int(entryCount) * 12
	if entriesStart+totalEntriesSize > len(raw) {
		slog.Warn("Not enough data for all MakerNote entries",
			"entryCount", entryCount,
			"entriesStart", entriesStart,
			"requiredBytes", totalEntriesSize,
			"availableBytes", len(raw)-entriesStart)
		// Adjust entry count to what we can actually read
		entryCount = uint16((len(raw) - entriesStart) / 12)
		slog.Debug("Adjusted entry count to fit available data", "newCount", entryCount)
	}

	slog.Debug("Apple MakerNote IFD parsed",
		"entryCount", entryCount,
		"entriesStart", entriesStart)

	// Create helper for MakerNote parsing
	mnHelper := helpers.ValueExtractor{
		Data:      raw,
		TiffStart: mnTiffStart,
		Endian:    mnEndian,
	}

	parsed := make(map[string]interface{})

	// Parse all entries
	for j := 0; j < int(entryCount); j++ {
		entryOffset := entriesStart + (j * 12)

		entry := helpers.ParseIFDEntry(raw, entryOffset, mnEndian)

		slog.Debug("Apple MakerNote entry",
			"index", j,
			"tag", fmt.Sprintf("0x%04x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		// Parse known tags
		switch entry.Tag {
		case 0x0001:
			parsed["MakerNoteVersion"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x0003:
			// RunTime - todo: parse plist
		case 0x0004:
			parsed["AEStable"] = mnHelper.GetUint32(entryOffset) == 1
		case 0x0005:
			parsed["AETarget"] = mnHelper.GetUint32(entryOffset)
		case 0x0006:
			parsed["AEAverage"] = mnHelper.GetUint32(entryOffset)
		case 0x0007:
			parsed["AFStable"] = mnHelper.GetUint32(entryOffset) == 1
		case 0x0008:
			x := mnHelper.GetRational(entry, 0, true)
			y := mnHelper.GetRational(entry, 8, true)
			z := mnHelper.GetRational(entry, 16, true)
			parsed["AccelerationVector"] = []float64{x, y, z}
		case 0x000a:
			switch mnHelper.GetUint32(entryOffset) {
			case 3:
				parsed["HDRImageType"] = "HDR Image"
			case 4:
				parsed["HDRImageType"] = "Original Image"
			default:
				parsed["HDRImageType"] = "Unknown"
			}
		case 0x000b:
			parsed["BurstUUID"] = mnHelper.GetString(entry, entryOffset)
		case 0x000c:
			p1 := mnHelper.GetRational(entry, 0, true)
			p2 := mnHelper.GetRational(entry, 8, true)
			parsed["FocusDistanceRange"] = fmt.Sprintf("%.2f - %.2f m", p1, p2)
		case 0x000f:
			parsed["OISMode"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x0011:
			parsed["ContentIdentifier"] = mnHelper.GetString(entry, entryOffset)
		case 0x0014:
			switch int32(mnHelper.GetUint32(entryOffset)) {
			case 1:
				parsed["ImageCaptureType"] = "ProRAW"
			case 2:
				parsed["ImageCaptureType"] = "Portrait"
			case 10:
				parsed["ImageCaptureType"] = "Photo"
			case 11:
				parsed["ImageCaptureType"] = "Manual Focus"
			case 12:
				parsed["ImageCaptureType"] = "Scene"
			default:
				parsed["ImageCaptureType"] = "Unknown Value"
			}
		case 0x0015:
			parsed["ImageUniqueID"] = mnHelper.GetString(entry, entryOffset)
		case 0x0017:
			// todo - implement when runtime info is gathered
			continue
		case 0x0019:
			parsed["ImageProcessingFlags"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x001a:
			parsed["QualityHint"] = mnHelper.GetString(entry, entryOffset)
		case 0x001d:
			parsed["LuminanceNoiseAmplitude"] = mnHelper.GetRational(entry, 0, true)
		case 0x001f:
			parsed["PhotosAppFeatureFlags"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x0020:
			parsed["ImageCaptureRequestID"] = mnHelper.GetString(entry, entryOffset)
		case 0x0021:
			parsed["HDRHeadroom"] = mnHelper.GetRational(entry, 0, true)
		case 0x0023:
			values := mnHelper.GetUint32Array(entry, 2)
			if len(values) != 2 {
				continue
			}

			focusDistance := int32(values[0])
			packedValue := int32(values[1])

			highBits := (packedValue >> 28) & 0xf
			lowBits := packedValue & 0xfffffff

			parsed["AFPerformance"] = fmt.Sprintf("%d %d %d", focusDistance, highBits, lowBits)
		case 0x0025:
			parsed["SceneFlags"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x0027:
			parsed["SignalToNoiseRatio"] = mnHelper.GetRational(entry, 0, true)
		case 0x002b:
			parsed["PhotoIdentifier"] = mnHelper.GetString(entry, entryOffset)
		case 0x002d:
			parsed["ColorTemperature"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x002e:
			switch int32(mnHelper.GetUint32(entryOffset)) {
			case 0:
				parsed["CameraType"] = "Back Wide Angle"
			case 1:
				parsed["CameraType"] = "Back Normal"
			case 6:
				parsed["CameraType"] = "Front"
			default:
				parsed["CameraType"] = "Unknown"
			}
		case 0x002f:
			parsed["FocusPosition"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x0030:
			parsed["HDRGain"] = mnHelper.GetRational(entry, 0, true)
		case 0x0038:
			parsed["AFMeasuredDepth"] = int32(mnHelper.GetUint32(entryOffset))
		case 0x003d:
			parsed["AFConfidence"] = int32(mnHelper.GetUint32(entryOffset))
		}
	}

	return &parsed, nil
}
