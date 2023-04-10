package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"os/exec"
	"strings"
	ttmpl "text/template"
	"unicode"
)

func HasUcase(s string) bool {
	for _, c := range s {
		if unicode.IsUpper(c) {
			return true
		}
	}
	return false
}

/*
run executes a command or a script. Vars define the command environment,
each var is converted into OS environemnt variable with ENVVAR_PREFIX
prepended.
*/
func runCmd(mV Vars, cmd string, args ...string) (sout, serr []byte, err error) {

	c := exec.Command(cmd, args...)

	// write user-defined vars first, built-in vars last,
	// so that built-ins take precedence
	c.Env = os.Environ()
	for k := range mV {
		if !HasUcase(k) {
			v := mV.GetStr(k)
			c.Env = append(c.Env, ENVVAR_PREFIX+strings.ToUpper(k)+"="+v)
		}
	}
	for k := range mV {
		if HasUcase(k) {
			v := mV.GetStr(k)
			c.Env = append(c.Env, ENVVAR_PREFIX+strings.ToUpper(k)+"="+v)
		}
	}

	var errbuf, outbuf bytes.Buffer
	c.Stdout = &outbuf
	c.Stderr = &errbuf

	err = c.Run()
	return outbuf.Bytes(), errbuf.Bytes(), err
}

func runCmdMergedOutput(mV Vars, cmd string, args ...string) string {
	so, se, err := runCmd(mV, cmd, args...)

	parts := make([]string, 0, 3)
	if err != nil {
		cmdstr := cmd + " " + strings.Join(args, " ")
		parts = append(parts, fmt.Sprintf("CMD ERROR on `%s`: %s", cmdstr, err.Error()))
	}
	if len(se) > 0 {
		parts = append(parts, string(se))
	}
	if len(so) > 0 {
		parts = append(parts, string(so))
	}
	return strings.Join(parts, "\n")
}

func funcMap(mV Vars) map[string]interface{} {
	return map[string]interface{}{
		"cmdRaw": func(cmd string, params ...string) string {
			return runCmdMergedOutput(mV, cmd, params...)
		},
		"cmdHtmlEncoded": func(cmd string, params ...string) string {
			return html.EscapeString(runCmdMergedOutput(mV, cmd, params...))
		},
	}
}

func delimOvr(mV Vars) (string, string) {
	l, r := "{{", "}}"
	ol, or := mV.GetStr("ldelim"), mV.GetStr("rdelim")
	if len(ol) > 0 {
		l = ol
	}
	if len(or) > 0 {
		r = or
	}
	return l, r
}

func textTemplate(mV Vars) *ttmpl.Template {
	return ttmpl.New("").
		Delims(delimOvr(mV)).
		Funcs(funcMap(mV)).
		Option("missingkey=zero")
}
