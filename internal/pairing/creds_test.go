package pairing

import (
	"strings"
	"testing"
	"unicode"
)

func TestGenerateServiceName(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s, err := GenerateServiceName()
		if err != nil {
			t.Fatalf("GenerateServiceName: %v", err)
		}
		if !strings.HasPrefix(s, "studio-") {
			t.Fatalf("missing studio- prefix: %q", s)
		}
		suffix := strings.TrimPrefix(s, "studio-")
		if len(suffix) != serviceNameRandLen {
			t.Fatalf("suffix length = %d, want %d", len(suffix), serviceNameRandLen)
		}
		for _, r := range suffix {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				t.Fatalf("non-alnum rune %q in suffix %q", r, suffix)
			}
		}
		if seen[s] {
			t.Fatalf("duplicate service name %q after %d iterations", s, i)
		}
		seen[s] = true
	}
}

func TestGeneratePassword(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		p, err := GeneratePassword()
		if err != nil {
			t.Fatalf("GeneratePassword: %v", err)
		}
		if len(p) != passwordLen {
			t.Fatalf("password length = %d, want %d", len(p), passwordLen)
		}
		for _, r := range p {
			if r < 0x21 || r > 0x7e {
				t.Fatalf("non-printable ASCII rune %q in password %q", r, p)
			}
		}
		if seen[p] {
			t.Fatalf("duplicate password %q after %d iterations", p, i)
		}
		seen[p] = true
	}
}

func TestQRPayload(t *testing.T) {
	got := QRPayload("studio-abc", "pw1234")
	want := "WIFI:T:ADB;S:studio-abc;P:pw1234;;"
	if got != want {
		t.Fatalf("QRPayload = %q, want %q", got, want)
	}
}
