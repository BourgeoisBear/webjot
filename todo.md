features:
	- generate index
	- markdown option flags

correctness:
	- invalid header checking/reporting ( non comment, no key, not envvar compatible [A-Z][a-z]_ ) on parse

documentation:
	- cmdRaw / cmdHtmlEncoded template funcs
	- header documentation in `-help` flag
	- better example site

default site:
	- direct recursive copy of `default_conf`
	- 'created by' message at bottom of default layout

