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

Keep your texts in markdown or HTML format, right in the main directory of your
blog/site `<site>`.  Keep all service files (extensions, layout pages, deployment
scripts etc) in the `<site>/.webjot` subdirectory.  Site will be rendered to the
`<site>/.pub` subdirectory using Go's `text/template` syntax.

Template variables can be defined as environment variables (prefixed with
`ZS_`) prior to invocation, and at the top of each content file in YAML
format, followed by the default header terminator (`@@@@@@@`):

```md
title: My Website
keywords: best website, hello, world
@@@@@@@

# {{ .title }}

This is my website content.

```


## Templating

Use golang `text/template` syntax to access header variables and plugins in
your markdown or html files, e.g. `{{ .title }}` or `{{ command arg1 arg2 }}`.

Write extensions in any language you like and put them into the `.webjot`
subdirectory.  Everything the extensions prints to stdout becomes the value of
the placeholder.

Every variable from the content header will be passed via environment variables
like `title` becomes `$ZS_TITLE` and so on.


## Variables

Template variables may be specified, in YAML format, from an optional header
block (all text preceding `@@@@@@@`).  To preserve compatibility with
*environment variables* and *built-ins*, all top-level keys *must* be
completely lowercase and consist solely of letters `[a-z]`, numbers `[0-9]`,
and underscores `[_]`.  Any keys which do not follow this naming standard will
be discarded.

```md

title: My Markdown Document
categories: examples, help
author: Jason Stewart
WRONG: <discarded for being uppercase>
@@@@@@@

content begins here...

# {{ .title }}
### {{ .author }}

```

Webjot provides the following *built-in* / automatically-generated variables:

| Template               | Shell Environment         | Example Value                     |
| ---------------------- | ------------------------- | -------------                     |
| `{{ .SRC_ROOT }}`      | `$ZS_SRC_ROOT`            | /home/user/webjot/content         |
| `{{ .PUB_ROOT }}`      | `$ZS_PUB_ROOT`            | /home/user/webjot/content/.pub    |
| `{{ .CONF_ROOT }}`     | `$ZS_CONF_ROOT`           | /home/user/webjot/content/.webjot |
| `{{ .FNAME }}`         | `$ZS_FNAME`               | environment\_vars.md              |
| `{{ .PATH }}`          | `$ZS_PATH`                | subdir/environment\_vars.md       |
| `{{ .MODIFIED }}`      | `$ZS_MODIFIED`            | 2023-04-09T03:43:31-04:00         |
| `{{ .WATCHMODE }}`     | `$ZS_WATCHMODE`           | enabled (blank if disabled)       |

For templating purposes, built-ins are always `UPPERCASE`, and user-defined
variables are always `lowercase`.


### Delimiter Overrides

Delimiters may be overridden on a per-file basis with the `ldelim` and `rdelim` header keys:

```md

ldelim: <?
rdelim: ?>
title: Delimiter Override Example
@@@@@@@
# <? .title ?>
My markdown content...

```


### Variable Precedence

Variables may be specified globally through shell `environment variables`,
inside shared `layouts`, and inside the `document` itself.  When the same
variable name is used at different levels, the following precedence is
observed:

```
document > document's layout > environment variables
```

So if `my_var` is set to `one` in `doc.md`, `two` inside its layout, and
`three` inside `$ZS_MY_VAR`, `my_var` will be rendered as `one`.


```
TODO: layout.html documentation
	* HTML_CONTENT
	* layout: header

TODO: built-in template funcs documentation
```

**NOTE**: To ensure that live-refresh scripts are excluded from your final
pages, be sure to re-build *without* the `-watch` flag prior to publication.


## CLI Help

```
USAGE
  webjot [FLAG]... <source dir>

Static site template renderer.
Templates in <source dir> are rendered to the '<source dir>/.pub' directory.

The default delimiters '{{' and '}}' are escaped thus:

  {{ "{{" }}
  {{ "}}" }}

FLAG
  -init
        create a new site configuration inside the given directory
  -port int
        HTTP port for watch-mode web server (default 8080)
  -vdelim string
        vars/body delimiter (default "@@@@@@@")
  -vshow
        show per-page render vars on build
  -watch
        rebuild on file change

EXAMPLES
  create new site:
    webjot -init <new_site_path>

  re-build site:
    webjot <site_source_path>

  update site contents w/ live refresh:
    webjot -watch <site_source_path>
```

