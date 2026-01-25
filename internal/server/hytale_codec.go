package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	maxHytalePayloadLen      = int32(1677721600)
	maxDebugBufferedPayload  = int32(16 * 1024 * 1024)
	maxDebugPayloadHexPrefix = 96
)

func writeFramedPacket(w io.Writer, packetID int32, payload []byte) error {
	// Hytale stream framing: uint32le(payloadLen) + uint32le(packetID) + payload.
	buf := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(payload)))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(packetID))
	copy(buf[8:], payload)
	for len(buf) > 0 {
		n, err := w.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

func encodeDisconnectPayload(reason string) ([]byte, error) {
	var payload bytes.Buffer
	if reason == "" {
		payload.WriteByte(0)
		payload.WriteByte(0)
		return payload.Bytes(), nil
	}
	payload.WriteByte(0x01)
	payload.WriteByte(0)
	if err := writeVarString(&payload, reason, 4096000); err != nil {
		return nil, err
	}
	return payload.Bytes(), nil
}

func encodeConnectAcceptPayload(passwordChallenge []byte) ([]byte, error) {
	var payload bytes.Buffer
	if passwordChallenge == nil {
		payload.WriteByte(0)
		return payload.Bytes(), nil
	}
	if len(passwordChallenge) > 64 {
		return nil, fmt.Errorf("password challenge too long")
	}
	payload.WriteByte(0x01)
	if err := writeVarInt(&payload, len(passwordChallenge)); err != nil {
		return nil, err
	}
	payload.Write(passwordChallenge)
	return payload.Bytes(), nil
}

func encodeClientReferralPayload(host string, port uint16, data []byte) ([]byte, error) {
	if host == "" {
		return nil, fmt.Errorf("host must not be empty")
	}
	hostBytes := []byte(host)
	if len(hostBytes) > 256 {
		return nil, fmt.Errorf("host too long")
	}
	if len(data) > 4096 {
		return nil, fmt.Errorf("data too long")
	}

	var payload bytes.Buffer

	nullBits := byte(0)
	if host != "" {
		nullBits |= 0x01
	}
	if data != nil {
		nullBits |= 0x02
	}

	payload.WriteByte(nullBits)

	hostToOffsetSlot := payload.Len()
	payload.Write([]byte{0, 0, 0, 0})
	dataOffsetSlot := payload.Len()
	payload.Write([]byte{0, 0, 0, 0})

	varBlockStart := payload.Len()

	if (nullBits & 0x01) != 0 {
		setInt32LE(payload.Bytes(), hostToOffsetSlot, int32(payload.Len()-varBlockStart))
		if err := writeHostAddress(&payload, host, port); err != nil {
			return nil, err
		}
	} else {
		setInt32LE(payload.Bytes(), hostToOffsetSlot, -1)
	}

	if (nullBits & 0x02) != 0 {
		setInt32LE(payload.Bytes(), dataOffsetSlot, int32(payload.Len()-varBlockStart))
		if err := writeVarInt(&payload, len(data)); err != nil {
			return nil, err
		}
		payload.Write(data)
	} else {
		setInt32LE(payload.Bytes(), dataOffsetSlot, -1)
	}

	if payload.Len() < 9 {
		return nil, fmt.Errorf("referral payload too small")
	}
	return payload.Bytes(), nil
}

func setInt32LE(b []byte, pos int, v int32) {
	binary.LittleEndian.PutUint32(b[pos:pos+4], uint32(v))
}

func writeHostAddress(w *bytes.Buffer, host string, port uint16) error {
	var tmp [2]byte
	binary.LittleEndian.PutUint16(tmp[:], port)
	w.Write(tmp[:])
	return writeVarString(w, host, 256)
}

func writeVarString(w *bytes.Buffer, s string, maxLen int) error {
	b := []byte(s)
	if len(b) > maxLen {
		return fmt.Errorf("string too long")
	}
	if err := writeVarInt(w, len(b)); err != nil {
		return err
	}
	w.Write(b)
	return nil
}

func writeVarInt(w *bytes.Buffer, value int) error {
	if value < 0 {
		return fmt.Errorf("negative varint")
	}
	for (value & 0xFFFFFF80) != 0 {
		w.WriteByte(byte(value&0x7F | 0x80))
		value >>= 7
	}
	w.WriteByte(byte(value))
	return nil
}

type hostAddress struct {
	Host string
	Port uint16
}

type connectPayloadInfo struct {
	protocolHash         string
	protocolCrc          int32
	protocolBuildNumber  int32
	clientVersion        string
	clientType           uint8
	uuid                 string
	language             string
	identityTokenPresent bool
	username             string
	referralDataLen      int
	referralSource       *hostAddress
}

func decodeConnectPayload(payload []byte) (connectPayloadInfo, bool) {
	if len(payload) >= 102 && looksLikeHexFixedASCII(payload, 1, 64) {
		return decodeConnectPayloadV1(payload)
	}
	if info, ok := decodeConnectPayloadV2(payload); ok {
		return info, true
	}
	return decodeConnectPayloadV1(payload)
}

func looksLikeHexFixedASCII(b []byte, start int, length int) bool {
	if start < 0 || length <= 0 || start+length > len(b) {
		return false
	}
	for _, c := range b[start : start+length] {
		if c == 0 {
			continue
		}
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}

func decodeConnectPayloadV1(payload []byte) (connectPayloadInfo, bool) {
	if len(payload) < 102 {
		return connectPayloadInfo{}, false
	}

	nullBits := payload[0]

	info := connectPayloadInfo{}
	info.protocolHash = readFixedASCII(payload, 1, 64)
	info.clientType = payload[65]

	msb := binary.BigEndian.Uint64(payload[66:74])
	lsb := binary.BigEndian.Uint64(payload[74:82])
	info.uuid = formatUUID(msb, lsb)

	languageOffset := int(int32(binary.LittleEndian.Uint32(payload[82:86])))
	identityOffset := int(int32(binary.LittleEndian.Uint32(payload[86:90])))
	usernameOffset := int(int32(binary.LittleEndian.Uint32(payload[90:94])))
	referralDataOffset := int(int32(binary.LittleEndian.Uint32(payload[94:98])))
	referralSourceOffset := int(int32(binary.LittleEndian.Uint32(payload[98:102])))

	if (nullBits&0x01) != 0 && languageOffset >= 0 {
		pos := 102 + languageOffset
		if s, _, ok := readVarString(payload, pos, 128); ok {
			info.language = s
		}
	}

	if (nullBits&0x02) != 0 && identityOffset >= 0 {
		pos := 102 + identityOffset
		_, _, ok := readVarString(payload, pos, 8192)
		info.identityTokenPresent = ok
	}

	if usernameOffset >= 0 {
		pos := 102 + usernameOffset
		if s, _, ok := readVarString(payload, pos, 16); ok {
			info.username = s
		}
	}

	if (nullBits&0x04) != 0 && referralDataOffset >= 0 {
		pos := 102 + referralDataOffset
		l, lsz, ok := readVarInt(payload, pos)
		if ok && l >= 0 && l <= 4096 {
			start := pos + lsz
			end := start + l
			if start >= 0 && end <= len(payload) {
				info.referralDataLen = l
			}
		}
	}

	if (nullBits&0x08) != 0 && referralSourceOffset >= 0 {
		pos := 102 + referralSourceOffset
		if ha, _, ok := decodeHostAddress(payload, pos); ok {
			info.referralSource = &ha
		}
	}

	return info, true
}

func decodeConnectPayloadV2(payload []byte) (connectPayloadInfo, bool) {
	if len(payload) < 66 {
		return connectPayloadInfo{}, false
	}

	nullBits := payload[0]

	info := connectPayloadInfo{}
	info.protocolCrc = int32(binary.LittleEndian.Uint32(payload[1:5]))
	info.protocolBuildNumber = int32(binary.LittleEndian.Uint32(payload[5:9]))
	info.clientVersion = readFixedASCII(payload, 9, 20)
	info.protocolHash = info.clientVersion
	info.clientType = payload[29]

	msb := binary.BigEndian.Uint64(payload[30:38])
	lsb := binary.BigEndian.Uint64(payload[38:46])
	info.uuid = formatUUID(msb, lsb)

	usernameOffset := int(int32(binary.LittleEndian.Uint32(payload[46:50])))
	identityOffset := int(int32(binary.LittleEndian.Uint32(payload[50:54])))
	languageOffset := int(int32(binary.LittleEndian.Uint32(payload[54:58])))
	referralDataOffset := int(int32(binary.LittleEndian.Uint32(payload[58:62])))
	referralSourceOffset := int(int32(binary.LittleEndian.Uint32(payload[62:66])))

	if usernameOffset < 0 {
		return connectPayloadInfo{}, false
	}
	if s, _, ok := readVarString(payload, 66+usernameOffset, 16); ok {
		info.username = s
	} else {
		return connectPayloadInfo{}, false
	}

	if (nullBits&0x01) != 0 && identityOffset >= 0 {
		pos := 66 + identityOffset
		_, _, ok := readVarString(payload, pos, 8192)
		info.identityTokenPresent = ok
	}

	if languageOffset < 0 {
		return connectPayloadInfo{}, false
	}
	if s, _, ok := readVarString(payload, 66+languageOffset, 16); ok {
		info.language = s
	} else {
		return connectPayloadInfo{}, false
	}

	if (nullBits&0x02) != 0 && referralDataOffset >= 0 {
		pos := 66 + referralDataOffset
		l, lsz, ok := readVarInt(payload, pos)
		if ok && l >= 0 && l <= 4096 {
			start := pos + lsz
			end := start + l
			if start >= 0 && end <= len(payload) {
				info.referralDataLen = l
			}
		}
	}

	if (nullBits&0x04) != 0 && referralSourceOffset >= 0 {
		pos := 66 + referralSourceOffset
		if ha, _, ok := decodeHostAddress(payload, pos); ok {
			info.referralSource = &ha
		}
	}

	return info, true
}

func readFixedASCII(b []byte, start int, length int) string {
	if start < 0 || length < 0 || start+length > len(b) {
		return ""
	}
	slice := b[start : start+length]
	end := 0
	for end < len(slice) && slice[end] != 0 {
		end++
	}
	return string(slice[:end])
}

func readVarInt(b []byte, pos int) (value int, size int, ok bool) {
	if pos < 0 || pos >= len(b) {
		return 0, 0, false
	}

	shift := 0
	for i := 0; i < 5; i++ {
		if pos+i >= len(b) {
			return 0, 0, false
		}
		by := b[pos+i]
		value |= int(by&0x7f) << shift
		size++
		if (by & 0x80) == 0 {
			return value, size, true
		}
		shift += 7
	}
	return 0, 0, false
}

func readVarString(b []byte, pos int, maxLen int) (s string, size int, ok bool) {
	l, lsz, ok := readVarInt(b, pos)
	if !ok || l < 0 || l > maxLen {
		return "", 0, false
	}
	start := pos + lsz
	end := start + l
	if start < 0 || end > len(b) {
		return "", 0, false
	}
	return string(b[start:end]), lsz + l, true
}

func decodeHostAddress(b []byte, pos int) (hostAddress, int, bool) {
	if pos < 0 || pos+2 > len(b) {
		return hostAddress{}, 0, false
	}
	port := binary.LittleEndian.Uint16(b[pos : pos+2])
	host, hostSize, ok := readVarString(b, pos+2, 256)
	if !ok {
		return hostAddress{}, 0, false
	}
	return hostAddress{Host: host, Port: port}, 2 + hostSize, true
}

func formatUUID(msb uint64, lsb uint64) string {
	var b [16]byte
	binary.BigEndian.PutUint64(b[0:8], msb)
	binary.BigEndian.PutUint64(b[8:16], lsb)
	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3],
		b[4], b[5],
		b[6], b[7],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15],
	)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func packetName(id int32) string {
	switch id {
	case 0:
		return "Connect"
	case 1:
		return "Disconnect"
	case 2:
		return "Ping"
	case 3:
		return "Pong"
	case 14:
		return "ConnectAccept"
	case 18:
		return "ClientReferral"
	default:
		return "unknown"
	}
}
