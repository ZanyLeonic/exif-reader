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

type EXIFTag uint16

// APP1 IFD Tags
const (
	ProcessingSoftware EXIFTag = 0x000b
	ImageWidth         EXIFTag = 0x0100
	ImageHeight        EXIFTag = 0x0101
	ImageDescription   EXIFTag = 0x010e
	Make               EXIFTag = 0x010f
	Model              EXIFTag = 0x0110
	Orientation        EXIFTag = 0x0112
	Software           EXIFTag = 0x0131
	ModifyDate         EXIFTag = 0x0132
	Artist             EXIFTag = 0x013b
	EXIFSubIFD         EXIFTag = 0x8769
	GPSSubIFD          EXIFTag = 0x8825
	XPTitle            EXIFTag = 0x9c9b
	XPComment          EXIFTag = 0x9c9c
	XPAuthor           EXIFTag = 0x9c9d
	XPKeywords         EXIFTag = 0x9c9e
	XPSubject          EXIFTag = 0x9c9f
)

// EXIF Sub-IFD Tags
const (
	Copyright               EXIFTag = 0x8298
	ExposureTime            EXIFTag = 0x829a
	FNumber                 EXIFTag = 0x829d
	ExposureProgram         EXIFTag = 0x8822
	ISO                     EXIFTag = 0x8827
	ExifVersion             EXIFTag = 0x9000
	DateCaptured            EXIFTag = 0x9003
	CreateDate              EXIFTag = 0x9004
	OffsetTime              EXIFTag = 0x9010
	OffsetTimeOriginal      EXIFTag = 0x9011
	OffsetTimeDigitized     EXIFTag = 0x9012
	ComponentsConfiguration EXIFTag = 0x9101
	MeteringMode            EXIFTag = 0x9207
	LightSource             EXIFTag = 0x9208
	FlashFired              EXIFTag = 0x9209
	FocalLength             EXIFTag = 0x920a
	MakerNote               EXIFTag = 0x927c
	UserComment             EXIFTag = 0x9286
	SubSecTime              EXIFTag = 0x9290
	SubSecTimeOriginal      EXIFTag = 0x9291
	SubSecTimeDigitized     EXIFTag = 0x9292
	FlashpixVersion         EXIFTag = 0xa000
	ColorSpace              EXIFTag = 0xa001
	PixelXDimension         EXIFTag = 0xa002
	PixelYDimension         EXIFTag = 0xa003
	RelatedSoundFile        EXIFTag = 0xa004
	FileSource              EXIFTag = 0xa300
	SceneType               EXIFTag = 0xa301
	WhiteBalance            EXIFTag = 0xa403
	DigitalZoomRatio        EXIFTag = 0xa404
	SceneCaptureType        EXIFTag = 0xa406
	Contrast                EXIFTag = 0xa408
	Saturation              EXIFTag = 0xa409
	Sharpness               EXIFTag = 0xa40a
	SubjectDistanceRange    EXIFTag = 0xa40c
	ImageUniqueID           EXIFTag = 0xa420
	BodySerialNumber        EXIFTag = 0xa431
	LensInfo                EXIFTag = 0xa432
	LensMake                EXIFTag = 0xa433
	LensModel               EXIFTag = 0xa434
	LensSerialNumber        EXIFTag = 0xa435
	ImageEditor             EXIFTag = 0xa438
	CameraFirmware          EXIFTag = 0xa439
	CompositeImage          EXIFTag = 0xa460
	CompositeImageCount     EXIFTag = 0xa461
	SerialNumber            EXIFTag = 0xfde9
)

// GPS Sub-IFD Tags
const (
	GPSVersionID     EXIFTag = 0x0
	LatitudeRef      EXIFTag = 0x1
	Latitude         EXIFTag = 0x2
	LongitudeRef     EXIFTag = 0x3
	Longitude        EXIFTag = 0x4
	AltitudeRef      EXIFTag = 0x5
	Altitude         EXIFTag = 0x6
	Timestamp        EXIFTag = 0x7
	SpeedRef         EXIFTag = 0x0c
	Speed            EXIFTag = 0x0d
	ImgDirectionRef  EXIFTag = 0x10
	ImgDirection     EXIFTag = 0x11
	MapDatum         EXIFTag = 0x12
	DestLatitudeRef  EXIFTag = 0x13
	DestLatitude     EXIFTag = 0x14
	DestLongitudeRef EXIFTag = 0x15
	DestLongitude    EXIFTag = 0x16
	DestBearingRef   EXIFTag = 0x17
	DestBearing      EXIFTag = 0x18
	DestDistanceRef  EXIFTag = 0x19
	DestDistance     EXIFTag = 0x1a
	ProcessingMethod EXIFTag = 0x1b
	Datestamp        EXIFTag = 0x1d
	Differential     EXIFTag = 0x1e
)

type GPSExif struct {
	Version              string    `json:"version"`
	Altitude             float64   `json:"altitude"`
	Latitude             float64   `json:"latitude"`
	Longitude            float64   `json:"longitude"`
	Timestamp            time.Time `json:"timestamp"`
	Speed                string    `json:"speed"`
	Direction            string    `json:"direction"`
	MapDatum             string    `json:"mapDatum"`
	DestinationLatitude  float64   `json:"destinationLatitude"`
	DestinationLongitude float64   `json:"destinationLongitude"`
	DestinationBearing   float64   `json:"destinationBearing"`
	DestinationDistance  float64   `json:"destinationDistance"`
	ProcessingMethod     string    `json:"processingMethod"`
	Differential         string    `json:"differential"`
}

