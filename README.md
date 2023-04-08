# webjot

Another static site generator, and an opinionated fork of https://github.com/zserge/zs.

## Features

* embedded HTTP server
* os-based file watching & live-reload
* golang template expansion (https://docs.gomplate.ca/syntax/)
* markdown processing (via https://github.com/yuin/goldmark)
* CSS preprocessing (via https://github.com/yosssi/gcss)

## Installation

```
go install github.com/BourgeoisBear/webjot@latest
```

## Usage

| Action                               | Command                            |
| ------                               | -------                            |
| create new site                      | `webjot -init <new_site_path>`     |
| re-build site                        | `webjot <site_source_path>`        |
| update site contents w/ live refresh | `webjot -watch <site_source_path>` |

Keep your texts in markdown or HTML format, right in the main directory of your blog/site.  Keep all service files (extensions, layout pages, deployment scripts etc) in the `.webjot` subdirectory.

Define variables in the header of the content files using:

```
title: My web site
keywords: best website, hello, world
---

# {{ .title }}
Markdown text goes after a header *separator*
```

Use golang `text/template` syntax to access header variables and plugins in your markdown or html files, e.g. `{{ .title }}` or `{{ command arg1 arg2 }}`.

Write extensions in any language you like and put them into the `.webjot` subdirectory.  Everything the extensions prints to stdout becomes the value of the placeholder.

Every variable from the content header will be passed via environment variables like `title` becomes `$ZS_TITLE` and so on. There are some special variables:

| Env Var      | Description                                     |
| -------      | -----------                                     |
| `$ZS`        | a path to the `webjot` executable               |
| `$ZS_OUTDIR` | a path to the directory with generated files    |
| `$ZS_FILE`   | a path to the currently processed markdown file |
| `$ZS_URL`    | a URL for the currently generated page          |

To build your site:

```
webjot ./path_to_sources
```

The site will be rendered to the `./path_to_sources/.pub` subdirectory.

For live-reloading during site development, try `webjot` in *watch mode*:

```
webjot -watch ./path_to_sources
```

A browser should open to your `.pub` subdirectory.  Each file will live-reload as you make changes to its source.

**NOTE**: To ensure that live-refresh scripts are excluded from your final pages, be sure to re-build **without** the `-watch` flag prior to publication.

## Flags

```
Usage of webjot:
  -init
        create a new site configuration inside the given directory
  -ldelim string
        left template delimiter (default "{{")
  -port int
        HTTP port for watch-mode web server (default 8080)
  -rdelim string
        right template delimiter (default "}}")
  -vdelim string
        vars/body delimiter (default "---")
  -vshow
        show per-page render vars on build
  -watch
        rebuild on file change
```

