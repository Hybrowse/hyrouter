package referral

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"testing"
)

func TestDecodeSecret(t *testing.T) {
	if _, err := DecodeSecret(""); err == nil {
		t.Fatalf("expected error")
	}

	s, err := DecodeSecret("abc")
	if err != nil {
		t.Fatalf("DecodeSecret: %v", err)
	}
	if string(s) != "abc" {
		t.Fatalf("got=%q", string(s))
	}

	s, err = DecodeSecret("base64:YWJj")
	if err != nil {
		t.Fatalf("DecodeSecret base64: %v", err)
	}
	if string(s) != "abc" {
		t.Fatalf("got=%q", string(s))
	}

	s, err = DecodeSecret("hex:616263")
	if err != nil {
		t.Fatalf("DecodeSecret hex: %v", err)
	}
	if string(s) != "abc" {
		t.Fatalf("got=%q", string(s))
	}

	if _, err := DecodeSecret("base64:$$$"); err == nil {
		t.Fatalf("expected error")
	}
	if _, err := DecodeSecret("hex:zz"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestEncodeV1_NilContentReturnsNil(t *testing.T) {
	b, err := EncodeV1(nil, 1, []byte("secret"))
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}
	if b == nil {
		t.Fatalf("expected envelope")
	}
	env, err := Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(env.Content) != 0 {
		t.Fatalf("content len=%d", len(env.Content))
	}
}

func TestEncodeV1_UnsignedAndParse(t *testing.T) {
	content := []byte("hello")
	b, err := EncodeV1(content, 7, nil)
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("empty")
	}

	env, err := Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if env.Version != 1 {
		t.Fatalf("version=%d", env.Version)
	}
	if env.Flags != 0 {
		t.Fatalf("flags=%02x", env.Flags)
	}
	if env.KeyID != 7 {
		t.Fatalf("keyID=%d", env.KeyID)
	}
	if env.Signed {
		t.Fatalf("expected unsigned")
	}
	if !bytes.Equal(env.Content, content) {
		t.Fatalf("content=%q", string(env.Content))
	}
	if len(env.HMAC) != 0 {
		t.Fatalf("expected no mac")
	}
	if !bytes.Equal(env.Raw, b) {
		t.Fatalf("raw mismatch")
	}
	if !bytes.Equal(env.RawNoSig, b) {
		t.Fatalf("rawnosig mismatch")
	}
}

func TestEncodeV1_SignedAndVerify(t *testing.T) {
	secret := []byte("secret")
	content := []byte{1, 2, 3}

	b, err := EncodeV1(content, 9, secret)
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}

	env, err := Parse(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !env.Signed {
		t.Fatalf("expected signed")
	}
	if env.KeyID != 9 {
		t.Fatalf("keyID=%d", env.KeyID)
	}
	if len(env.HMAC) != 32 {
		t.Fatalf("mac len=%d", len(env.HMAC))
	}

	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(env.RawNoSig)
	expected := mac.Sum(nil)
	if !hmac.Equal(env.HMAC, expected) {
		t.Fatalf("mac mismatch")
	}

	verified, err := VerifyHMACSHA256(b, secret)
	if err != nil {
		t.Fatalf("VerifyHMACSHA256: %v", err)
	}
	if !bytes.Equal(verified.Content, content) {
		t.Fatalf("content mismatch")
	}
}

func TestVerifyHMACSHA256_Errors(t *testing.T) {
	b, err := EncodeV1([]byte("x"), 1, []byte("secret"))
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}

	if _, err := VerifyHMACSHA256(b, []byte("wrong")); err == nil {
		t.Fatalf("expected error")
	}

	unsigned, err := EncodeV1([]byte("x"), 1, nil)
	if err != nil {
		t.Fatalf("EncodeV1 unsigned: %v", err)
	}
	if _, err := VerifyHMACSHA256(unsigned, []byte("secret")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestVerifyWithSecretProvider(t *testing.T) {
	b, err := EncodeV1([]byte("x"), 2, []byte("secret"))
	if err != nil {
		t.Fatalf("EncodeV1: %v", err)
	}

	_, err = VerifyWithSecretProvider(b, func(keyID uint8) ([]byte, bool) {
		if keyID == 2 {
			return []byte("secret"), true
		}
		return nil, false
	})
	if err != nil {
		t.Fatalf("VerifyWithSecretProvider: %v", err)
	}

	if _, err := VerifyWithSecretProvider(b, func(keyID uint8) ([]byte, bool) { return nil, false }); err == nil {
		t.Fatalf("expected error")
	}
}

func TestParse_Errors(t *testing.T) {
	if _, err := Parse([]byte{}); err == nil {
		t.Fatalf("expected error")
	}

	// bad magic
	b := make([]byte, v1HeaderSize)
	copy(b[0:4], []byte("NOPE"))
	b[4] = 1
	if _, err := Parse(b); err == nil {
		t.Fatalf("expected error")
	}

	// unsupported version
	b2 := make([]byte, v1HeaderSize)
	copy(b2[0:4], []byte(magicV1))
	b2[4] = 2
	if _, err := Parse(b2); err == nil {
		t.Fatalf("expected error")
	}

	// invalid length
	b3 := make([]byte, v1HeaderSize)
	copy(b3[0:4], []byte(magicV1))
	b3[4] = 1
	b3[5] = 0
	b3[6] = 0
	// contentLen=1 but buffer has no content
	b3[7] = 1
	b3[8] = 0
	if _, err := Parse(b3); err == nil {
		t.Fatalf("expected error")
	}
}
