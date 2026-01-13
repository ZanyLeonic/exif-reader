package helpers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Tag uint16

type MakerNoteData struct {
	Raw          []byte                 `json:"raw"`
	Manufacturer string                 `json:"manufacturer"`
	Parsed       map[string]interface{} `json:"parsed"`
}

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
	DestinationBearing   string    `json:"destinationBearing"`
	DestinationDistance  string    `json:"destinationDistance"`
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
	CompositeImageCount string  `json:"compositeImageCount"`
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
	ImageUniqueID    string        `json:"imageUniqueID"`
	MakerNote        MakerNoteData `json:"makerNote"`
	RelatedSoundFile string        `json:"relatedSoundFile"`
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
	Tag         Tag
	DataType    uint16
	Count       uint32
	ValueOffset uint32
}

func DetermineEndianess(data []byte, offset int) (binary.ByteOrder, error) {
	if data[offset+10] == 0x49 && data[offset+11] == 0x49 {
		return binary.LittleEndian, nil
	} else if data[offset+10] == 0x4D && data[offset+11] == 0x4D {
		return binary.BigEndian, nil
	}
	return nil, errors.New("unsupported byte order")
}

func ParseIFDEntry(data []byte, offset int, endian binary.ByteOrder) IFDEntry {
	return IFDEntry{
		Tag:         Tag(endian.Uint16(data[offset : offset+2])),
		DataType:    endian.Uint16(data[offset+2 : offset+4]),
		Count:       endian.Uint32(data[offset+4 : offset+8]),
		ValueOffset: endian.Uint32(data[offset+8 : offset+12]),
	}
}

func ParseOrientationValue(raw uint16) string {
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

func ParseExposureProgram(raw uint16) string {
	switch raw {
	case 0:
		return "Not Defined"
	case 1:
		return "Manual"
	case 2:
		return "Program AE"
	case 3:
		return "Aperture-priority AE"
	case 4:
		return "Shutter speed priority AE"
	case 5:
		return "Creative (Slow speed)"
	case 6:
		return "Action (High speed)"
	case 7:
		return "Portrait"
	case 8:
		return "Landscape"
	case 9:
		return "Bulb"
	default:
		return "Unknown"
	}
}

func ParseComponentsConfiguration(components []uint8) string {
	var names []string
	for _, comp := range components {
		switch comp {
		case 0:
			names = append(names, "-")
		case 1:
			names = append(names, "Y")
		case 2:
			names = append(names, "Cb")
		case 3:
			names = append(names, "Cr")
		case 4:
			names = append(names, "R")
		case 5:
			names = append(names, "G")
		case 6:
			names = append(names, "B")
		default:
			names = append(names, "?")
		}
	}
	return strings.Join(names, "")
}

func ParseMeteringMode(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Average"
	case 2:
		return "Center-weighted average"
	case 3:
		return "Spot"
	case 4:
		return "Multi-spot"
	case 5:
		return "Multi-segment"
	case 6:
		return "Partial"
	case 255:
		return "Other"
	default:
		return "Not Defined"
	}
}

func ParseLightSource(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Daylight"
	case 2:
		return "Fluorescent"
	case 3:
		return "Tungsten (Incandescent)"
	case 4:
		return "Flash"
	case 9:
		return "Fine Weather"
	case 10:
		return "Cloudy"
	case 11:
		return "Shade"
	case 12:
		return "Daylight Fluorescent"
	case 13:
		return "Day White Fluorescent"
	case 14:
		return "Cool White Fluorescent"
	case 15:
		return "White Fluorescent"
	case 16:
		return "Warm White Fluorescent"
	case 17:
		return "Standard Light A"
	case 18:
		return "Standard Light B"
	case 19:
		return "Standard Light C"
	case 20:
		return "D55"
	case 21:
		return "D65"
	case 22:
		return "D75"
	case 23:
		return "D50"
	case 24:
		return "ISO Studio Tungsten"
	case 255:
		return "Other"
	default:
		return "Not Defined"
	}
}

func ParseColourSpace(raw uint16) string {
	switch raw {
	case 0x1:
		return "sRGB"
	case 0x2:
		return "Adobe RGB"
	case 0xfffd:
		return "Wide Gamut RGB"
	case 0xfffe:
		return "ICC Profile"
	case 0xffff:
		return "Uncalibrated"
	default:
		return "None"
	}
}

func ParseFlashValue(raw uint16) string {
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

func FormatExposureTime(num, den uint32) string {
	if den == 0 {
		return "Invalid"
	}

	// For exposure >= 1 second, show a decimal
	if num >= den {
		seconds := float64(num) / float64(den)
		if seconds == float64(int(seconds)) {
			return fmt.Sprintf("%ds", int(seconds))
		}
		return fmt.Sprintf("%.1fs", seconds)
	}

	reciprocal := int((float64(den) / float64(num)) + 0.5)

	return fmt.Sprintf("1/%d", reciprocal)
}

func ParseFileSource(raw uint8) string {
	switch raw {
	case 0x1:
		return "Film Scanner (Transparent Scanner)"
	case 0x2:
		return "Film Scanner (Relection Print Scanner)"
	case 0x3:
		return "Digital Camera"
	default:
		return "Unknown"
	}
}

func ParseSceneType(raw uint16) string {
	switch raw {
	case 0:
		return "Standard"
	case 1:
		return "Landscape"
	case 2:
		return "Portrait"
	case 3:
		return "Night"
	case 4:
		return "Other"
	default:
		return "Unknown"
	}
}

// ParseProcessing for Contrast, Saturation, and Sharpness
func ParseProcessing(raw uint16) string {
	switch raw {
	case 0:
		return "Normal"
	case 1:
		return "Low"
	case 2:
		return "High"
	default:
		return "Unknown or not set"
	}
}

func ParseSubjectDistanceRange(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Macro"
	case 2:
		return "Close"
	case 3:
		return "Distant"
	default:
		return "Not defined"
	}
}

func ParseCompositeImage(raw uint16) string {
	switch raw {
	case 0:
		return "Unknown"
	case 1:
		return "Not a Composite Image"
	case 2:
		return "General Composite Image"
	case 3:
		return "Composite Image Captured While Shooting"
	default:
		return "Not defined"
	}
}
