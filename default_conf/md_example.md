title: Markdown Source Example
author: Jason Stewart
@@@@@@@

## Templating Syntax

Uses golang's `text/template` syntax, documented at:

* https://pkg.go.dev/text/template
* https://docs.gomplate.ca/syntax/
* https://blog.gopheracademy.com/advent-2017/using-go-templates/

**NOTE**: Default delimiters are escaped thus:

{{"{{"}} "{{"{{"}}" {{"}}"}}

{{"{{"}} "{{"}}"}}" {{"}}"}}

## Template Variables

**NOTE**: Field keys are case-sensitive!

**NOTE**: Uppercase field keys are auto-generated *built-ins*.

| Field          |        | Value            |
| -------------- | ------ | ---------------- |
{{ range (.GetPairs true) -}}
| **{{ .K }}** | | {{ .V }} |
{{ end -}}


## External Commands

### How does my command access template variables?

The output of external commands can be captured with the `doCmd` template
function.  Template variables are passed to these commands as environment
variables:

```
{{ doCmd "sh" "-c" "env | grep ^ZS_" }}
```

### What are the contents of my `$HOME` directory?

Even though environment variables are passed to the command, this command
should fail, since evironment variables are not expanded inside the command
string:

```
{{ doCmd "ls" "-lt" "$HOME" }}
```

To expand environment variables in the command string, or use features like
Unix pipes and I/O redirection, run the command inside a shell:

```
{{ doCmd "sh" "-c" "ls -lt $HOME" }}
```
