package tokens

import (
	"bytes"
	"testing"
)

func TestEmailVerifyOTPKeyed(t *testing.T) {
	k1 := EmailVerifyOTPKey([]byte("secret-one"))
	k2 := EmailVerifyOTPKey([]byte("secret-two"))
	if bytes.Equal(k1, k2) {
		t.Fatal("different secrets gave the same key")
	}

	code, hash, err := GenerateEmailVerifyOTP(k1)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEmailVerifyOTP(code) {
		t.Fatalf("generated code %q is not 6 digits", code)
	}
	if HashEmailVerifyOTPKeyed(k1, code) != hash {
		t.Fatal("stored hash does not match the code")
	}
	// Без секрета код по хэшу не проверить: голый sha256 и чужой ключ не совпадают.
	if HashEmailVerifyOTP(code) == hash {
		t.Fatal("keyed hash equals plain sha256")
	}
	if HashEmailVerifyOTPKeyed(k2, code) == hash {
		t.Fatal("hash matches under another key")
	}
}
