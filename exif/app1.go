package exif

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ZanyLeonic/exif-reader/pb"
	"google.golang.org/protobuf/proto"
)

// APP1 IFD Tags
const (
	ProcessingSoftware Tag = 0x000b
	ImageWidth         Tag = 0x0100
	ImageHeight        Tag = 0x0101
	ImageDescription   Tag = 0x010e
	Make               Tag = 0x010f
	Model              Tag = 0x0110
	Orientation        Tag = 0x0112
	Software           Tag = 0x0131
	ModifyDate         Tag = 0x0132
	Artist             Tag = 0x013b
	Copyright          Tag = 0x8298
	EXIFSubIFD         Tag = 0x8769
	GPSSubIFD          Tag = 0x8825
	XPTitle            Tag = 0x9c9b
	XPComment          Tag = 0x9c9c
	XPAuthor           Tag = 0x9c9d
	XPKeywords         Tag = 0x9c9e
	XPSubject          Tag = 0x9c9f
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

func DetermineEndianess(data []byte, offset int) (binary.ByteOrder, error) {
	if data[offset+10] == 0x49 && data[offset+11] == 0x49 {
		return binary.LittleEndian, nil
	} else if data[offset+10] == 0x4D && data[offset+11] == 0x4D {
		return binary.BigEndian, nil
	}
	return nil, errors.New("unsupported byte order")
}

func ExtractExifData(data []byte) (*PhotoExifEvidence, error) {
	// Determine if we are working with a JPEG with EXIF data
	offset, err := findAPP1Segment(data)
	if err != nil {
		return nil, err
	}

	endian, err := DetermineEndianess(data, offset)
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
	helper := ValueExtractor{
		Data:      data,
		TiffStart: tiffStart,
		Endian:    endian,
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
			ExtractExifSubIFD(exifIfdOffset, &metadata, &helper)
		case GPSSubIFD:
			gpsSubIfdPointer := helper.GetUint32(entryOffset)
			gpsIfdOffset := tiffStart + int(gpsSubIfdPointer)
			ExtractGPSIFD(gpsIfdOffset, &metadata, &helper)
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

	output, err := ExtractXMPData(data)
	if err != nil {
		slog.Error("Error extracting XMP metadata", "error", err)
		return &metadata, err
	}

	slog.Info("Found XMP data", "xmp", output)
	xmp := helper.DecodeXMPMeta([]byte(output))

	if xmp.RDF.Description.HasExtendedXMP == "" {
		return &metadata, nil
	}

	output, err = ExtractExtXMPData(data, xmp.RDF.Description.HasExtendedXMP)
	if err != nil {
		slog.Error("Error extracting XMP metadata", "error", err)
		return &metadata, err
	}

	extXmp := helper.DecodeXMPMeta([]byte(output))
	cleanBase64 := SanitizeBase64String(extXmp.RDF.Description.HdrPlusMakerNote)

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

		decrypted, err := DecryptHDRPBytes(encrypted[5:])
		if err != nil {
			return &metadata, err
		}

		protoBytes, err := ReadGzipContent(decrypted)
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
		metadata.Image.MakersNote = ConvertHDRPlusToMakerNote(&hdrPlusNotes, encrypted)
	}

	return &metadata, nil
}
