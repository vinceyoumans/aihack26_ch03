# Media Probe

End-to-end test tool for the two media RPCs against a live TellCartman server.

- **StreamAudio** — client-streams a synthetic ~48KB clip in 32KB chunks; the
  server reassembles and uploads to `audio/{patient}/{record}.opus`
- **RequestPhotoUpload** — gets a presigned S3 URL, then HTTP PUTs a fake JPEG
  directly to `photos/{patient}/{type}/{record}.jpg` (binary never touches the
  gRPC server)

## Usage

```bash
cd server
go run ./cmd/media-probe --server 18.218.64.156:50051
```

Prints the S3 keys returned by the server. Verify the objects with:

```bash
aws s3 ls s3://tellcartman-raw-dev-<ACCOUNT_ID>/audio/  --recursive
aws s3 ls s3://tellcartman-raw-dev-<ACCOUNT_ID>/photos/ --recursive
```

## Verified behavior (2026-10-17, live server)

| RPC | Result |
|---|---|
| StreamAudio | 2 chunks → 49,152 B `.opus`, content-type `audio/ogg; codecs=opus`, AES256 |
| RequestPhotoUpload | presigned PUT → 65,540 B `.jpg`, content-type `image/jpeg`, AES256 |
