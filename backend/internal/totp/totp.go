// Package totp implements RFC 6238 time-based one-time passwords (HMAC-SHA1,
// 6 digits, 30-second steps) — the shape every mainstream authenticator app
// speaks. It is deliberately tiny and dependency-free: the algorithm is a
// truncated HMAC, and pulling in a library for it would add supply-chain
// surface to the authentication path for no benefit.
//
// The package handles secrets and codes only. Storage, encryption, replay
// tracking, and throttling belong to the caller (see app/auth_mfa.go).
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const (
	// Period is the RFC 6238 time step. 30 seconds is what authenticator apps
	// assume and is not configurable for that reason.
	Period = 30 * time.Second

	// Digits is the code length. Six is the universal default.
	Digits = 6

	// secretBytes is the shared-secret length. RFC 4226 requires at least 128
	// bits and recommends 160 — the size of the SHA-1 output.
	secretBytes = 20
)

// ErrInvalidSecret is returned when a stored secret is not valid base32.
var ErrInvalidSecret = errors.New("totp: secret is not valid base32")

// base32NoPad is the encoding authenticator apps expect: RFC 4648 base32,
// uppercase, unpadded.
var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new random shared secret, base32-encoded for
// display and for the otpauth URI.
func GenerateSecret() (string, error) {
	secret := make([]byte, secretBytes)
	if _, err := io.ReadFull(rand.Reader, secret); err != nil {
		return "", fmt.Errorf("totp: generate secret: %w", err)
	}

	return base32NoPad.EncodeToString(secret), nil
}

// Step is the RFC 6238 counter for an instant.
func Step(at time.Time) uint64 {
	seconds := at.UTC().Unix()
	if seconds < 0 {
		return 0
	}

	return uint64(seconds) / uint64(Period.Seconds())
}

// Code returns the code for one counter value.
func Code(secret string, step uint64) (string, error) {
	key, err := decodeSecret(secret)
	if err != nil {
		return "", err
	}

	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], step)
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)

	// RFC 4226 dynamic truncation: the low nibble of the last byte selects the
	// 4-byte window, whose top bit is masked off.
	offset := sum[len(sum)-1] & 0x0f
	truncated := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	divisor := uint32(1)
	for range Digits {
		divisor *= 10
	}

	return fmt.Sprintf("%0*d", Digits, truncated%divisor), nil
}

// Validate checks a user-entered code against the secret, accepting the
// current step and `skew` steps either side of it to tolerate clock drift and
// a user typing a code as it rolls over.
//
// It returns the step the code matched, which the caller must persist: a TOTP
// code stays valid for its whole step, so without refusing steps at or below
// the last accepted one, an observed code can be replayed within the window.
//
// The comparison is constant-time and the search always runs the full window,
// so a wrong code takes the same work as a right one.
func Validate(secret string, code string, now time.Time, skew int) (uint64, bool) {
	code = normalizeCode(code)
	if len(code) != Digits {
		return 0, false
	}
	skew = max(skew, 0)

	current := Step(now)
	var matchedStep uint64
	matched := false
	for offset := -skew; offset <= skew; offset++ {
		step := current
		switch {
		case offset < 0:
			shift := uint64(-offset)
			if shift > current {
				continue
			}
			step = current - shift
		case offset > 0:
			step = current + uint64(offset)
		}

		candidate, err := Code(secret, step)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(code)) == 1 && !matched {
			matchedStep = step
			matched = true
		}
	}

	return matchedStep, matched
}

// URI builds the otpauth:// enrollment URI an authenticator app scans or
// accepts pasted. Algorithm, digits, and period are stated explicitly rather
// than left to each app's defaults.
func URI(issuer string, accountName string, secret string) string {
	issuer = strings.TrimSpace(issuer)
	accountName = strings.TrimSpace(accountName)
	label := accountName
	if issuer != "" {
		label = issuer + ":" + accountName
	}

	query := url.Values{}
	query.Set("secret", secret)
	if issuer != "" {
		query.Set("issuer", issuer)
	}
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprintf("%d", Digits))
	query.Set("period", fmt.Sprintf("%d", int(Period.Seconds())))

	return (&url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + label,
		RawQuery: query.Encode(),
	}).String()
}

// normalizeCode accepts what people actually type: spaces from a copy-paste,
// and nothing else.
func normalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.TrimSpace(code))
}

func decodeSecret(secret string) ([]byte, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(secret))
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.TrimRight(cleaned, "=")
	key, err := base32NoPad.DecodeString(cleaned)
	if err != nil {
		return nil, ErrInvalidSecret
	}
	if len(key) == 0 {
		return nil, ErrInvalidSecret
	}

	return key, nil
}
