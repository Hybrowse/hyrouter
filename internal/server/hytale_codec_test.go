package server

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestEncodeDisconnectPayload_EmptyReason(t *testing.T) {
	p, err := encodeDisconnectPayload("")
	if err != nil {
		t.Fatalf("encodeDisconnectPayload: %v", err)
	}
	if len(p) != 2 {
		t.Fatalf("len=%d", len(p))
	}
	if p[0] != 0 || p[1] != 0 {
		t.Fatalf("payload=%v", p)
	}
}

func TestEncodeDisconnectPayload_WithReason(t *testing.T) {
	p, err := encodeDisconnectPayload("no")
	if err != nil {
		t.Fatalf("encodeDisconnectPayload: %v", err)
	}
	if len(p) < 4 {
		t.Fatalf("len=%d", len(p))
	}
	if p[0] != 0x01 {
		t.Fatalf("nullbits=%02x", p[0])
	}
	if p[1] != 0 {
		t.Fatalf("type=%02x", p[1])
	}
}

func TestReadFixedASCII_OutOfBounds(t *testing.T) {
	if got := readFixedASCII([]byte{1, 2}, 0, 5); got != "" {
		t.Fatalf("got=%q", got)
	}
}

func TestWriteVarString_Success(t *testing.T) {
	var b bytes.Buffer
	if err := writeVarString(&b, "hi", 10); err != nil {
		t.Fatalf("writeVarString: %v", err)
	}
}

func TestEncodeClientReferralPayload_HostOnly(t *testing.T) {
	p, err := encodeClientReferralPayload("play.hyvane.com", 5520, nil)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(p) < 9 {
		t.Fatalf("payload too small: %d", len(p))
	}
	if p[0] != 0x01 {
		t.Fatalf("nullBits=%02x", p[0])
	}
	if got := int32(binary.LittleEndian.Uint32(p[1:5])); got != 0 {
		t.Fatalf("hostToOffset=%d", got)
	}
	if got := int32(binary.LittleEndian.Uint32(p[5:9])); got != -1 {
		t.Fatalf("dataOffset=%d", got)
	}

	port := binary.LittleEndian.Uint16(p[9:11])
	if port != 5520 {
		t.Fatalf("port=%d", port)
	}
	if p[11] != 0x0f {
		t.Fatalf("expected varint len 0x0f, got %02x", p[11])
	}
	host := string(p[12 : 12+15])
	if host != "play.hyvane.com" {
		t.Fatalf("host=%q", host)
	}
}

func TestPacketName(t *testing.T) {
	if packetName(0) == "" {
		t.Fatalf("empty")
	}
	_ = packetName(1)
	_ = packetName(2)
	_ = packetName(3)
	_ = packetName(14)
	_ = packetName(18)
	if packetName(999) != "unknown" {
		t.Fatalf("expected unknown")
	}
}

