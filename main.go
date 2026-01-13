package main

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ZanyLeonic/exif-reader/pb"
	"google.golang.org/protobuf/proto"
)

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
		case ProcessingSoftware:
			metadata.Processing.ProcessingSoftware = helper.GetString(entry, entryOffset)
		case ImageWidth:
			metadata.Image.Width = int(helper.GetUint32(entryOffset))
		case ImageHeight:
			metadata.Image.Height = int(helper.GetUint32(entryOffset))
		case ImageDescription:
			metadata.Authorship.ImageDescription = helper.GetString(entry, entryOffset)
		case Make:
			metadata.Device.Make = helper.GetString(entry, entryOffset)
		case Model:
			metadata.Device.Model = helper.GetString(entry, entryOffset)
		case Orientation:
			metadata.Image.Orientation = parseOrientationValue(helper.GetUint16(entryOffset))
		case Software:
			metadata.Processing.Software = helper.GetString(entry, entryOffset)
		case ModifyDate:
			dateStr := helper.GetString(entry, entryOffset)
			parsed, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found ModifyDate in IFD01, however, cannot parse", "error", err)
				continue
			}
			metadata.Temporal.ModifyDate = parsed
		case Artist:
			metadata.Authorship.Artist = helper.GetString(entry, entryOffset)
		case Copyright:
			metadata.Authorship.Copyright = helper.GetString(entry, entryOffset)
		case EXIFSubIFD:
			exifSubIfdPointer := helper.GetUint32(entryOffset)
			exifIfdOffset := tiffStart + int(exifSubIfdPointer)
			extractExifSubIFD(exifIfdOffset, &metadata, &helper)
		case GPSSubIFD:
			gpsSubIfdPointer := helper.GetUint32(entryOffset)
			gpsIfdOffset := tiffStart + int(gpsSubIfdPointer)
			extractGPSIFD(gpsIfdOffset, &metadata, &helper)
		case XPTitle:
			metadata.Authorship.XPTitle = helper.GetUTF16LEString(entry, entryOffset)
		case XPComment:
			metadata.Authorship.XPComment = helper.GetUTF16LEString(entry, entryOffset)
		case XPAuthor:
			metadata.Authorship.XPAuthor = helper.GetUTF16LEString(entry, entryOffset)
		case XPKeywords:
			metadata.Authorship.XPKeywords = helper.GetUTF16LEString(entry, entryOffset)
		case XPSubject:
			metadata.Authorship.XPSubject = helper.GetUTF16LEString(entry, entryOffset)
		}
	}

	// Photo doesn't need extra processing for MakerNote
	if !strings.HasPrefix(metadata.Processing.Software, "HDR+") {
		return &metadata, nil
	}

	output, err := extractXMPData(data)
	if err != nil {
		slog.Error("Error extracting XMP metadata", "error", err)
		return &metadata, err
	}

	slog.Info("Found XMP data", "xmp", output)
	xmp := helper.DecodeXMPMeta([]byte(output))

	if xmp.RDF.Description.HasExtendedXMP == "" {
		return &metadata, nil
	}

	output, err = extractExtXMPData(data, xmp.RDF.Description.HasExtendedXMP)
	if err != nil {
		slog.Error("Error extracting XMP metadata", "error", err)
		return &metadata, err
	}

	extXmp := helper.DecodeXMPMeta([]byte(output))
	cleanBase64 := sanitizeBase64String(extXmp.RDF.Description.HdrPlusMakerNote)

	slog.Debug("Base64 lengths", "raw", len(extXmp.RDF.Description.HdrPlusMakerNote), "cleaned", len(cleanBase64))

	// Try standard encoding first
	encrypted, err := base64.StdEncoding.DecodeString(cleanBase64)
	if err != nil {
		slog.Warn("StdEncoding failed, trying RawStdEncoding", "error", err)
		// Try without padding
		encrypted, err = base64.RawStdEncoding.DecodeString(cleanBase64)
		if err != nil {
			slog.Error("Failed to decode HDRPlusMakerNote with both encodings", "error", err, "cleanedLength", len(cleanBase64))
			return &metadata, err
		}
	}

	if string(encrypted[0:4]) == "HDRP" {
		slog.Info("Found Google's HDRPlus header")

		decrypted, err := decryptHDRPBytes(encrypted[5:])
		if err != nil {
			return &metadata, err
		}

		protoBytes, err := readGzipContent(decrypted)
		if err != nil {
			return &metadata, err
		}

		// Try to parse the protobuf, even if truncated
		hdrPlusNotes := pb.GoogleHDRPlusMakerNote{}
		unmarshalOpts := proto.UnmarshalOptions{
			DiscardUnknown: true,
		}
		err = unmarshalOpts.Unmarshal(protoBytes, &hdrPlusNotes)
		if err != nil {
			// Like ExifTool, treat protobuf parse errors as warnings
			// The data is likely truncated, but we can still extract other EXIF data
			slog.Warn("Protobuf parsing incomplete - data may be truncated or corrupted", "error", err, "dataSize", len(protoBytes))
		} else {
			slog.Info("Successfully parsed HDR Plus MakerNotes", "hasData", hdrPlusNotes.ProtoReflect().IsValid())
		}

		// Populate the MakerNote data in the metadata struct
		metadata.Image.MakersNote = convertHDRPlusToMakerNote(&hdrPlusNotes, encrypted)
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
		case ExposureTime:
			num, den := helper.GetRationalParts(entry, 0)
			metadata.Camera.ExposureTime = formatExposureTime(num, den)
		case FNumber:
			metadata.Camera.FNumber = helper.GetRational(entry, 0, false)
		case ExposureProgram:
			metadata.Camera.ExposureProgram = parseExposureProgram(helper.GetUint16(entryOffset))
		case ISO:
			metadata.Camera.ISO = int(helper.GetUint16(entryOffset))
		case ExifVersion:
			metadata.Image.ExifVersion = helper.GetVersion(entry, entryOffset)
		case DateCaptured:
			dateStr := helper.GetString(entry, entryOffset)
			captured, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found capture timestamp, but it is an invalid format!", "captureDate", dateStr, "error", err)
				continue
			}
			metadata.Temporal.DateCaptured = captured
		case CreateDate:
			dateStr := helper.GetString(entry, entryOffset)
			captured, err := time.Parse("2006:01:02 15:04:05", dateStr)
			if err != nil {
				slog.Warn("Found createdate timestamp, but it is an invalid format!", "createDate", dateStr, "error", err)
				continue
			}
			metadata.Temporal.CreateDate = captured
		case OffsetTime:
			metadata.Temporal.OffsetTime = helper.GetString(entry, entryOffset)
		case OffsetTimeOriginal:
			metadata.Temporal.OffsetTimeOriginal = helper.GetString(entry, entryOffset)
		case OffsetTimeDigitized:
			metadata.Temporal.OffsetTimeDigitized = helper.GetString(entry, entryOffset)
		case ComponentsConfiguration:
			if entry.Count == 4 {
				components := helper.GetUint8Array(entryOffset, 4)
				metadata.Image.ComponentsConfig = parseComponentsConfiguration(components)
			}
		case MeteringMode:
			metadata.Camera.MeteringMode = parseMeteringMode(helper.GetUint16(entryOffset))
		case LightSource:
			metadata.Camera.LightSource = parseLightSource(helper.GetUint16(entryOffset))
		case FlashFired:
			metadata.Camera.FlashFired = parseFlashValue(helper.GetUint16(entryOffset))
		case FocalLength:
			metadata.Camera.FocalLength = helper.GetRational(entry, 0, false)
		case MakerNote:
			//makerIfdPointer := helper.GetUint32(entryOffset)
			//makerifdOffset := tiffStart + int(exifSubIfdPointer)
			metadata.Image.MakersNote = helper.DecodeMakerNote(entry)
		case UserComment:
			metadata.Authorship.UserComment = helper.GetUserComment(entry, entryOffset)
		case SubSecTime:
			metadata.Temporal.SubSecTime = helper.GetString(entry, entryOffset)
		case SubSecTimeOriginal:
			metadata.Temporal.SubSecTimeOriginal = helper.GetString(entry, entryOffset)
		case SubSecTimeDigitized:
			metadata.Temporal.SubSecTimeDigitized = helper.GetString(entry, entryOffset)
		case FlashpixVersion:
			metadata.Image.FlashpixVersion = helper.GetVersion(entry, entryOffset)
		case ColorSpace:
			metadata.Image.ColorSpace = parseColourSpace(helper.GetUint16(entryOffset))
		case PixelXDimension:
			metadata.Image.PixelXDimension = float64(helper.GetUint16(entryOffset))
		case PixelYDimension:
			metadata.Image.PixelYDimension = float64(helper.GetUint16(entryOffset))
		case RelatedSoundFile:
			metadata.Authenticity.RelatedSoundFile = helper.GetString(entry, entryOffset)
		case FileSource:
			metadata.Image.FileSource = parseFileSource(helper.GetUint8(entryOffset))
		case SceneType:
			rawVal := helper.GetUint8(entryOffset)
			if rawVal == 0x01 {
				metadata.Image.SceneType = "Directly Photographed"
			} else {
				metadata.Image.SceneType = "Unknown"
			}
		case WhiteBalance:
			metadata.Camera.WhiteBalance = helper.GetString(entry, entryOffset)
		case DigitalZoomRatio:
			metadata.Processing.DigitalZoomRatio = helper.GetRational(entry, 0, false)
		case SceneCaptureType:
			metadata.Camera.SceneCaptureType = parseSceneType(helper.GetUint16(entryOffset))
		case Contrast:
			metadata.Processing.Contrast = parseProcessing(helper.GetUint16(entryOffset))
		case Saturation:
			metadata.Processing.Saturation = parseProcessing(helper.GetUint16(entryOffset))
		case Sharpness:
			metadata.Processing.Sharpness = parseProcessing(helper.GetUint16(entryOffset))
		case SubjectDistanceRange:
			metadata.Camera.SubjectDistanceRange = parseSubjectDistanceRange(helper.GetUint16(entryOffset))
		case ImageUniqueID:
			metadata.Authenticity.ImageUniqueID = helper.GetString(entry, entryOffset)
		case BodySerialNumber:
			metadata.Device.BodySerialNumber = helper.GetString(entry, entryOffset)
		case LensInfo:
			metadata.Device.LensInfo = helper.GetString(entry, entryOffset)
		case LensMake:
			metadata.Device.LensMake = helper.GetString(entry, entryOffset)
		case LensModel:
			metadata.Device.LensModel = helper.GetString(entry, entryOffset)
		case LensSerialNumber:
			metadata.Device.LensSerialNumber = helper.GetString(entry, entryOffset)
		case ImageEditor:
			metadata.Processing.ImageEditor = helper.GetString(entry, entryOffset)
		case CameraFirmware:
			metadata.Device.CameraFirmware = helper.GetString(entry, entryOffset)
		case CompositeImage:
			metadata.Processing.CompositeImage = parseCompositeImage(helper.GetUint16(entryOffset))
		case CompositeImageCount:
			sourceNum, usedNum := helper.GetCompositeImageCount(entry, entryOffset)
			metadata.Processing.CompositeImageCount = fmt.Sprintf("%d/%d", sourceNum, usedNum)
		case SerialNumber:
			metadata.Device.SerialNumber = helper.GetString(entry, entryOffset)
		}
	}
}

