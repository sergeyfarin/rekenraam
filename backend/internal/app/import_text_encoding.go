package app

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/wlynxg/chardet"
	"github.com/wlynxg/chardet/lookup"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
)

const minimumAutomaticEncodingConfidence = 50

type decodedImportText struct {
	Bytes      []byte
	Encoding   string
	Source     string
	Confidence int
}

// decodeImportText converts a text import to canonical UTF-8. An explicit
// encoding is authoritative. Auto mode accepts Unicode deterministically and
// uses chardet only when its best legacy-encoding result clears the confidence
// floor; ambiguous financial text must be retried with an explicit selection.
func decodeImportText(raw []byte, requested string) (decodedImportText, error) {
	requested = normalizeImportEncoding(requested)
	if requested != "auto" {
		decoded, canonical, err := decodeNamedImportEncoding(raw, requested)
		if err != nil {
			return decodedImportText{}, err
		}
		return decodedImportText{Bytes: decoded, Encoding: canonical, Source: "selected", Confidence: 100}, nil
	}

	if bomEncoding := importTextBOMEncoding(raw); bomEncoding != "" {
		decoded, canonical, err := decodeNamedImportEncoding(raw, bomEncoding)
		if err != nil {
			return decodedImportText{}, err
		}
		return decodedImportText{Bytes: decoded, Encoding: canonical, Source: "bom", Confidence: 100}, nil
	}
	if utf8.Valid(raw) {
		return decodedImportText{Bytes: raw, Encoding: "utf-8", Source: "utf8", Confidence: 100}, nil
	}

	detected := chardet.Detect(raw)
	confidence := int(math.Round(detected.Confidence * 100))
	if strings.TrimSpace(detected.Charset) == "" || confidence < minimumAutomaticEncodingConfidence {
		return decodedImportText{}, ValidationError{Message: "text encoding could not be detected confidently; choose it explicitly and upload again"}
	}
	decoded, canonical, err := decodeNamedImportEncoding(raw, detected.Charset)
	if err != nil {
		return decodedImportText{}, ValidationError{Message: fmt.Sprintf("detected text encoding %q is not supported; choose an encoding explicitly", detected.Charset)}
	}
	return decodedImportText{Bytes: decoded, Encoding: canonical, Source: "detected", Confidence: confidence}, nil
}

func decodeNamedImportEncoding(raw []byte, name string) ([]byte, string, error) {
	canonical := normalizeImportEncoding(name)
	if canonical == "auto" {
		return nil, "", ValidationError{Message: "text encoding must name a concrete encoding"}
	}
	if canonical == "utf-8" {
		raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		if !utf8.Valid(raw) {
			return nil, "", ValidationError{Message: "file is not valid UTF-8"}
		}
		return raw, canonical, nil
	}

	decoder, err := importEncodingDecoder(canonical)
	if err != nil || decoder == nil {
		return nil, "", ValidationError{Message: fmt.Sprintf("unsupported text encoding %q", name)}
	}
	decoded, err := decoder.NewDecoder().Bytes(raw)
	if err != nil {
		return nil, "", ValidationError{Message: fmt.Sprintf("file could not be decoded as %s", canonical)}
	}
	decoded = bytes.TrimPrefix(decoded, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(decoded) {
		return nil, "", ValidationError{Message: fmt.Sprintf("file could not be decoded as %s", canonical)}
	}
	return decoded, canonical, nil
}

func importEncodingDecoder(name string) (encoding.Encoding, error) {
	switch name {
	case "shift_jis", "cp932":
		return japanese.ShiftJIS, nil
	case "euc-kr", "cp949", "ks_c_5601-1987":
		return korean.EUCKR, nil
	}
	return lookup.LookupEncoding(name)
}

func normalizeImportEncoding(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "auto":
		return "auto"
	case "utf8", "utf-8-sig":
		return "utf-8"
	case "shift-jis", "sjis", "windows-31j":
		return "shift_jis"
	case "gb2312", "gbk", "cp936", "windows-936":
		return "gb18030"
	case "windows-932", "ms932":
		return "cp932"
	case "windows-949", "ms949":
		return "cp949"
	case "x-mac-cyrillic":
		return "maccyrillic"
	default:
		return normalized
	}
}

func importTextBOMEncoding(raw []byte) string {
	switch {
	case bytes.HasPrefix(raw, []byte("\x00\x00\xfe\xff")):
		return "utf-32be"
	case bytes.HasPrefix(raw, []byte("\xff\xfe\x00\x00")):
		return "utf-32le"
	case bytes.HasPrefix(raw, []byte("\xef\xbb\xbf")):
		return "utf-8"
	case bytes.HasPrefix(raw, []byte("\xfe\xff")):
		return "utf-16be"
	case bytes.HasPrefix(raw, []byte("\xff\xfe")):
		return "utf-16le"
	default:
		return ""
	}
}
