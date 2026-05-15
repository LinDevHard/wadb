package pairing

import (
	"crypto/rand"
	"fmt"
)

const (
	serviceNamePrefix = "studio-"
	serviceNameRandLen = 10
	passwordLen        = 12
)

const (
	alnum      = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	passChars  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// GenerateServiceName returns a string of the form "studio-XXXXXXXXXX".
// The "studio-" prefix is required: the Android device uses this string
// verbatim as the mDNS instance name when it advertises _adb-tls-pairing._tcp
// after scanning the QR — it is our discovery key.
func GenerateServiceName() (string, error) {
	suffix, err := randString(serviceNameRandLen, alnum)
	if err != nil {
		return "", fmt.Errorf("service name: %w", err)
	}
	return serviceNamePrefix + suffix, nil
}

func GeneratePassword() (string, error) {
	s, err := randString(passwordLen, passChars)
	if err != nil {
		return "", fmt.Errorf("password: %w", err)
	}
	return s, nil
}

// QRPayload builds the exact string Android Studio puts in its pairing QR.
// Trailing double `;;` is required by the Android-side parser.
func QRPayload(serviceName, password string) string {
	return fmt.Sprintf("WIFI:T:ADB;S:%s;P:%s;;", serviceName, password)
}

func randString(n int, alphabet string) (string, error) {
	if len(alphabet) == 0 || len(alphabet) > 256 {
		return "", fmt.Errorf("invalid alphabet length %d", len(alphabet))
	}
	buf := make([]byte, n)
	out := make([]byte, n)
	max := byte(len(alphabet))
	// Rejection-sample to avoid modulo bias.
	limit := byte(256 - (256 % int(max)))
	i := 0
	for i < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out[i] = alphabet[b%max]
			i++
			if i == n {
				break
			}
		}
	}
	return string(out), nil
}