func extractGPSIFD(exifIfdOffset int, metadata *PhotoExifEvidence, helper *ExifValueExtractor) {
	entryCount := helper.endian.Uint16(helper.data[exifIfdOffset : exifIfdOffset+2])

	var hours, minutes int
	var seconds, speed, imgDir, destBearing, destDistance float64
	var latRef, longRef, imgDirRef, destLatRef, destLongRef, destBearingRef, destDistanceRef, dateStr, speedMetric string
	var hasLat, hasLong, hasDestLat, hasDestLong, hasTime, hasSpeed, hasImgDir, hasDestBearing, hasDestDistance, underSeaLevel bool

	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)
		entry := parseIFDEntry(helper.data, entryOffset, helper.endian)

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

func extractXMPData(data []byte) (string, error) {
	xmpHeader := "http://ns.adobe.com/xap/1.0/\x00"
	for i := 0; i < len(data)-len(xmpHeader); i++ {
		start := 0
		if string(data[i:i+len(xmpHeader)]) == xmpHeader {
			start = i
		} else {
			continue
		}
		end := start
		for end < len(data)-11 {
			if string(data[end:end+12]) == "</x:xmpmeta>" {
				end += 12
				return strings.TrimLeft(string(data[start:end]), xmpHeader), nil
			}
			end++
		}
		return "", errors.New("XMP end tag not found")
	}
	return "", errors.New("XMP block not found")
}