// TemporalData Temporal evidence with full precision
type TemporalData struct {
	DateCaptured        time.Time `json:"dateCaptured"`
	CreateDate          time.Time `json:"createDate"`
	ModifyDate          time.Time `json:"modifyDate"`
	SubSecTime          string    `json:"subSecTime"`
	SubSecTimeOriginal  string    `json:"subSecTimeOriginal"`
	SubSecTimeDigitized string    `json:"subSecTimeDigitized"`
	OffsetTime          string    `json:"offsetTime"`
	OffsetTimeOriginal  string    `json:"offsetTimeOriginal"`
	OffsetTimeDigitized string    `json:"offsetTimeDigitized"`
}

// DeviceData Device identification data
type DeviceData struct {
	Make             string `json:"make"`
	Model            string `json:"model"`
	BodySerialNumber string `json:"bodySerialNumber"`
	SerialNumber     string `json:"serialNumber"`
	CameraFirmware   string `json:"cameraFirmware"`
	LensInfo         string `json:"lensInfo"`
	LensMake         string `json:"lensMake"`
	LensModel        string `json:"lensModel"`
	LensSerialNumber string `json:"lensSerialNumber"`
}

// ImageProperties Image dimensions and properties
type ImageProperties struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	PixelXDimension  float64 `json:"pixelXDimension"`
	PixelYDimension  float64 `json:"pixelYDimension"`
	Orientation      string  `json:"orientation"`
	ColorSpace       string  `json:"colorSpace"`
	ComponentsConfig string  `json:"componentsConfiguration"`
	FileSource       string  `json:"fileSource"`
	SceneType        string  `json:"sceneType"`
	ExifVersion      string  `json:"exifVersion"`
	FlashpixVersion  string  `json:"flashpixVersion"`
}

// CameraSettings Camera settings used during capture
type CameraSettings struct {
	ExposureTime         string  `json:"exposureTime"`
	FNumber              float64 `json:"fNumber"`
	ExposureProgram      string  `json:"exposureProgram"`
	ISO                  int     `json:"iso"`
	FocalLength          float64 `json:"focalLength"`
	MeteringMode         string  `json:"meteringMode"`
	LightSource          string  `json:"lightSource"`
	Flash                string  `json:"flash"`
	FlashFired           string  `json:"flashFired"`
	WhiteBalance         string  `json:"whiteBalance"`
	SceneCaptureType     string  `json:"sceneCaptureType"`
	SubjectDistanceRange string  `json:"subjectDistanceRange"`
}

// ProcessingData Post-processing and manipulation indicators
type ProcessingData struct {
	Software            string  `json:"software"`
	ProcessingSoftware  string  `json:"processingSoftware"`
	ImageEditor         string  `json:"imageEditor"`
	DigitalZoomRatio    float64 `json:"digitalZoomRatio"`
	Contrast            string  `json:"contrast"`
	Saturation          string  `json:"saturation"`
	Sharpness           string  `json:"sharpness"`
	CompositeImage      string  `json:"compositeImage"`
	CompositeImageCount int     `json:"compositeImageCount"`
}

// AuthorshipData Authorship and chain of custody
type AuthorshipData struct {
	Artist           string `json:"artist"`
	Copyright        string `json:"copyright"`
	ImageDescription string `json:"imageDescription"`
	XPTitle          string `json:"xpTitle"`
	XPComment        string `json:"xpComment"`
	XPAuthor         string `json:"xpAuthor"`
	XPKeywords       string `json:"xpKeywords"`
	XPSubject        string `json:"xpSubject"`
	UserComment      string `json:"userComment"`
}

// AuthenticityData Authenticity and integrity markers
type AuthenticityData struct {
	ImageUniqueID    string `json:"imageUniqueID"`
	MakerNote        string `json:"makerNote"`
	RelatedSoundFile string `json:"relatedSoundFile"`
}

type PhotoExifEvidence struct {
	Temporal     TemporalData     `json:"temporal"`
	GPS          GPSExif          `json:"gps"`
	Device       DeviceData       `json:"device"`
	Image        ImageProperties  `json:"image"`
	Camera       CameraSettings   `json:"camera"`
	Processing   ProcessingData   `json:"processing"`
	Authorship   AuthorshipData   `json:"authorship"`
	Authenticity AuthenticityData `json:"authenticity"`
}

type IFDEntry struct {
	Tag         EXIFTag
	DataType    uint16
	Count       uint32
	ValueOffset uint32
}

func parseIFDEntry(data []byte, offset int, endian binary.ByteOrder) IFDEntry {
	return IFDEntry{
		Tag:         EXIFTag(endian.Uint16(data[offset : offset+2])),
		DataType:    endian.Uint16(data[offset+2 : offset+4]),
		Count:       endian.Uint32(data[offset+4 : offset+8]),
		ValueOffset: endian.Uint32(data[offset+8 : offset+12]),
	}
}

type ExifValueExtractor struct {
	data      []byte
	tiffStart int
	endian    binary.ByteOrder
}

func (e *ExifValueExtractor) GetString(entry IFDEntry, entryOffset int) string {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getEXIFString(e.data, entryOffset, offset, int(entry.Count))
}

func (e *ExifValueExtractor) GetUint16(entryOffset int) uint16 {
	return getEXIFuInt16(e.data, entryOffset, e.endian)
}

func (e *ExifValueExtractor) GetRational(entry IFDEntry) float64 {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getEXIFRational(e.data, offset, e.endian)
}

func (e *ExifValueExtractor) GetGPSCoord(entry IFDEntry) float64 {
	offset := e.tiffStart + int(entry.ValueOffset)
	return getGPSCoord(e.data, offset, e.endian)
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
