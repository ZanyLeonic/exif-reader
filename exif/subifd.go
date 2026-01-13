package exif

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ZanyLeonic/exif-reader/exif/helpers"
	"github.com/ZanyLeonic/exif-reader/exif/makernotes"
)

// EXIF Sub-IFD Tags
const (
	ExposureTime            helpers.Tag = 0x829a
	FNumber                 helpers.Tag = 0x829d
	ExposureProgram         helpers.Tag = 0x8822
	ISO                     helpers.Tag = 0x8827
	ExifVersion             helpers.Tag = 0x9000
	DateCaptured            helpers.Tag = 0x9003
	CreateDate              helpers.Tag = 0x9004
	OffsetTime              helpers.Tag = 0x9010
	OffsetTimeOriginal      helpers.Tag = 0x9011
	OffsetTimeDigitized     helpers.Tag = 0x9012
	ComponentsConfiguration helpers.Tag = 0x9101
	MeteringMode            helpers.Tag = 0x9207
	LightSource             helpers.Tag = 0x9208
	FlashFired              helpers.Tag = 0x9209
	FocalLength             helpers.Tag = 0x920a
	MakerNote               helpers.Tag = 0x927c
	UserComment             helpers.Tag = 0x9286
	SubSecTime              helpers.Tag = 0x9290
	SubSecTimeOriginal      helpers.Tag = 0x9291
	SubSecTimeDigitized     helpers.Tag = 0x9292
	FlashpixVersion         helpers.Tag = 0xa000
	ColorSpace              helpers.Tag = 0xa001
	PixelXDimension         helpers.Tag = 0xa002
	PixelYDimension         helpers.Tag = 0xa003
	RelatedSoundFile        helpers.Tag = 0xa004
	FileSource              helpers.Tag = 0xa300
	SceneType               helpers.Tag = 0xa301
	WhiteBalance            helpers.Tag = 0xa403
	DigitalZoomRatio        helpers.Tag = 0xa404
	SceneCaptureType        helpers.Tag = 0xa406
	Contrast                helpers.Tag = 0xa408
	Saturation              helpers.Tag = 0xa409
	Sharpness               helpers.Tag = 0xa40a
	SubjectDistanceRange    helpers.Tag = 0xa40c
	ImageUniqueID           helpers.Tag = 0xa420
	BodySerialNumber        helpers.Tag = 0xa431
	LensInfo                helpers.Tag = 0xa432
	LensMake                helpers.Tag = 0xa433
	LensModel               helpers.Tag = 0xa434
	LensSerialNumber        helpers.Tag = 0xa435
	ImageEditor             helpers.Tag = 0xa438
	CameraFirmware          helpers.Tag = 0xa439
	CompositeImage          helpers.Tag = 0xa460
	CompositeImageCount     helpers.Tag = 0xa461
	SerialNumber            helpers.Tag = 0xfde9
)

func ExtractExifSubIFD(exifIfdOffset int, metadata *helpers.PhotoExifEvidence, helper *helpers.ValueExtractor) {
	entryCount := helper.Endian.Uint16(helper.Data[exifIfdOffset : exifIfdOffset+2])
	for j := 0; j < int(entryCount); j++ {
		entryOffset := exifIfdOffset + 2 + (j * 12)
		entry := helpers.ParseIFDEntry(helper.Data, entryOffset, helper.Endian)

		slog.Info("ExifIFD Entry",
			"tag", fmt.Sprintf("%#x", entry.Tag),
			"type", entry.DataType,
			"count", entry.Count,
			"valueOffset", entry.ValueOffset)

		switch entry.Tag {
		case ExposureTime:
			num, den := helper.GetRationalParts(entry, 0)
			metadata.Camera.ExposureTime = helpers.FormatExposureTime(num, den)
		case FNumber:
			metadata.Camera.FNumber = helper.GetRational(entry, 0, false)
		case ExposureProgram:
			metadata.Camera.ExposureProgram = helpers.ParseExposureProgram(helper.GetUint16(entryOffset))
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
				metadata.Image.ComponentsConfig = helpers.ParseComponentsConfiguration(components)
			}
		case MeteringMode:
			metadata.Camera.MeteringMode = helpers.ParseMeteringMode(helper.GetUint16(entryOffset))
		case LightSource:
			metadata.Camera.LightSource = helpers.ParseLightSource(helper.GetUint16(entryOffset))
		case FlashFired:
			metadata.Camera.FlashFired = helpers.ParseFlashValue(helper.GetUint16(entryOffset))
		case FocalLength:
			metadata.Camera.FocalLength = helper.GetRational(entry, 0, false)
		case MakerNote:
			manufacturer, parsed, err := makernotes.DetectAndParse(helper, entry)
			if err != nil {
				slog.Warn("Cannot parse MakerNote, skipping", "err", err)
				continue
			}
			metadata.Authenticity.MakerNote = helpers.MakerNoteData{
				Raw:          helper.GetByteArray(entry, helper.TiffStart+int(entry.ValueOffset)),
				Manufacturer: manufacturer,
				Parsed:       *parsed,
			}
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
			metadata.Image.ColorSpace = helpers.ParseColourSpace(helper.GetUint16(entryOffset))
		case PixelXDimension:
			metadata.Image.PixelXDimension = float64(helper.GetUint16(entryOffset))
		case PixelYDimension:
			metadata.Image.PixelYDimension = float64(helper.GetUint16(entryOffset))
		case RelatedSoundFile:
			metadata.Authenticity.RelatedSoundFile = helper.GetString(entry, entryOffset)
		case FileSource:
			metadata.Image.FileSource = helpers.ParseFileSource(helper.GetUint8(entryOffset))
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
			metadata.Camera.SceneCaptureType = helpers.ParseSceneType(helper.GetUint16(entryOffset))
		case Contrast:
			metadata.Processing.Contrast = helpers.ParseProcessing(helper.GetUint16(entryOffset))
		case Saturation:
			metadata.Processing.Saturation = helpers.ParseProcessing(helper.GetUint16(entryOffset))
		case Sharpness:
			metadata.Processing.Sharpness = helpers.ParseProcessing(helper.GetUint16(entryOffset))
		case SubjectDistanceRange:
			metadata.Camera.SubjectDistanceRange = helpers.ParseSubjectDistanceRange(helper.GetUint16(entryOffset))
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
			metadata.Processing.CompositeImage = helpers.ParseCompositeImage(helper.GetUint16(entryOffset))
		case CompositeImageCount:
			sourceNum, usedNum := helper.GetCompositeImageCount(entry, entryOffset)
			metadata.Processing.CompositeImageCount = fmt.Sprintf("%d/%d", sourceNum, usedNum)
		case SerialNumber:
			metadata.Device.SerialNumber = helper.GetString(entry, entryOffset)
		}
	}
}
