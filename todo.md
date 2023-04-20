documentation:
	- html source example
	- document builtin escapers
	- document 'skip' var
	- document doTmpl . vs. nil
	- header documentation in `-help` flag

features:
	- recursion depth catcher
	- move .webjot to top of PATH for doCmd
			TODO: ensure proper path separators
	- YAML-parse envvars
	- source code highlighting
	- separate modules?
	- test mixed delimiters
	- SCRIPTS var
	- raw, non-html/md, template expansion
	- markdown option flags
	- "l/r delim var options"

	- ignore globals-in, YAML-encode globals-out

	- fetch YAML/JSON template funcs
			to/fromJSON
			to/fromYAML

| func                           | funcmap key |
| ----                           | ----        |
| text/template.JSEscapeString   | js          |
| text/template.HTMLEscapeString | html        |
| text/template.URLQueryEscaper  | urlquery    |
