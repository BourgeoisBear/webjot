documentation:
	- layouts
	- html source example
	- document builtin escapers
	- header documentation in `-help` flag

features:
	- SCRIPTS var
	- raw, non-html/md, template expansion
	- markdown option flags
	- "l/r delim var options"

| func                           | funcmap key |
| ----                           | ----        |
| text/template.JSEscapeString   | js          |
| text/template.HTMLEscapeString | html        |
| text/template.URLQueryEscaper  | urlquery    |
