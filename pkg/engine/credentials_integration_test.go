//go:build credentialintegration

package engine

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// This opt-in test uses only a disposable, synthetic credential.
func TestSystemCredentialStoreRoundTrip(t *testing.T) {
	if os.Getenv("TOYS_KEYRING_TEST") != "1" {
		t.Skip("set TOYS_KEYRING_TEST=1 to exercise the native credential store")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	ref := "integration/" + hex.EncodeToString(random)
	credentials := SystemCredentials{}
	if err := credentials.Set(ref, "synthetic-test-value"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := credentials.Delete(ref); err != nil {
			t.Error(err)
		}
	})
	token, err := credentials.Get(ref)
	if err != nil || token != "synthetic-test-value" {
		t.Fatal("credential round trip failed")
	}
	if err := credentials.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := credentials.Get(ref); err == nil {
		t.Fatal("deleted credential still accessible")
	}
}
