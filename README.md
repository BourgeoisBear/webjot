# webjot

Another static site generator, and an opinionated fork of https://github.com/zserge/zs.

## Fork Additions

* embedded HTTP server
* os-based file watching & live-rebuild
* client-side live-reload (via https://livejs.com/)
* golang template expansion in CSS/GCSS files, in addition to HTML/XML/MD files (https://docs.gomplate.ca/syntax/)
* markdown processing (via https://github.com/yuin/goldmark)


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

Keep your texts in markdown or HTML format in the folder `<site>`. Keep all
service files (extensions, layout pages, deployment scripts...) in the
`<site>/.webjot` subdirectory.  After invoking `webjot`, your site will be
rendered to the `<site>/.pub` subdirectory using Go's `text/template` syntax.

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

Write extensions in any language you like, and render them into your templates
using the `doCmd` template func. Everything the extension writes to STDOUT &
STDERR becomes the value of the placeholder.

Every variable from the content header will be passed via environment variables
like `title` becomes `$ZS_TITLE` and so on.


## Variables

Template variables may be specified, in YAML format, from an optional header
block (all text preceding `@@@@@@@`).  To preserve compatibility with
*environment variables* and *built-ins*, all top-level keys must consist solely
of *lowercase* letters `[a-z]`, numbers `[0-9]`, underscores `[_]`, and not
begin with a number.  Any keys which do not follow this naming standard will be
discarded.

```md

title: My Markdown Document
categories: examples, help
author: Jason Stewart
WRONG: <discarded for being uppercase>
1variable: <discarded for starting with a number>
@@@@@@@

content begins here...

# {{ .title }}
### {{ .author }}

```

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


## Layouts

By default, markdown and HTML sources are rendered into a layout template (default = `<site>/.webjot/layout.html`).  Layouts can be overridden by specifiying a value for `layout` in your document header.  When `layout` is set to blank, no layout will be applied.


### Example Document

```md
title: My Example
@@@@@@@
Here it is...
```


### Example Layout

```html
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" lang="en">
  <head>
    <title>{{ html .title }}</title>
  </head>
  <body>
    <ul id="menu">
      {{ range (allDocs "title") -}}
      <li>
        <a href="/{{ .URI_PATH }}">{{ html .title }}</a>
      </li>
      {{ end -}}
    </ul>
    <article>
      {{ if .title }}<h1>{{ html .title }}</h1>{{ end }}
      {{ doTmpl .DOC_KEY . }}
    </article>
  </body>
</html>
```

`( allDocs "title" )` is a template function.  It returns an array of variable maps&mdash;one for each document in the site.  It is sorted by the document variable(s) keyed by the remaining string parameters.  The array is left unsorted when no other parameters are given.  `allDocs` can be used to render site-wide menus, sitemaps, etc.

`{{ doTmpl .DOC_KEY . }}` is where each document body will be rendered into the layout.  `.DOC_KEY` is a built-in variable.  It contains a relative path to the document source file currently being rendered.  `.` is a reference to the current document's variable map.


### Template Functions

Go's built-in `text/template` functions are defined here: https://pkg.go.dev/text/template#hdr-Functions

In addition to those, here are webjot's built-ins:

| Function  | Description |
| --------  | ----------- |
| `allDocs` | Returns an array of variable maps, one for each document in the site, sorted by the document variable(s) keyed by the remaining string parameters. |
| `doTmpl`  | Renders a template named by the 1st parameter with the vars specified in the 2nd. |
| `doCmd`   | Executes another program and returns the combined output of STDOUT & STDERR.  `<site>/.webjot` is given highest priority in `$PATH` prior to command execution.<br/>Unix piping and IO redirection must be wrapped inside an explicit shell invocation, like `{{ doCmd "sh" "-c" "env \| grep ^ZS_" }}`, since `doCmd` is a simple exec, not a subshell. |
| `md2html` | Renders markdown source in the 1st parameter to HTML. |


### Variable Precedence

Variables may be specified globally through shell environment variables,
inside shared layouts, and inside the document itself.  When the same
variable name is used at different levels, the following precedence is
observed (x > y, meaning x replaces y):

```
document > document's layout > environment variables
```

So if `my_var` is set to `one` in `doc.md`, `two` inside its layout, and
`three` inside `$ZS_MY_VAR`, `my_var` will be rendered as `one`.

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

