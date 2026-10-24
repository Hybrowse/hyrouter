# Hytale 0.7.4 protocol fixtures

These synthetic packets were serialized by the supplied release 0.7.4 server JAR
(protocol 3, build 258, CRC -386561135). They contain an invented UUID, username,
unsigned token and referral data. They contain no account credentials or game assets.

The server JAR is not included. With Java 25+ and a locally obtained matching JAR,
regenerate the binary files with:

```
java --class-path /path/to/HytaleServer.jar testdata/generate/ProtocolFixture.java connect.bin referral.bin
```

Convert the files to one-line hexadecimal with `xxd -p -c 10000`.
The unsigned identity claims test routing metadata only. The backend server must
perform real authentication.
