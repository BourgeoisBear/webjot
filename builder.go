package main

import (
	"bytes"
	"fmt"
	tp_html "html/template"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	tp_txt "text/template"
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
	Ldelim      string
	Rdelim      string
	Vdelim      string
	IsShowVars  bool
	IsTty       bool
	IsWatchMode bool
}

/*
returns list of variables defined in a text file and actual file
content following the variables declaration.
*/
func (oB Builder) getVars(path string, mGlobals Vars) (
	Vars, []byte, error,
) {

	bsSrc, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}

	// clone globals
	mV := Vars{}
	for name, value := range mGlobals {
		mV[name] = value
	}

	// title from filename
	fname := filepath.Base(path)
	title := strings.TrimSuffix(fname, filepath.Ext(fname))
	mV["title"] = strings.Title(title)

	// split into header/body
	header, body, found := bytes.Cut(bsSrc, []byte("\n"+oB.Vdelim+"\n"))
	if !found {
		return mV, bsSrc, nil
	}

	// parse vars from header
	parseVarsHeader(header, mV)
	return mV, body, nil
}

// TODO: vars header?
func (oB Builder) applyLayout(content tp_html.HTML, iWri io.Writer, mV Vars) error {

	relLayout := mV["layout"]
	if len(relLayout) == 0 {
		relLayout = "layout.html"
	}

	// load layout
	bsLayout, err := ioutil.ReadFile(filepath.Join(oB.ConfDir, relLayout))
	if err != nil {
		return err
	}

	// create layout template
	tmpl, err := tp_html.New("").
		Delims(oB.delimOvr(mV)).
		Funcs(funcMap(mV)).
		Parse(string(bsLayout))
	if err != nil {
		return err
	}

	// clone vars
	m := make(map[string]interface{}, len(mV))
	for k, v := range mV {
		m[k] = v
	}
	m["content"] = content

	// render
	return tmpl.Execute(iWri, m)
}

func (oB Builder) delimOvr(mV Vars) (string, string) {
	l, r := oB.Ldelim, oB.Rdelim
	/*
		if v := mV["ldelim"]; len(v) > 0 {
			l = v
		}
		if v := mV["rdelim"]; len(v) > 0 {
			r = v
		}
	*/
	return l, r
}

func (oB Builder) buildCSS(body []byte, iWri io.Writer, mV Vars, isGCSS bool) error {

	// render vars
	tmpl, err := tp_txt.New("").
		Delims(oB.delimOvr(mV)).
		Funcs(funcMap(mV)).
		Parse(string(body))
	if err != nil {
		return err
	}

	var bufTmpl bytes.Buffer
	if err = tmpl.Execute(&bufTmpl, mV); err != nil {
		return err
	}

	if isGCSS {
		_, err = gcss.Compile(iWri, &bufTmpl)
	} else {
		_, err = io.Copy(iWri, &bufTmpl)
	}
	return err
}

func (oB Builder) buildHTML(body []byte, iWri io.Writer, mV Vars) error {

	// render html
	tmpl, err := tp_html.New("").
		Delims(oB.delimOvr(mV)).
		Funcs(funcMap(mV)).
		Parse(string(body))
	if err != nil {
		return err
	}
	var strOut strings.Builder
	if err = tmpl.Execute(&strOut, mV); err != nil {
		return err
	}

	// wrap inside layout
	return oB.applyLayout(tp_html.HTML(strOut.String()), iWri, mV)
}

func (oB Builder) buildMarkdown(body []byte, iWri io.Writer, mV Vars) error {

	// render vars
	tmpl, err := tp_txt.New("").
		Delims(oB.delimOvr(mV)).
		Funcs(funcMap(mV)).
		Parse(string(body))
	if err != nil {
		return err
	}

	var bufTmpl bytes.Buffer
	if err = tmpl.Execute(&bufTmpl, mV); err != nil {
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

	var strOut strings.Builder
	if err := md.Convert(bufTmpl.Bytes(), &strOut); err != nil {
		return err
	}

	// wrap inside layout
	return oB.applyLayout(tp_html.HTML(strOut.String()), iWri, mV)
}

func (oB Builder) build(path string, iWri io.Writer, mV Vars) error {

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
	bGetVars := false
	switch ext {
	case ".md", ".mkd":
		dst = strings.TrimSuffix(dst, ext) + ".html"
		bGetVars = true
	case ".html", ".xml":
		bGetVars = true
	case ".css":
		bGetVars = true
	case ".gcss":
		dst = strings.TrimSuffix(dst, ext) + ".css"
		bGetVars = true
	}

	// vars
	var body []byte
	if bGetVars {
		mV, body, err = oB.getVars(path, mV)
		if err != nil {
			return err
		}
		mV["pubdir"] = oB.PubDir
		mV["path"] = relpath
		mV["fname"] = filepath.Base(path)
		mV["modified"] = info.ModTime().Format(time.RFC3339)
		if oB.IsWatchMode {
			mV["watchmode"] = "enabled"
		}

		if oB.IsShowVars {
			mV.PrettyPrint(os.Stdout, oB.IsTty)
		}
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

	// build
	switch ext {
	case ".md", ".mkd":
		return oB.buildMarkdown(body, iWri, mV)
	case ".html", ".xml":
		return oB.buildHTML(body, iWri, mV)
	case ".css":
		return oB.buildCSS(body, iWri, mV, false)
	case ".gcss":
		return oB.buildCSS(body, iWri, mV, true)
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
		"cmdHtml": func(cmd string, params ...string) tp_html.HTML {
			return tp_html.HTML(runCmdMergedOutput(mV, cmd, params...))
		},
	}
}
