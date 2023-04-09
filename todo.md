features:
	- different prefix for env vars
	- generate index
	- markdown option flags

correctness:
	- invalid header checking/reporting ( non comment, no key ) on parse
		- env vars must be alpha-numeric + underscore, cannot start with number

documentation:
	- cmdRaw / cmdHtmlEncoded template funcs
	- header documentation in `-help` flag
	- better example site

default site:
	- direct recursive copy of `default_conf`
	- 'created by' message at bottom of default layout

