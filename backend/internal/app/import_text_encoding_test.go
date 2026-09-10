package app

import (
	"testing"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeImportText_ExplicitEncodingsAcrossScripts(t *testing.T) {
	tests := []struct {
		name     string
		encoding string
		codec    encoding.Encoding
		text     string
	}{
		{name: "Central European", encoding: "windows-1250", codec: charmap.Windows1250, text: "Zażółć gęślą jaźń"},
		{name: "Cyrillic", encoding: "windows-1251", codec: charmap.Windows1251, text: "Кафе и питание"},
		{name: "Greek", encoding: "windows-1253", codec: charmap.Windows1253, text: "Καφές και φαγητό"},
		{name: "Hebrew", encoding: "windows-1255", codec: charmap.Windows1255, text: "בית קפה ומזון"},
		{name: "Arabic", encoding: "windows-1256", codec: charmap.Windows1256, text: "مقهى وطعام"},
		{name: "Japanese", encoding: "shift_jis", codec: japanese.ShiftJIS, text: "喫茶店と食料品"},
		{name: "Simplified Chinese", encoding: "gb18030", codec: simplifiedchinese.GB18030, text: "咖啡馆和食品"},
		{name: "Traditional Chinese", encoding: "big5", codec: traditionalchinese.Big5, text: "咖啡館和食品"},
		{name: "Korean", encoding: "euc-kr", codec: korean.EUCKR, text: "카페와 식료품"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.codec.NewEncoder().Bytes([]byte(tc.text))
			require.NoError(t, err)

			got, err := decodeImportText(raw, tc.encoding)
			require.NoError(t, err)
			assert.Equal(t, tc.text, string(got.Bytes))
			assert.Equal(t, tc.encoding, got.Encoding)
			assert.Equal(t, "selected", got.Source)
			assert.Equal(t, 100, got.Confidence)
		})
	}
}

func TestDecodeImportText_AutoDetectsMSMoneyWindows1251(t *testing.T) {
	raw := []byte("P\xca\xe0\xf4\xe5 \xe8 \xef\xe8\xf2\xe0\xed\xe8\xe5 \xe4\xeb\xff \xe2\xf1\xe5\xe9 \xf1\xe5\xec\xfc\xe8\n")

	got, err := decodeImportText(raw, "auto")
	require.NoError(t, err)
	assert.Equal(t, "PКафе и питание для всей семьи\n", string(got.Bytes))
	assert.Equal(t, "windows-1251", got.Encoding)
	assert.Equal(t, "detected", got.Source)
	assert.GreaterOrEqual(t, got.Confidence, minimumAutomaticEncodingConfidence)
}

func TestDecodeImportText_ValidUTF8IsNeverReinterpreted(t *testing.T) {
	raw := []byte("Кафе Καφές カフェ مقهى")

	got, err := decodeImportText(raw, "auto")
	require.NoError(t, err)
	assert.Equal(t, raw, got.Bytes)
	assert.Equal(t, "utf-8", got.Encoding)
	assert.Equal(t, "utf8", got.Source)
	assert.Equal(t, 100, got.Confidence)
}

func TestDecodeImportText_UsesUnicodeBOMWithoutStatisticalDetection(t *testing.T) {
	raw, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte("PКафе\n"))
	require.NoError(t, err)

	got, err := decodeImportText(raw, "auto")
	require.NoError(t, err)
	assert.Equal(t, "PКафе\n", string(got.Bytes))
	assert.Equal(t, "utf-16le", got.Encoding)
	assert.Equal(t, "bom", got.Source)
	assert.Equal(t, 100, got.Confidence)
}

func TestDecodeImportText_AmbiguousLegacyTextRequiresExplicitEncoding(t *testing.T) {
	raw, err := charmap.Windows1250.NewEncoder().Bytes([]byte("Zażółć gęślą jaźń"))
	require.NoError(t, err)

	_, err = decodeImportText(raw, "auto")
	require.Error(t, err)
	var validationErr ValidationError
	assert.ErrorAs(t, err, &validationErr)
	assert.Contains(t, validationErr.Message, "choose it explicitly")
}

func TestDecodeImportText_UnknownExplicitEncodingIsRejected(t *testing.T) {
	_, err := decodeImportText([]byte("text"), "made-up-code-page")
	require.Error(t, err)
	var validationErr ValidationError
	assert.ErrorAs(t, err, &validationErr)
}

func TestDecodeImportText_EncodingSelectorOptionsAreSupported(t *testing.T) {
	options := []string{
		"utf-8", "utf-16le", "utf-16be",
		"windows-1250", "windows-1251", "windows-1252", "windows-1253", "windows-1254",
		"windows-1255", "windows-1256", "windows-1257", "windows-1258", "windows-874",
		"iso-8859-1", "iso-8859-2", "iso-8859-5", "iso-8859-6", "iso-8859-7",
		"iso-8859-8", "iso-8859-9", "iso-8859-13", "iso-8859-15",
		"koi8-r", "koi8-u", "ibm866", "macintosh", "maccyrillic",
		"shift_jis", "euc-jp", "gb18030", "big5", "euc-kr",
	}

	for _, option := range options {
		t.Run(option, func(t *testing.T) {
			_, _, err := decodeNamedImportEncoding([]byte(""), option)
			require.NoError(t, err)
		})
	}
}
