documentation:
	- document builtin escapers
	- html source example
	- header documentation in `-help` flag

features:
	- SCRIPTS var
	- windows testing
	- generate index
	- markdown option flags
	- "l/r delim var options" in doc\_props.go

| func                           | funcmap key |
| ----                           | ----        |
| text/template.JSEscapeString   | js          |
| text/template.HTMLEscapeString | html        |
| text/template.URLQueryEscaper  | urlquery    |
