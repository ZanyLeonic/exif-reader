package makernotes

import (
	"errors"

	"github.com/ZanyLeonic/exif-reader/exif/helpers"
)

type Parser interface {
	Parse(e *helpers.ValueExtractor, entry helpers.IFDEntry) (*map[string]interface{}, error)
	Manufacturer() string
}

func DetectAndParse(e *helpers.ValueExtractor, entry helpers.IFDEntry) (string, *map[string]interface{}, error) {
	parsers := []Parser{
		&AppleParser{},
	}

	for _, parser := range parsers {
		if parsed, err := parser.Parse(e, entry); err == nil && parsed != nil {
			return parser.Manufacturer(), parsed, nil
		} else if err != nil {
			return parser.Manufacturer(), nil, err
		}
	}
	return "Unknown", nil, errors.New("cannot parse makernote, corrupted or unsupported")
}
