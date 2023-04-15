package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	ttmpl "text/template"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
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

func funcMap(pt *ttmpl.Template, dp *DocProps) map[string]interface{} {
	return map[string]interface{}{
		"doCmd": func(cmd string, params ...string) string {
			return runCmdMergedOutput(dp.Vars, cmd, params...)
		},
		// TODO: add a `doMarkdown` CMD
		// NOTE: tmplName == document src path, relative to document root
		"doTmpl": func(tmplName string, data interface{}) (string, error) {
			var buf bytes.Buffer
			err := pt.ExecuteTemplate(&buf, tmplName, data)
			if err != nil {
				return "", err
			}

			// content-specific post-processing
			ext := strings.ToLower(filepath.Ext(tmplName))
			switch ext {
			case ".md":
				md := goldmark.New(
					goldmark.WithExtensions(
						extension.GFM,
						extension.Typographer,
						extension.Table,
					),
					goldmark.WithParserOptions(
						parser.WithAutoHeadingID(),
					),
					goldmark.WithRendererOptions(
						html.WithUnsafe(),
						html.WithXHTML(),
					),
				)
				var bufMd bytes.Buffer
				if err = md.Convert(buf.Bytes(), &bufMd); err != nil {
					return "", err
				}
				return bufMd.String(), nil
			default:
				return buf.String(), nil
			}
		},
	}
}

func textTemplate(name string, dp *DocProps) *ttmpl.Template {
	// NOTE: all funcs need to exist at Parse(),
	//       but funcs are re-bound after Parse(), with data.
	return ttmpl.New(name).
		Delims(delimOvr(dp.Vars)).
		Funcs(funcMap(nil, nil)).
		Option("missingkey=zero")
}
