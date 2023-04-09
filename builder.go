package main

import (
	"bytes"
	"fmt"
	tHtml "html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	tText "text/template"
	"time"

	"github.com/yosssi/gcss"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

type Builder struct {
	PubDir      string
	ConfDir     string
	DirMode     os.FileMode
	FileMode    os.FileMode
	Vdelim      string
	IsShowVars  bool
	IsTty       bool
	IsWatchMode bool
}

func textTemplate(mV Vars) *tText.Template {
	return tText.New("").Delims(delimOvr(mV)).Funcs(funcMap(mV)).Option("missingkey=zero")
}

func htmlTemplate(mV Vars) *tHtml.Template {
	return tHtml.New("").Delims(delimOvr(mV)).Funcs(funcMap(mV)).Option("missingkey=zero")
}

type LayoutMode uint8

const (
	NOLAYOUT LayoutMode = iota
	LAYOUT
)

func (oB Builder) getDocAndLayout(path string, vinit Vars, mode LayoutMode) (
	DocProps, error,
) {

	doc, err := GetDoc(path, oB.Vdelim)
	if err != nil {
		return doc, err
	}

	// auto vars
	doc.Vars["FNAME"] = filepath.Base(path)
	doc.Vars["MODIFIED"] = doc.Info.ModTime().Format(time.RFC3339)
	if oB.IsWatchMode {
		doc.Vars["WATCHMODE"] = "enabled"
	}

	// layout control
	switch mode {
	case NOLAYOUT:

		// no layout: merge-in doc vars (global < doc)
		doc.Vars = MergeVars(vinit, doc.Vars)
		delete(doc.Vars, "layout")

	case LAYOUT:

		if len(doc.Vars["layout"]) == 0 {
			doc.Vars["layout"] = "layout.html"
		}
		pathLayout := filepath.Join(oB.ConfDir, doc.Vars["layout"])
		dlay, err := GetDoc(pathLayout, oB.Vdelim)

		if os.IsNotExist(err) {

			// TODO: notify to STDERR if not found
			// no layout: merge-in doc vars (global < doc)
			doc.Vars = MergeVars(vinit, doc.Vars)
			delete(doc.Vars, "layout")

		} else if err != nil {

			return doc, err

		} else {

			// merge vars (global < layout)
			// create layout tmpl, get/set layout delims
			dlay.Vars = MergeVars(vinit, dlay.Vars)
			tmplLayout := htmlTemplate(dlay.Vars)
			if tmplLayout, err = tmplLayout.Parse(string(dlay.Body)); err != nil {
				return dlay, err
			}
			doc.Layout = tmplLayout

			// clear delims from vars
			// layout: merge-in doc vars (global < layout < doc)
			dlay.Vars.ClearDelims()
			doc.Vars = MergeVars(dlay.Vars, doc.Vars)
		}
	}

	if oB.IsShowVars {
		doc.Vars.PrettyPrint(os.Stdout, oB.IsTty)
	}

	return doc, nil
}

func delimOvr(mV Vars) (string, string) {
	l, r := "{{", "}}"
	if v := mV["ldelim"]; len(v) > 0 {
		l = v
	}
	if v := mV["rdelim"]; len(v) > 0 {
		r = v
	}
	return l, r
}

func (oB Builder) buildCSS(iWri io.Writer, doc DocProps, ext string) error {

	// render vars
	tmpl := textTemplate(doc.Vars)
	tmpl, err := tmpl.Parse(string(doc.Body))
	if err != nil {
		return err
	}

	var bufTmpl bytes.Buffer
	if err = tmpl.Execute(&bufTmpl, doc.Vars); err != nil {
		return err
	}

	if ext == ".gcss" {
		_, err = gcss.Compile(iWri, &bufTmpl)
	} else {
		_, err = io.Copy(iWri, &bufTmpl)
	}
	return err
}

func (oB Builder) buildHTML(iWri io.Writer, doc DocProps) error {

	// render html
	tmpl := htmlTemplate(doc.Vars)
	tmpl, err := tmpl.Parse(string(doc.Body))
	if err != nil {
		return err
	}

	var strOut bytes.Buffer
	if err = tmpl.Execute(&strOut, doc.Vars); err != nil {
		return err
	}

	// wrap inside layout
	return doc.ApplyLayout(strOut.Bytes(), iWri)
}

func (oB Builder) buildMarkdown(iWri io.Writer, doc DocProps) error {

	// render vars
	tmpl := textTemplate(doc.Vars)
	tmpl, err := tmpl.Parse(string(doc.Body))
	if err != nil {
		return err
	}

	var bufTmpl bytes.Buffer
	if err = tmpl.Execute(&bufTmpl, doc.Vars); err != nil {
		return err
	}
	// render markdown
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

	var strOut bytes.Buffer
	if err := md.Convert(bufTmpl.Bytes(), &strOut); err != nil {
		return err
	}

	// wrap inside layout
	return doc.ApplyLayout(strOut.Bytes(), iWri)
}

func (oB Builder) build(path string, iWri io.Writer) error {

	var err error

	// get src info
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	// skip hidden
	if strings.HasPrefix(info.Name(), ".") {
		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}

	// get relative path of src
	relpath, err := filepath.Rel(filepath.Dir(oB.ConfDir), path)
	if err != nil {
		return err
	}

	// create dst from relative path
	dst := filepath.Join(oB.PubDir, relpath)

	// create destination dirs
	if info.IsDir() {
		err = os.MkdirAll(dst, oB.DirMode)
		if os.IsExist(err) {
			err = nil
		}
		return err
	}

	// progress indicator
	if oB.IsTty {
		fmt.Print("\x1b[96;1m>\x1b[0m ")
	} else {
		fmt.Print("> ")
	}
	fmt.Println(relpath)

	// extension renames
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md":
		dst = strings.TrimSuffix(dst, ext) + ".html"
	case ".gcss":
		dst = strings.TrimSuffix(dst, ext) + ".css"
	}

	// create output file
	if iWri == nil {
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, oB.FileMode)
		if err != nil {
			return err
		}
		defer out.Close()
		iWri = out
	}

	vbase := GetEnvGlobals()
	vbase["PATH"] = relpath

	// build
	switch ext {
	case ".md":
		dp, err := oB.getDocAndLayout(path, vbase, LAYOUT)
		if err != nil {
			return err
		}
		return oB.buildMarkdown(iWri, dp)
	case ".html", ".xml":
		dp, err := oB.getDocAndLayout(path, vbase, LAYOUT)
		if err != nil {
			return err
		}
		return oB.buildHTML(iWri, dp)
	case ".gcss", ".css":
		dp, err := oB.getDocAndLayout(path, vbase, NOLAYOUT)
		if err != nil {
			return err
		}
		return oB.buildCSS(iWri, dp, ext)
	default:
		fSrc, err := os.Open(path)
		if err == nil {
			_, err = io.Copy(iWri, fSrc)
			fSrc.Close()
		}
		return err
	}
}

/*
run executes a command or a script. Vars define the command environment,
each var is converted into OS environemnt variable with ZS_ prefix
prepended.  Additional variable $ZS contains path to the binary. Command
stderr is printed to stderr, command output is returned as a string.
*/
func runCmd(mV Vars, cmd string, args ...string) (sout, serr []byte, err error) {

	var errbuf, outbuf bytes.Buffer
	c := exec.Command(cmd, args...)

	env := os.Environ()
	for k, v := range mV {
		env = append(env, "ZS_"+strings.ToUpper(k)+"="+v)
	}

	c.Env = env
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
		"cmdText": func(cmd string, params ...string) string {
			return runCmdMergedOutput(mV, cmd, params...)
		},
		"cmdHtml": func(cmd string, params ...string) tHtml.HTML {
			return tHtml.HTML(runCmdMergedOutput(mV, cmd, params...))
		},
	}
}
