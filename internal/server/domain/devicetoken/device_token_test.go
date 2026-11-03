package devicetoken_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/devicetoken"
)

func TestNewSecret_HasThePrefixAndIsUnique(t *testing.T) {
	first, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(first.String(), devicetoken.Prefix) {
		t.Errorf("token %q does not start with %q", first, devicetoken.Prefix)
	}
	if first.String() == second.String() {
		t.Errorf("two tokens are the same: %q", first)
	}
}

func TestSecret_HashIsStableAndTheFingerprintHidesIt(t *testing.T) {
	secret, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if secret.Hash() != secret.Hash() {
		t.Error("the hash of one token changed between calls")
	}
	if len(secret.Hash()) != 64 {
		t.Errorf("hash %q is not a hex SHA-256", secret.Hash())
	}
	if !strings.HasPrefix(secret.Fingerprint(), devicetoken.Prefix) {
		t.Errorf("fingerprint %q does not carry the prefix", secret.Fingerprint())
	}
	if strings.Contains(secret.Fingerprint(), strings.TrimPrefix(secret.String(), devicetoken.Prefix)) {
		t.Errorf("fingerprint %q leaks the secret", secret.Fingerprint())
	}
	if got := devicetoken.FingerprintOfHash(secret.Hash()); got != secret.Fingerprint() {
		t.Errorf("FingerprintOfHash(%q) = %q, want %q", secret.Hash(), got, secret.Fingerprint())
	}
}

func TestParseSecret(t *testing.T) {
	secret, err := devicetoken.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := devicetoken.ParseSecret(secret.String())
	if err != nil {
		t.Fatalf("ParseSecret(%q): %v", secret, err)
	}
	if parsed != secret {
		t.Errorf("ParseSecret(%q) = %q", secret, parsed)
	}

	for _, raw := range []string{
		"",
		"secret",
		"Bearer " + secret.String(),
		strings.ToUpper(devicetoken.Prefix) + strings.TrimPrefix(secret.String(), devicetoken.Prefix),
		devicetoken.Prefix,
		devicetoken.Prefix + "not-hex",
		devicetoken.Prefix + strings.TrimPrefix(secret.String(), devicetoken.Prefix) + "00",
	} {
		if _, err := devicetoken.ParseSecret(raw); !errors.Is(err, devicetoken.ErrNotADeviceToken) {
			t.Errorf("ParseSecret(%q) error = %v, want ErrNotADeviceToken", raw, err)
		}
	}
}
