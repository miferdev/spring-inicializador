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

**Pre-alpha.** The only thing that works today, and the only thing with tests, is the metadata parser —
it reads the Initializr catalog offline from a frozen fixture. There is no generation, no extraction and
no interface yet.

| Slice | What it adds | State |
| --- | --- | --- |
| VS0 | Module, metadata parser, offline fixture | done |
| VS1 | Generate and extract a project from flags | next |
| VS2 | Project form with flag parity | planned |
| VS3 | Searchable dependency list | planned |
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

Generating a project will need network access to the Initializr API.

## Usage

Not usable yet. Today the binary only reports its version:

```console
$ spring-inicializador
spring-inicializador dev
```

The interface is being built slice by slice. When the dependency pane exists it will look roughly like
this — categories and counts come from the metadata, never from this file:

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
cmd/spring-inicializador/   command entrypoint
internal/catalog/           metadata parsing, fixture and index
```

## Development

```bash
go test ./...
```

The suite runs with the network unplugged. The metadata payload is frozen in
`internal/catalog/testdata/`, with its endpoint, capture date and SHA-256 recorded next to it, so a
catalog change shows up as a deliberate diff instead of a surprise failure.

## Feedback

Issues are welcome. There is no support timeline — this is a spare-time project.

## License

Apache-2.0. See [LICENSE](LICENSE).
