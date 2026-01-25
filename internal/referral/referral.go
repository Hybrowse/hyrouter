package referral

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	magicV1           = "HYRP"
	envelopeVersionV1 = uint8(1)

	flagSignedHMACSHA256 = uint8(0x01)

	maxEnvelopeSize = 4096
	hmacSize        = 32
	v1HeaderSize    = 4 + 1 + 1 + 1 + 2
)

type Envelope struct {
	Version  uint8
	Flags    uint8
	KeyID    uint8
	Content  []byte
	HMAC     []byte
	Signed   bool
	Raw      []byte
	RawNoSig []byte
}

func DecodeSecret(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty secret")
	}
	if strings.HasPrefix(strings.ToLower(s), "base64:") {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s[len("base64:"):]))
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	if strings.HasPrefix(strings.ToLower(s), "hex:") {
		b, err := hex.DecodeString(strings.TrimSpace(s[len("hex:"):]))
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return []byte(s), nil
}

func EncodeV1(content []byte, keyID uint8, secret []byte) ([]byte, error) {
	if content == nil {
		content = []byte{}
	}
	if len(content) > maxEnvelopeSize {
		return nil, fmt.Errorf("content too large")
	}

	flags := uint8(0)
	if len(secret) > 0 {
		flags |= flagSignedHMACSHA256
	}

	sigSize := 0
	if (flags & flagSignedHMACSHA256) != 0 {
		sigSize = hmacSize
	}

	total := v1HeaderSize + len(content) + sigSize
	if total > maxEnvelopeSize {
		return nil, fmt.Errorf("envelope too large")
	}

	out := make([]byte, total)
	copy(out[0:4], []byte(magicV1))
	out[4] = envelopeVersionV1
	out[5] = flags
	out[6] = keyID
	binary.LittleEndian.PutUint16(out[7:9], uint16(len(content)))
	copy(out[9:9+len(content)], content)

	if (flags & flagSignedHMACSHA256) != 0 {
		mac := hmac.New(sha256.New, secret)
		_, _ = mac.Write(out[:9+len(content)])
		sum := mac.Sum(nil)
		copy(out[9+len(content):], sum)
	}

	return out, nil
}

func Parse(b []byte) (Envelope, error) {
	if len(b) < v1HeaderSize {
		return Envelope{}, fmt.Errorf("buffer too small")
	}
	if string(b[0:4]) != magicV1 {
		return Envelope{}, fmt.Errorf("invalid magic")
	}
	if b[4] != envelopeVersionV1 {
		return Envelope{}, fmt.Errorf("unsupported version")
	}

	flags := b[5]
	keyID := b[6]
	contentLen := int(binary.LittleEndian.Uint16(b[7:9]))

	signed := (flags & flagSignedHMACSHA256) != 0
	need := v1HeaderSize + contentLen
	if signed {
		need += hmacSize
	}
	if contentLen < 0 || need > len(b) {
		return Envelope{}, fmt.Errorf("invalid length")
	}
	if need > maxEnvelopeSize {
		return Envelope{}, fmt.Errorf("envelope too large")
	}

	content := b[9 : 9+contentLen]
	var mac []byte
	if signed {
		mac = b[9+contentLen : 9+contentLen+hmacSize]
	}

	raw := b[:need]
	rawNoSig := raw
	if signed {
		rawNoSig = raw[:len(raw)-hmacSize]
	}

	return Envelope{
		Version:  envelopeVersionV1,
		Flags:    flags,
		KeyID:    keyID,
		Content:  append([]byte(nil), content...),
		HMAC:     append([]byte(nil), mac...),
		Signed:   signed,
		Raw:      append([]byte(nil), raw...),
		RawNoSig: append([]byte(nil), rawNoSig...),
	}, nil
}

func VerifyHMACSHA256(b []byte, secret []byte) (Envelope, error) {
	env, err := Parse(b)
	if err != nil {
		return Envelope{}, err
	}
	if !env.Signed {
		return Envelope{}, fmt.Errorf("not signed")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(env.RawNoSig)
	expected := mac.Sum(nil)
	if !hmac.Equal(expected, env.HMAC) {
		return Envelope{}, fmt.Errorf("invalid hmac")
	}
	return env, nil
}

func VerifyWithSecretProvider(b []byte, secretForKeyID func(keyID uint8) ([]byte, bool)) (Envelope, error) {
	env, err := Parse(b)
	if err != nil {
		return Envelope{}, err
	}
	if !env.Signed {
		return Envelope{}, fmt.Errorf("not signed")
	}
	secret, ok := secretForKeyID(env.KeyID)
	if !ok {
		return Envelope{}, fmt.Errorf("unknown key id")
	}
	return VerifyHMACSHA256(b, secret)
}
