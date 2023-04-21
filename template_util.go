package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	tt "text/template"
	"unicode"

	"github.com/yosssi/gcss"
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

func postProcess(
	iDst io.Writer,
	tmplName string, // same as source document name
	ptmpl *tt.Template,
	data interface{},
) error {
	ext := strings.ToLower(filepath.Ext(tmplName))
	if (ext == ".md") || (ext == ".gcss") {
		// pre-render template
		buf := bytes.NewBuffer(make([]byte, 0, 64*1024))
		if err := ptmpl.Execute(buf, data); err != nil {
			return err
		}
		// post-process result
		switch ext {
		case ".md":
			return Md2HtmlWri(iDst, buf.Bytes())
		case ".gcss":
			_, err := gcss.Compile(iDst, buf)
			return err
		}
	}
	return ptmpl.Execute(iDst, data)
}

type DocsMap map[string]Doc

func funcMap(
	tmplName string,
	mDocs DocsMap,
	sNavDocs []Vars,
) map[string]interface{} {

	fnVars := func(name string) (Doc, bool) {
		if doc, ok := mDocs[name]; ok {
			return doc, true
		}
		fmt.Println("DOC NAMES:")
		for k := range mDocs {
			fmt.Println(k)
		}
		return Doc{}, false
	}

	var funcmap map[string]interface{}
	funcmap = map[string]interface{}{
		"md2html": func(md string) (string, error) {
			return Md2HtmlStr([]byte(md))
		},
		"doCmd": func(cmd string, params ...string) string {
			doc, _ := fnVars(tmplName)
			return runCmdMergedOutput(doc.Vars, cmd, params...)
		},
		// NOTE: tmplName == document src path, relative to document root
		"doTmpl": func(tmplName string, data interface{}) (string, error) {
			// get doc
			doc, ok := fnVars(tmplName)
			if !ok {
				return "", fmt.Errorf("template `%s` not found", tmplName)
			}
			// default to doc's own vars when data == nil
			if data == nil {
				data = doc.Vars
			}
			// render
			pbuf := bytes.NewBuffer(make([]byte, 0, 64*1024))
			doc.Tmpl.Funcs(funcmap)
			err := postProcess(pbuf, tmplName, doc.Tmpl, data)
			return pbuf.String(), err
		},
		"docsAll": func() []Vars {
			// clone
			ret := make([]Vars, len(sNavDocs))
			for i := range sNavDocs {
				ret[i] = sNavDocs[i]
			}
			return ret
		},
		"docsSort": func(sVars []Vars, bAsc bool, ordKeys ...string) []Vars {
			if len(ordKeys) == 0 {
				return sVars
			}
			sort.Slice(sVars, func(i, j int) bool {
				sT := make([]string, 2)
				for ixStr, ixVar := range []int{i, j} {
					for _, ordK := range ordKeys {
						if v, ok := sVars[ixVar][ordK].(string); ok {
							sT[ixStr] = v
							break
						}
					}
				}
				if bAsc {
					return sT[0] < sT[1]
				}
				return sT[0] > sT[1]
			})
			return sVars
		},
	}
	return funcmap
}

func IsLayoutableExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".htm", ".html", ".xml", ".md":
		return true
	}
	return false
}

func IsTemplateExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".htm", ".html", ".xml", ".md", ".css", ".gcss":
		return true
	}
	return false
}

func Md2HtmlWri(dst io.Writer, md []byte) error {
	md_enc := goldmark.New(
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
	return md_enc.Convert(md, dst)
}

func Md2HtmlStr(md []byte) (string, error) {
	var bufHtml bytes.Buffer
	if err := Md2HtmlWri(&bufHtml, md); err != nil {
		return "", err
	}
	return bufHtml.String(), nil
}

func NewTemplate(tmplName string, dl Delims) *tt.Template {
	// NOTE: all funcs need to exist at Parse(),
	//       but funcs are re-bound after Parse(), with data.
	return tt.New(tmplName).
		Delims(dl.L, dl.R).
		Funcs(funcMap("", nil, nil)).
		Option("missingkey=zero")
}
