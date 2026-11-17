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

**Feature-complete for its own plan; still pre-1.0.** The whole flow works: project form, dependency pane
with categories and search, a detail region, a review that names every decision before anything is
written, and a report that says what happened. Everything on offer comes from the server's catalog.
There is no metadata cache yet, so every run fetches it.

| Slice | What it adds | State |
| --- | --- | --- |
| VS0 | Module, metadata parser, offline fixture | done |
| VS1 | Generate and extract a project from flags | done |
| VS2 | Project form with flag parity | done |
| VS3 | Searchable dependency list | done |
| VS4 | Category level in the dependency pane | done |
| VS5 | Detail panel and terminal theming | done |
| VS6 | Review, run and non-interactive contract | done |
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

On a terminal it opens the wizard. The form asks the project fields in four sections — Project, Details,
Build, Versions — one per screen, with `enter` moving on and `esc` going back. `Project` asks what the
project is called and where it should be created; the name alone decides the artifact id, the package
prefix and the folder. `esc` on the first field leaves without writing anything. Then the dependencies — the
categories with their counts, `enter` to open one, `/` to search all 206 from any level, `space` to
select, `g` to open the focused dependency's guide, `esc` to back out a level and `ctrl+d` to finish. From
120 columns the focused dependency's description and guides sit beside the list; between 100 and 119 they
collapse to a single line under the row. Flags answer fields instead of being a separate mode: pass two
and the wizard asks only for the rest. The wizard only opens when both stdin and stdout are terminals, so
piping the output gives you a plain log instead of an interface drawn into a file.

A run reports itself, one line per step, and stops at the first failure with the server's own message:

```console
$ spring-inicializador --deps web,data-jpa --output ~/projects --yes
Catalog ........ ok   (206 dependencies)
Generating ..... ok   (15 KB)
Extracting ..... ok   (24 files)
Permissions .... ok   (mvnw)
Created /home/you/projects/demo
Next: cd /home/you/projects/demo && ./mvnw spring-boot:run
```

| Flag | Meaning |
| --- | --- |
| `--deps` | Dependency IDs, for non-interactive runs; the list is the normal way |
| `--output` | Parent directory; the project is created as `<output>/<name>` (default `.`) |
| `--name` | Project name: the artifact id, the package prefix and the folder |
| `--artifact-id` | Alias of `--name`; `--name` wins when both are given |
| `--group-id` | Group id; with the name it derives the package |
| `--package-name` | Override the derived package |
| `--build` | `maven` (default), `gradle`, `gradle-kotlin` |
| `--language`, `--packaging` | `java`/`kotlin`/`groovy`, `jar`/`war` — rejected before any request |
| `--java`, `--boot-version` | Version ids, validated against what the server publishes |
| `--dry-run` | Print the request it would send and exit without generating |
| `--server` | Initializr base URL for self-hosted instances (default `start.spring.io`) |
| `--force` | Replace a destination that already has content |
| `--yes` | Answer every prompt with its default; never destructive |
| `--no` | Answer every prompt with "no": never replace an output directory, even with `--force` |
| `--fancy` | Unicode borders instead of ASCII |
| `--no-color` | Strip every style; `NO_COLOR` in the environment does the same |
| `--version`, `--help` | |

Exit codes: `0` success, `1` server or filesystem failure, `2` usage error, `130` interrupted.

The project is extracted into `<output>/<artifact-id>`, with the permission bits the server stored — so
`mvnw` and `gradlew` arrive executable, and a failed run leaves no half-written directory behind.

Server rejections are printed verbatim. The tool does not reimplement the server's validation, so when
a version or dependency is invalid you read the same sentence the web UI would show you.

The dependency pane opens on the categories and looks roughly like this — counts and names come from the
metadata, never from this file:

```text
23 categories  206 dependencies
> AI                                                                    (50)
  SQL                                                                   (19)
  Web                                                                   (18)
  NoSQL                                                                 (15)
  Messaging                                                             (12)
  I/O                                                                   (12)
  Observability                                                         (11)
  Security                                                               (9)
  Developer Tools                                                        (7)
  Ops                                                                    (6)
  ...
2 selected: web, data-jpa
enter open  / find  esc back  ctrl+d finish
```

Inside a category the header turns into `< Web (18)`, and searching turns it into `Search: "postgres"`.
The same `/` searches all 206 from either level, because a search that silently narrowed would hide
results you cannot see you are missing.

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
