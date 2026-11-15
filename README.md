# spring-inicializador

**Spring Initializr in your terminal.**

A small Go binary that talks to the same public API as [start.spring.io](https://start.spring.io):
pick your build tool, language, Java version and dependencies, get the generated archive, and read the
command that starts the project.

## What it is, and what it is not

The web UI is not going anywhere, and this project does not try to replace it. It is another client of
the same public API, aimed at people who already live in a terminal.

It asks the server for Java, Kotlin or Groovy projects. The tool itself is a single Go binary: no JVM,
no runtime, no `npx` latency.

## Status

**Pre-alpha.** The project form works. Run the binary on a terminal and it asks for group, artifact,
build, language, packaging and versions, offering only what the server publishes; or pass flags and they
answer the rows they cover, leaving the form to ask the rest. Choosing dependencies still means knowing
their IDs — the searchable pane is the next slice.

| Slice | What it adds | State |
| --- | --- | --- |
| VS0 | Module, metadata parser, offline fixture | done |
| VS1 | Generate and extract a project from flags | done |
| VS2 | Project form with flag parity | done |
| VS3 | Searchable dependency list | next |
| VS4 | Category level in the dependency pane | planned |
| VS5 | Detail panel and terminal theming | planned |
| VS6 | Review, run and non-interactive contract | planned |

## Install

Requires Go 1.27 or newer.

```bash
go install github.com/miferdev/spring-inicializador@latest
```

Or build it from a clone:

```bash
git clone https://github.com/miferdev/spring-inicializador
cd spring-inicializador
go build -o spring-inicializador ./cmd/spring-inicializador
```

Generating a project needs network access to the Initializr API.

## Usage

On a terminal it opens the project form: `tab` moves on, `esc` goes back a field, and `esc` on the first
field leaves without writing anything. Flags answer fields instead of being a separate mode — pass two
and the form asks only for the rest. Redirecting stdin or passing `--yes` skips the form entirely, which
is what makes the same binary usable in a pipeline.

```console
$ spring-inicializador --deps web,data-jpa --output ~/projects --yes
Created /home/you/projects/demo
Next: cd /home/you/projects/demo && ./mvnw spring-boot:run
```

| Flag | Meaning |
| --- | --- |
| `--deps` | Comma-separated dependency IDs, e.g. `web,data-jpa,postgresql` |
| `--output` | Directory that receives the project (default `.`) |
| `--group-id`, `--artifact-id` | Project coordinates; the package name follows them |
| `--name`, `--package-name` | Override the derived name and package |
| `--build` | `maven` (default), `gradle`, `gradle-kotlin` |
| `--language`, `--packaging` | `java`/`kotlin`/`groovy`, `jar`/`war` — rejected before any request |
| `--java`, `--boot-version` | Version ids, validated against what the server publishes |
| `--dry-run` | Print the request it would send and exit without generating |
| `--server` | Initializr base URL for self-hosted instances (default `start.spring.io`) |
| `--force` | Replace a destination that already has content |
| `--yes` | Never prompt, accept every default |
| `--version`, `--help` | |

Exit codes: `0` success, `1` server or filesystem failure, `2` usage error, `130` interrupted.

The project is extracted into `<output>/<artifact-id>`, with the permission bits the server stored — so
`mvnw` and `gradlew` arrive executable, and a failed run leaves no half-written directory behind.

Server rejections are printed verbatim. The tool does not reimplement the server's validation, so when
a version or dependency is invalid you read the same sentence the web UI would show you.

The dependency pane is the next slice. When it lands it will look roughly like this — categories and
counts come from the metadata, never from this file:

```text
+------------------------------------------------------------------------------+
|  spring-inicializador   Dependencies                         / search all    |
+------------------------------------------------------------------------------+
|  > AI                                                                   (50) |
|    SQL                                                                  (19) |
|    Web                                                                  (18) |
|    NoSQL                                                                (15) |
|    Messaging                                                            (12) |
|    I/O                                                                  (12) |
|    Observability                                                        (11) |
|    Security                                                              (9) |
|    Developer Tools                                                       (7) |
|    Ops                                                                   (6) |
|    ... 23 categories                                                         |
+------------------------------------------------------------------------------+
|  enter open category   / fuzzy find all 206   space toggle   ? help           |
+------------------------------------------------------------------------------+
```

## Design commitments

- **Nothing is hardcoded.** Build options, Java versions and the dependency catalog come from
  `GET /metadata/client`. At the time of writing that is 23 categories and 206 dependencies; the tool
  never ships a list of its own.
- **Every prompt has a flag.** The interactive interface is a convenience. If a step cannot be skipped
  from a script, it is a bug.
- **One direction.** Flags and the form both fill the same options value, which is handed to the
  generator. There is no second code path.

## How it works

```
form  ─┐
       ├─→ options ─→ POST /starter.zip ─→ archive ─→ extract ─→ next steps
flags ─┘
```

The metadata catalog feeds the form; the generator only sees the resolved options and returns bytes.
Extraction takes bytes and a destination and touches nothing else.

```
cmd/spring-inicializador/   command entrypoint and flags
internal/options/           the value both frontends fill
internal/catalog/           metadata fetch, parsing, fixture and index
internal/render/            POST /starter.zip
internal/extract/           unzip, permission bits, staging directory
```

## Development

```bash
go test ./...
```

No test makes an HTTP request. The metadata payload is frozen in
`internal/catalog/testdata/`, with its endpoint, capture date and SHA-256 recorded next to it, so a
catalog change shows up as a deliberate diff instead of a surprise failure; `httptest` servers on
loopback cover the two endpoints.

Third-party modules are not vendored, so a fresh clone downloads `go.sum` once and runs offline after
that. Vendoring a TUI library would put third-party code in every review for something the module cache
already provides.

## Feedback

Issues are welcome. There is no support timeline — this is a spare-time project.

## License

Apache-2.0. See [LICENSE](LICENSE).
