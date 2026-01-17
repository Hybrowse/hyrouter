# Referral envelope

Hyrouter forwards backend routing information to the Hytale client via `ClientReferral` (packet ID `18`).

The packet contains a `referral_data` blob.

Hyrouter always sets `referral_data` to a fixed, router-controlled envelope (even if the envelope content is empty), so that:

- The format can be versioned for future changes.
- Plugins can only mutate the envelope *content*, not the envelope itself.
- Backends can verify the envelope using a shared secret (HMAC).

## Envelope format (v1)

All integers are little-endian.

```
0..3   magic        ASCII "HYRP"
4      version      uint8  = 1
5      flags        uint8
6      key_id       uint8
7..8   content_len  uint16
9..    content      content_len bytes
..     hmac         32 bytes (optional; present if flags bit0 is set)
```

Flags:

- `0x01`: signed with HMAC-SHA256

Size limits:

- The entire envelope must fit into the Hytale `ClientReferral.referral_data` limit (currently 4096 bytes).

## Signing (HMAC-SHA256)

If signing is enabled, Hyrouter appends a 32-byte HMAC:

- Algorithm: HMAC-SHA256
- MAC input: all envelope bytes *up to and including* `content` (i.e. header + content)

The backend can reject connections if the HMAC is missing (when required) or invalid.

## Backend verification

Backends should parse/verify the envelope directly using the format described above.

Verification steps:

1. Parse header and `content_len`.
2. If `flags & 0x01 == 0`, treat the envelope as unsigned.
3. If signed, compute `HMAC-SHA256(secret, header+content)` and compare against the 32-byte MAC at the end.

Secret decoding follows the same scheme as Hyrouter config:

- raw string (UTF-8 bytes)
- `base64:<...>`
- `hex:<...>`

## Plugin interaction

Plugins never receive the raw envelope.

- Plugins read/write `referral_content`.
- Hyrouter wraps `referral_content` into the envelope and (optionally) signs it.
