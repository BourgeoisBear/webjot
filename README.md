Another static site generator, and an opinionated fork of https://github.com/zserge/zs.

## Features

* embedded HTTP server
* os-based file watching & live-reload
* golang template expansion (https://docs.gomplate.ca/syntax/)
* markdown processing (via https://github.com/yuin/goldmark)
* CSS preprocessing (via https://github.com/yosssi/gcss)

## Installation

`go install github.com/BourgeoisBear/zs@latest`

## Usage

| Action                               | Command                        |
| ------                               | -------                        |
| create new site                      | `zs -init <new_site_path>`     |
| re-build site                        | `zs <site_source_path>`        |
| update site contents w/ live refresh | `zs -watch <site_source_path>` |

Keep your texts in markdown or HTML format, right in the main directory of your blog/site.

Keep all service files (extensions, layout pages, deployment scripts etc) in the `.zs` subdirectory.

Define variables in the header of the content files using [YAML]:

	title: My web site
	keywords: best website, hello, world
	---

	Markdown text goes after a header *separator*

Use placeholders for variables and plugins in your markdown or html files, e.g. `{{ title }}` or `{{ command arg1 arg2 }}.

Write extensions in any language you like and put them into the `.zs` subdirectory.

Everything the extensions prints to stdout becomes the value of the placeholder.

Every variable from the content header will be passed via environment variables like `title` becomes `$ZS_TITLE` and so on. There are some special variables:

* `$ZS` - a path to the `zs` executable
* `$ZS_OUTDIR` - a path to the directory with generated files
* `$ZS_FILE` - a path to the currently processed markdown file
* `$ZS_URL` - a URL for the currently generated page

**NOTE**: To ensure that live-refresh scripts are excluded from your final pages, be sure to re-build **without** the `-watch` flag prior to publication.

## Flags

```
Usage of zs:
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

## RSS Generation Example

Extensions can be written in any language you know (Bash, Python, Lua, JavaScript, Go, even Assembler). Here's an example of how to scan all markdown blog posts and create RSS items:

``` bash
for f in ./blog/*.md ; do
	d=$($ZS var $f date)
	if [ ! -z $d ] ; then
		timestamp=`date --date "$d" +%s`
		url=`$ZS var $f url`
		title=`$ZS var $f title | tr A-Z a-z`
		descr=`$ZS var $f description`
		echo $timestamp \
			"<item>" \
			"<title>$title</title>" \
			"<link>http://zserge.com/$url</link>" \
			"<description>$descr</description>" \
			"<pubDate>$(date --date @$timestamp -R)</pubDate>" \
			"<guid>http://zserge.com/$url</guid>" \
		"</item>"
	fi
done | sort -r -n | cut -d' ' -f2-
```