func TestEncodeClientReferralPayload_WithData(t *testing.T) {
	data := make([]byte, 300)
	for i := range data {
		data[i] = byte(i)
	}
	_, err := encodeClientReferralPayload("h", 1, data)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func TestEncodeClientReferralPayload_Errors(t *testing.T) {
	if _, err := encodeClientReferralPayload("", 1, nil); err == nil {
		t.Fatalf("expected error")
	}
	longHost := make([]byte, 257)
	for i := range longHost {
		longHost[i] = 'a'
	}
	if _, err := encodeClientReferralPayload(string(longHost), 1, nil); err == nil {
		t.Fatalf("expected error")
	}
	data := make([]byte, 4097)
	if _, err := encodeClientReferralPayload("h", 1, data); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteVarInt(t *testing.T) {
	var b bytes.Buffer
	if err := writeVarInt(&b, -1); err == nil {
		t.Fatalf("expected error")
	}
	b.Reset()
	if err := writeVarInt(&b, 0); err != nil {
		t.Fatalf("err: %v", err)
	}
	b.Reset()
	if err := writeVarInt(&b, 128); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestMinInt(t *testing.T) {
	if minInt(1, 2) != 1 {
		t.Fatalf("unexpected")
	}
	if minInt(3, 2) != 2 {
		t.Fatalf("unexpected")
	}
}

func TestReadVarIntFailures(t *testing.T) {
	if _, _, ok := readVarInt([]byte{}, 0); ok {
		t.Fatalf("expected false")
	}
	if _, _, ok := readVarInt([]byte{0x80}, 0); ok {
		t.Fatalf("expected false")
	}
}

func TestReadVarStringFailures(t *testing.T) {
	if _, _, ok := readVarString([]byte{0x05, 'a'}, 0, 1); ok {
		t.Fatalf("expected false")
	}
	if _, _, ok := readVarString([]byte{0x02, 'a'}, 0, 10); ok {
		t.Fatalf("expected false")
	}
}

func TestDecodeHostAddressFailures(t *testing.T) {
	if _, _, ok := decodeHostAddress([]byte{}, 0); ok {
		t.Fatalf("expected false")
	}
	if _, _, ok := decodeHostAddress([]byte{0, 0, 0x81}, 0); ok {
		t.Fatalf("expected false")
	}
}

func TestWriteFramedPacket(t *testing.T) {
	var b bytes.Buffer
	payload := []byte{0x01, 0x02, 0x03}
	if err := writeFramedPacket(&b, 18, payload); err != nil {
		t.Fatalf("writeFramedPacket: %v", err)
	}
	out := b.Bytes()
	if len(out) != 8+len(payload) {
		t.Fatalf("len=%d", len(out))
	}
	if binary.LittleEndian.Uint32(out[0:4]) != uint32(len(payload)) {
		t.Fatalf("payloadLen=%d", binary.LittleEndian.Uint32(out[0:4]))
	}
	if binary.LittleEndian.Uint32(out[4:8]) != 18 {
		t.Fatalf("packetID=%d", binary.LittleEndian.Uint32(out[4:8]))
	}
	if !bytes.Equal(out[8:], payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestDecodeConnectPayload_Minimal(t *testing.T) {
	payload := buildConnectPayloadForTest(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
	)

	info, ok := decodeConnectPayload(payload)
	if !ok {
		t.Fatalf("decode failed")
	}
	if info.protocolHash != "6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9" {
		t.Fatalf("protocolHash=%q", info.protocolHash)
	}
	if info.clientType != 0 {
		t.Fatalf("clientType=%d", info.clientType)
	}
	if info.uuid != "d3e6ef90-e113-49a7-a845-1c11f24fe166" {
		t.Fatalf("uuid=%q", info.uuid)
	}
	if info.language != "de-DE" {
		t.Fatalf("language=%q", info.language)
	}
	if !info.identityTokenPresent {
		t.Fatalf("identityTokenPresent=false")
	}
	if info.username != "Krymo" {
		t.Fatalf("username=%q", info.username)
	}
}

func TestDecodeConnectPayload_ReferralFields(t *testing.T) {
	refData := []byte{1, 2, 3, 4, 5}
	payload := buildConnectPayloadForTestWithReferral(
		"6708f121966c1c443f4b0eb525b2f81d0a8dc61f5003a692a8fa157e5e02cea9",
		0,
		"d3e6ef90-e113-49a7-a845-1c11f24fe166",
		"de-DE",
		"tok",
		"Krymo",
		refData,
		"localhost",
		5520,
	)

	info, ok := decodeConnectPayload(payload)
	if !ok {
		t.Fatalf("decode failed")
	}
	if info.referralDataLen != len(refData) {
		t.Fatalf("referralDataLen=%d", info.referralDataLen)
	}
	if info.referralSource == nil || info.referralSource.Host != "localhost" || info.referralSource.Port != 5520 {
		t.Fatalf("referralSource=%#v", info.referralSource)
	}
}

func TestWriteVarStringTooLong(t *testing.T) {
	var b bytes.Buffer
	if err := writeVarString(&b, "ab", 1); err == nil {
		t.Fatalf("expected error")
	}
}

func buildConnectPayloadForTest(protocolHash string, clientType byte, uuidStr string, language string, identity string, username string) []byte {
	nullBits := byte(0x01 | 0x02)

	fixed := make([]byte, 102)
	fixed[0] = nullBits

	ph := []byte(protocolHash)
	copy(fixed[1:65], ph)
	fixed[65] = clientType

	uuidBytes := parseUUIDBytes(uuidStr)
	copy(fixed[66:82], uuidBytes)

	varBlock := make([]byte, 0, 128)

	langOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(language)))
	varBlock = append(varBlock, []byte(language)...)

	identOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(identity)))
	varBlock = append(varBlock, []byte(identity)...)

	userOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(username)))
	varBlock = append(varBlock, []byte(username)...)

	binary.LittleEndian.PutUint32(fixed[82:86], uint32(int32(langOffset)))
	binary.LittleEndian.PutUint32(fixed[86:90], uint32(int32(identOffset)))
	binary.LittleEndian.PutUint32(fixed[90:94], uint32(int32(userOffset)))
	binary.LittleEndian.PutUint32(fixed[94:98], uint32(0xFFFFFFFF))
	binary.LittleEndian.PutUint32(fixed[98:102], uint32(0xFFFFFFFF))

	return append(fixed, varBlock...)
}

