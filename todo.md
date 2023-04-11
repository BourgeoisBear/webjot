documentation:
	- layouts
	- html source example
	- document builtin escapers
	- header documentation in `-help` flag

features:
	- generate index
		- pre-render each (non-layout) document to .tmp
		- cache render data for each page
		- apply layout to each document, passing in cached render data as _PAGES
		- disambiguate between _SITE_PAGES and _LAYOUT_PAGES
	- SCRIPTS var
	- raw, non-html/md, template expansion
	- markdown option flags
	- "l/r delim var options" in doc\_props.go

| func                           | funcmap key |
| ----                           | ----        |
| text/template.JSEscapeString   | js          |
| text/template.HTMLEscapeString | html        |
| text/template.URLQueryEscaper  | urlquery    |
