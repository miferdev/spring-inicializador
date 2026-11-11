# metadata-client.json

- Endpoint: `GET https://start.spring.io/metadata/client`
- Captured: 2026-11-10 UTC
- Size: 76403 bytes
- SHA-256: `5838cbfc105fb2b13aa4fd3ef2388e383ed8df2bea8b0aeb403707345065be3a`
- Shape at capture: 23 categories, 206 dependencies

## Capture command

```sh
curl -s https://start.spring.io/metadata/client -o internal/catalog/testdata/metadata-client.json
```

## Frozen fixture rule

This fixture is frozen and replaced only deliberately. Every test parses
exactly these bytes, so a refresh is a reviewable change, never a side effect of
running the suite.

When replacing it:

1. Re-run the capture command above and update the capture date, size and
   SHA-256 in this file.
2. Re-verify the shape counts (currently 23 categories, 206 dependencies) and
   update the two literals in `internal/catalog/fixture_test.go` together with
   the counts recorded here.
3. Run `go test ./internal/catalog -run 'TestFixture' -v -count=1` and review the
   observed shape counts printed by the guide/link test as part of the diff.