func buildConnectPayloadForTestWithReferral(protocolHash string, clientType byte, uuidStr string, language string, identity string, username string, referralData []byte, referralHost string, referralPort uint16) []byte {
	nullBits := byte(0x01 | 0x02 | 0x04 | 0x08)

	fixed := make([]byte, 102)
	fixed[0] = nullBits

	ph := []byte(protocolHash)
	copy(fixed[1:65], ph)
	fixed[65] = clientType

	uuidBytes := parseUUIDBytes(uuidStr)
	copy(fixed[66:82], uuidBytes)

	varBlock := make([]byte, 0, 256)

	langOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(language)))
	varBlock = append(varBlock, []byte(language)...)

	identOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(identity)))
	varBlock = append(varBlock, []byte(identity)...)

	userOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(username)))
	varBlock = append(varBlock, []byte(username)...)

	refDataOffset := len(varBlock)
	varBlock = append(varBlock, byte(len(referralData)))
	varBlock = append(varBlock, referralData...)

	refSrcOffset := len(varBlock)
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], referralPort)
	varBlock = append(varBlock, tmp[:]...)
	varBlock = append(varBlock, byte(len(referralHost)))
	varBlock = append(varBlock, []byte(referralHost)...)

	binary.LittleEndian.PutUint32(fixed[82:86], uint32(int32(langOffset)))
	binary.LittleEndian.PutUint32(fixed[86:90], uint32(int32(identOffset)))
	binary.LittleEndian.PutUint32(fixed[90:94], uint32(int32(userOffset)))
	binary.LittleEndian.PutUint32(fixed[94:98], uint32(int32(refDataOffset)))
	binary.LittleEndian.PutUint32(fixed[98:102], uint32(int32(refSrcOffset)))

	return append(fixed, varBlock...)
}

func parseUUIDBytes(s string) []byte {
	b := make([]byte, 16)
	hex := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		hex = append(hex, s[i])
	}
	for i := 0; i < 16; i++ {
		b[i] = fromHex(hex[i*2])<<4 | fromHex(hex[i*2+1])
	}
	return b
}

func fromHex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

func TestDecodeConnectPayloadUpdate7Language(t *testing.T) {
	for _, language := range []string{"de-DE", strings.Repeat("a", 17), strings.Repeat("a", 100)} {
		t.Run(language, func(t *testing.T) {
			info, ok := decodeConnectPayload(buildConnectPayloadV2ForTest(language))
			if !ok {
				t.Fatal("Connect packet was rejected")
			}
			if info.language != language || info.username != "TestPlayer" || info.uuid != "d3e6ef90-e113-49a7-a845-1c11f24fe166" {
				t.Fatalf("lost routing fields: %#v", info)
			}
			if info.protocolCrc != 0x12345678 || info.protocolBuildNumber != 7 || info.clientVersion != "0.7.3" || !info.identityTokenPresent {
				t.Fatalf("incorrect Connect metadata: %#v", info)
			}
		})
	}
	if _, ok := decodeConnectPayloadV2(buildConnectPayloadV2ForTest(strings.Repeat("a", 101))); ok {
		t.Fatal("V2 accepted a language longer than the protocol limit")
	}
}

