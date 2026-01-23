package makernotes

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/ZanyLeonic/exif-reader/exif/helpers"
	"github.com/ZanyLeonic/exif-reader/pb"
)

// ConvertHDRPlusToMakerNote converts a GoogleHDRPlusMakerNote protobuf to MakerNoteData
func ConvertHDRPlusToMakerNote(notes *pb.GoogleHDRPlusMakerNote, rawData []byte) helpers.MakerNoteData {
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

	return helpers.MakerNoteData{
		Raw:          rawData,
		Manufacturer: "Google HDR+",
		Parsed:       parsed,
	}
}

// DecryptHDRPBytes implements the custom 64-bit XOR cipher used by Google, encrypting their MakerNote (ported from Exiftool)
func DecryptHDRPBytes(data []byte) ([]byte, error) {
	// Pad to 8-byte alignment
	pad := (8 - (len(data) % 8)) & 0x07
	if pad > 0 {
		padded := make([]byte, len(data)+pad)
		copy(padded, data)
		data = padded
	}

	// Initial key
	// my $key = 0x2515606b4a7791cd;
	hi := uint32(0x2515606b)
	lo := uint32(0x4a7791cd)

	// Convert to 32-bit words for processing
	wordCount := len(data) / 4
	words := make([]uint32, wordCount)
	buf := bytes.NewReader(data)
	if err := binary.Read(buf, binary.LittleEndian, &words); err != nil {
		return nil, err
	}

	// Process each 64-bit (8-byte) block
	for i := 0; i < len(words); i += 2 {
		// Transform the key
		// $key ^= $key >> 12;
		lo ^= lo>>12 | (hi&0xfff)<<20
		hi ^= hi >> 12

		// $key ^= ($key << 25) & 0xffffffffffffffff;
		hi ^= (hi&0x7f)<<25 | lo>>7
		lo ^= (lo & 0x7f) << 25

		// $key ^= ($key >> 27) & 0xffffffffffffffff;
		lo ^= lo>>27 | (hi&0x7ffffff)<<5
		hi ^= hi >> 27

		// $key = ($key * 0x2545f4914f6cdd1d) & 0xffffffffffffffff;
		// Multiply using 32-bit arithmetic
		hi, lo = multiply64(hi, lo)

		// XOR the words with the key
		words[i] ^= lo
		words[i+1] ^= hi
	}

	// Convert back to bytes
	result := new(bytes.Buffer)
	if err := binary.Write(result, binary.LittleEndian, words); err != nil {
		return nil, err
	}

	// Remove padding from the END
	decrypted := result.Bytes()
	if pad > 0 {
		decrypted = decrypted[:len(decrypted)-pad]
	}

	return decrypted, nil
}

// multiply64 multiplies a 64-bit number (hi:lo) by 0x2545f4914f6cdd1d
// Returns the low 64 bits as (hi, lo) (ported from the exiftool project)
func multiply64(hi, lo uint32) (uint32, uint32) {
	// Pack as big-endian 32-bit, then unpack as big-endian 16-bit
	// Perl: my @a = unpack('n*', pack('N*', $hi, $lo));
	a := []uint32{
		(hi >> 16) & 0xffff, // high 16 bits of hi
		hi & 0xffff,         // low 16 bits of hi
		(lo >> 16) & 0xffff, // high 16 bits of lo
		lo & 0xffff,         // low 16 bits of lo
	}

	// Multiplier: 0x2545f4914f6cdd1d split into 16-bit parts
	b := []uint32{0x2545, 0xf491, 0x4f6c, 0xdd1d}

	// Multiply (school multiplication method)
	c := make([]uint64, 7)
	for j := 0; j < 4; j++ {
		for k := 0; k < 4; k++ {
			c[j+k] += uint64(a[j]) * uint64(b[k])
		}
	}

	// Propagate carries - match Perl's exact logic
	for j := 6; j >= 3; j-- {
		for c[j] > 0xffffffff {
			c[j-2]++
			c[j] -= 4294967296
		}
		c[j-1] += c[j] >> 16
		c[j] &= 0xffff
	}

	// Extract the low 64 bits
	// Perl: $hi = ($c[3] << 16) + $c[4];
	// Perl: $lo = ($c[5] << 16) + $c[6];
	newHi := uint32((c[3] << 16) + c[4])
	newLo := uint32((c[5] << 16) + c[6])

	return newHi, newLo
}

func ReadGzipContent(decrypted []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		// if the gunzip header is corrupted, attempt to raw inflate
		slog.Warn("gzip.NewReader failed, attempting raw inflate", "error", err)

		protoBytes, err := tryRawInflate(decrypted)
		if err != nil || len(protoBytes) == 0 {
			return nil, fmt.Errorf("both gzip and raw inflate failed: %w", err)
		}
		slog.Debug("Successfully inflated using raw deflate", "size", len(protoBytes))

		return protoBytes, nil
	}
	defer reader.Close()

	// Read all available data, even if we hit EOF early
	protoBytes, err := io.ReadAll(reader)

	// Check if we got usable data despite errors
	if len(protoBytes) == 0 && err != nil {
		return nil, fmt.Errorf("failed to read gzip data: %w", err)
	}

	// Like ExifTool, treat EOF-related errors as warnings if we got data
	if err != nil && err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
		slog.Warn("gzip ReadAll encountered error, using partial data", "error", err, "bytesRead", len(protoBytes))
	} else if errors.Is(err, io.ErrUnexpectedEOF) {
		slog.Warn("gzip stream truncated (unexpected EOF), using available data", "bytesRead", len(protoBytes))
	}

	slog.Debug("Decompressed protobuf data", "size", len(protoBytes))

	return protoBytes, nil
}

// tryRawInflate attempts to decompress data using raw DEFLATE format
// This is more permissive than gzip and can handle truncated streams
// Similar to how Compress::Raw::Zlib handles partial data in Perl
func tryRawInflate(data []byte) ([]byte, error) {
	// Try flate (raw DEFLATE) reader
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()

	// Read as much as possible, even if we hit EOF
	var result bytes.Buffer
	_, err := io.Copy(&result, reader)

	if err != nil && err != io.EOF && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("raw inflate failed: %w", err)
	}

	// Return whatever we managed to decompress, even if incomplete
	return result.Bytes(), nil
}
