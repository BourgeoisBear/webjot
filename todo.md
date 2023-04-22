documentation:
	- html source example
	- document builtin escapers
	- document doTmpl . vs. nil
	- header documentation in `-help` flag

features:
	- doCmd / $PATH interactions
	- recursion depth catcher
	- YAML-parse envvars
		- ignore globals-in, YAML-encode globals-out?
	- source code highlighting
	- separate modules?
	- test mixed delimiters
	- raw, non-html/md, template expansion?
	- markdown option flags (per file, or global?)
	- "l/r delim var options"

| func                           | funcmap key |
| ----                           | ----        |
| text/template.JSEscapeString   | js          |
| text/template.HTMLEscapeString | html        |
| text/template.URLQueryEscaper  | urlquery    |