// Synthetic Connect fixture: no captured identity token or proprietary binary.
func buildConnectPayloadV2ForTest(language string) []byte {
	fixed := make([]byte, 66)
	fixed[0] = 1 // identity token present
	binary.LittleEndian.PutUint32(fixed[1:5], 0x12345678)
	binary.LittleEndian.PutUint32(fixed[5:9], 7)
	copy(fixed[9:29], "0.7.3")
	copy(fixed[30:46], parseUUIDBytes("d3e6ef90-e113-49a7-a845-1c11f24fe166"))
	var variable bytes.Buffer
	for _, field := range []struct {
		offset int
		value  string
	}{{46, "TestPlayer"}, {50, "synthetic-test-token"}, {54, language}} {
		binary.LittleEndian.PutUint32(fixed[field.offset:field.offset+4], uint32(variable.Len()))
		_ = writeVarString(&variable, field.value, 8192)
	}
	binary.LittleEndian.PutUint32(fixed[58:62], 0xffffffff)
	binary.LittleEndian.PutUint32(fixed[62:66], 0xffffffff)
	return append(fixed, variable.Bytes()...)
}

func readProtocolFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/hytale-0.7.4-" + name + ".hex")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestConnectProtocol3FromServerSerializer(t *testing.T) {
	payload := readProtocolFixture(t, "connect")
	info, ok := decodeConnectPayloadForProtocol(payload, "hytale/3")
	if !ok {
		t.Fatal("rejected a packet from the server serializer")
	}
	if info.protocolCrc != -386561135 || info.protocolBuildNumber != 258 || info.clientVersion != "0.7.4" || info.clientType != 0 {
		t.Fatalf("incorrect version fields: %#v", info)
	}
	if info.uuid != "00112233-4455-6677-8899-aabbccddeeff" || info.username != "TestPlayer" || info.language != "de-DE" || !info.identityTokenPresent {
		t.Fatalf("incorrect routing hints: %#v", info)
	}
	if info.referralDataLen != 3 || info.referralSource == nil || info.referralSource.Host != "localhost" || info.referralSource.Port != 5520 {
		t.Fatalf("incorrect referral: %#v", info)
	}
	for _, slot := range []int{30, 34, 38, 42} {
		t.Run(fmt.Sprint(slot), func(t *testing.T) {
			bad := bytes.Clone(payload)
			binary.LittleEndian.PutUint32(bad[slot:slot+4], 0x7fffffff)
			if _, ok := decodeConnectPayloadForProtocol(bad, "hytale/3"); ok {
				t.Fatal("accepted an invalid offset")
			}
		})
	}
	if _, ok := decodeConnectPayloadForProtocol(payload[:len(payload)-1], "hytale/3"); ok {
		t.Fatal("accepted truncated referral source")
	}
}

func TestProtocol3LanguageLimit(t *testing.T) {
	original := readProtocolFixture(t, "connect")
	start := 46 + int(binary.LittleEndian.Uint32(original[34:38]))
	for _, length := range []int{17, 100, 101} {
		payload := append(bytes.Clone(original[:start]), byte(length))
		payload = append(payload, strings.Repeat("a", length)...)
		payload = append(payload, original[start+6:]...)
		for _, slot := range []int{38, 42} {
			binary.LittleEndian.PutUint32(payload[slot:slot+4], binary.LittleEndian.Uint32(original[slot:slot+4])+uint32(length-5))
		}
		info, ok := decodeConnectPayloadForProtocol(payload, "hytale/3")
		if ok != (length <= 100) {
			t.Fatalf("length %d: accepted=%t", length, ok)
		}
		if ok && len(info.language) != length {
			t.Fatal("lost language")
		}
	}
}

func TestReferralProtocol3MatchesServerSerializer(t *testing.T) {
	want := readProtocolFixture(t, "referral")
	got, err := encodeClientReferralPayload("127.0.0.1", 15520, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("referral differs from server serializer: got %x want %x", got, want)
	}
}

func TestUnverifiedIdentityClaimsRejectMalformedHints(t *testing.T) {
	for _, token := range []string{"", "not-a-token", "a.%%%.b", "a.e30.b", "a.bnVsbA.b"} {
		if uuid, username := unverifiedIdentityClaims(token); uuid != "" || username != "" {
			t.Fatal("accepted malformed identity claims")
		}
	}
}
