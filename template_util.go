package main

import (
	"bytes"
	"fmt"
	"io"
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

func funcMap(
	pt *ttmpl.Template,
	dp *DocProps,
	allDocs []DocVar,
) map[string]interface{} {
	return map[string]interface{}{
		"md2html": func(md string) (string, error) {
			return Md2Html([]byte(md))
		},
		"doCmd": func(cmd string, params ...string) string {
			return runCmdMergedOutput(dp.Vars, cmd, params...)
		},
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
				return Md2Html(buf.Bytes())
			}

			return buf.String(), nil
		},
		"allDocs": func() []DocVar {
			return allDocs
		},
	}
}

func FromMarkdown(dst io.Writer, src []byte) error {
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
	return md.Convert(src, dst)
}

func Md2Html(md []byte) (string, error) {
	var bufHtml bytes.Buffer
	if err := FromMarkdown(&bufHtml, md); err != nil {
		return "", err
	}
	return bufHtml.String(), nil
}

func NewTemplate(tmplName string, dl Delims) *ttmpl.Template {
	// NOTE: all funcs need to exist at Parse(),
	//       but funcs are re-bound after Parse(), with data.
	return ttmpl.New(tmplName).
		Delims(dl.L, dl.R).
		Funcs(funcMap(nil, nil, nil)).
		Option("missingkey=zero")
}
