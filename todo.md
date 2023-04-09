features:
	- different prefix for env vars
	- generate index
	- markdown option flags

correctness:
	- invalid header reporting ( non comment, no key )

documentation:
	- vars handling / precedence
	- cmdRaw / cmdHtmlEncoded template funcs
	- header documentation in `-help` flag
	- better example site

default site:
	- direct recursive copy of `default_conf`
	- 'created by' message at bottom of default layout