func extractExtXMPData(data []byte, extId string) (string, error) {
	extHeader := fmt.Sprintf("http://ns.adobe.com/xmp/extension/\x00%s\x00", extId)
	for i := 0; i < len(data)-len(extHeader); i++ {
		start := 0
		if string(data[i:i+len(extHeader)]) == extHeader {
			start = i
		} else {
			continue
		}
		end := start
		for end < len(data)-11 {
			if string(data[end:end+12]) == "</x:xmpmeta>" {
				end += 12
				// Skip past the header by moving start position
				start = start + len(extHeader)
				xmlString := string(data[start:end])

				// Find the actual XML start
				tagStart := strings.Index(xmlString, "<x:xmpmeta")
				if tagStart != -1 {
					xmlString = xmlString[tagStart:]
				}

				var b strings.Builder
				for _, c := range xmlString {
					if c == '\uFFFD' {
						continue
					}
					b.WriteRune(c)
				}

				return sanitizeXMLString(b.String()), nil
			}
			end++
		}
		return "", errors.New("XMP end tag not found")
	}
	return "", errors.New("extended XMP data not found")
}

// convertHDRPlusToMakerNote converts a GoogleHDRPlusMakerNote protobuf to MakerNoteData
func convertHDRPlusToMakerNote(notes *pb.GoogleHDRPlusMakerNote, rawData []byte) MakerNoteData {
	parsed := make(map[string]interface{})

	if notes.GetImageInfo() != nil {
		imageInfo := notes.GetImageInfo()
		imageData := make(map[string]interface{})

		if imageInfo.GetImageName() != "" {
			imageData["imageName"] = imageInfo.GetImageName()
		}
		if len(imageInfo.GetImageData()) > 0 {
			imageData["imageDataSize"] = len(imageInfo.GetImageData())
		}

		if len(imageData) > 0 {
			parsed["imageInfo"] = imageData
		}
	}

	if notes.GetTimeLogText() != "" {
		timeLog := notes.GetTimeLogText()
		parsed["timeLogText"] = timeLog
	}

	if notes.GetSummaryText() != "" {
		summary := notes.GetSummaryText()
		parsed["summaryText"] = summary
	}

	if notes.GetFrameCount() != nil {
		frameInfo := notes.GetFrameCount()
		parsed["frameCount"] = frameInfo.GetFrameCount()
	}

	if notes.GetDeviceInfo() != nil {
		deviceInfo := notes.GetDeviceInfo()
		deviceData := make(map[string]interface{})

		if deviceInfo.GetDeviceMake() != "" {
			deviceData["make"] = deviceInfo.GetDeviceMake()
		}
		if deviceInfo.GetDeviceModel() != "" {
			deviceData["model"] = deviceInfo.GetDeviceModel()
		}
		if deviceInfo.GetDeviceCodename() != "" {
			deviceData["codename"] = deviceInfo.GetDeviceCodename()
		}
		if deviceInfo.GetDeviceHardwareRevision() != "" {
			deviceData["hardwareRevision"] = deviceInfo.GetDeviceHardwareRevision()
		}
		if deviceInfo.GetHDRPSoftware() != "" {
			deviceData["hdrpSoftware"] = deviceInfo.GetHDRPSoftware()
		}
		if deviceInfo.GetAndroidRelease() != "" {
			deviceData["androidRelease"] = deviceInfo.GetAndroidRelease()
		}
		if deviceInfo.GetSoftwareDate() != 0 {
			deviceData["softwareDate"] = deviceInfo.GetSoftwareDate()
		}
		if deviceInfo.GetApplication() != "" {
			deviceData["application"] = deviceInfo.GetApplication()
		}
		if deviceInfo.GetAppVersion() != "" {
			deviceData["appVersion"] = deviceInfo.GetAppVersion()
		}

		if deviceInfo.GetExposureTimeInfo() != nil {
			expInfo := deviceInfo.GetExposureTimeInfo()
			if expInfo.GetExposureTimeMin() != 0 {
				deviceData["exposureTimeMin"] = expInfo.GetExposureTimeMin()
			}
			if expInfo.GetExposureTimeMax() != 0 {
				deviceData["exposureTimeMax"] = expInfo.GetExposureTimeMax()
			}
		}

		if deviceInfo.GetIsoInfo() != nil {
			isoInfo := deviceInfo.GetIsoInfo()
			if isoInfo.GetIsoMin() != 0 {
				deviceData["isoMin"] = isoInfo.GetIsoMin()
			}
			if isoInfo.GetIsoMax() != 0 {
				deviceData["isoMax"] = isoInfo.GetIsoMax()
			}
		}

		if deviceInfo.GetMaxAnalogISO() != 0 {
			deviceData["maxAnalogIso"] = deviceInfo.GetMaxAnalogISO()
		}

		if len(deviceData) > 0 {
			parsed["deviceInfo"] = deviceData
		}
	}

	return MakerNoteData{
		Raw:          rawData,
		Manufacturer: "Google HDR+",
		Parsed:       parsed,
	}
}

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

	metadata, err := extractExifData(data)
	if metadata != nil && err != nil {
		slog.Warn("Extracted metadata with warnings", "warning", err)
	} else if err != nil {
		slog.Error("Error extracting exif metadata", "error", err)
		os.Exit(1)
	}

	slog.Info("Metadata search successful", "metadata", metadata)

	os.Exit(0)
}
