package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	IsShowVars  bool
	IsTty       bool
	IsWatchMode bool

	rxHdrDelim *regexp.Regexp
}

func (oB *Builder) SetHdrDelim(headerDelim string) (err error) {
	if len(headerDelim) == 0 {
		oB.rxHdrDelim = nil
	} else {
		hdrPat := `(?:^|\r?\n)` + regexp.QuoteMeta(headerDelim) + `(?:$|\r?\n)`
		oB.rxHdrDelim, err = regexp.Compile(hdrPat)
	}
	return
}

func (oB *Builder) GetHdrDelim() *regexp.Regexp {
	return oB.rxHdrDelim
}

type LayoutMode uint8

const (
	NOLAYOUT LayoutMode = iota
	LAYOUT
)

func (oB Builder) getDocAndLayout(path string, vinit Vars, mode LayoutMode) (
	DocProps, error,
) {

	doc, err := GetDoc(path, oB.rxHdrDelim)
	if err != nil {
		return doc, err
	}

	// auto vars
	doc.Vars["SRC_ROOT"] = filepath.Dir(oB.ConfDir)
	doc.Vars["PUB_ROOT"] = oB.PubDir
	doc.Vars["CONF_ROOT"] = oB.ConfDir
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

		// default layout if not specified
		docLayout := doc.Vars.GetStr("layout")
		if len(docLayout) == 0 {
			docLayout = "layout.html"
			doc.Vars["layout"] = docLayout
		}

		// get layout doc & parse header
		dlay, err := GetDoc(
			filepath.Join(oB.ConfDir, docLayout),
			oB.rxHdrDelim,
		)

		if os.IsNotExist(err) {

			// no layout: merge-in doc vars (global < doc)
			doc.Vars = MergeVars(vinit, doc.Vars)
			delete(doc.Vars, "layout")

		} else if err != nil {

			return doc, err

		} else {

			// append any non-conforming keys to main document
			doc.NonConformingKeys = append(
				doc.NonConformingKeys, dlay.NonConformingKeys...,
			)

			// merge vars (global < layout)
			// create layout tmpl, get/set layout delims
			dlay.Vars = MergeVars(vinit, dlay.Vars)
			tmplLayout := textTemplate(dlay.Vars)
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
		doc.Vars.PrettyPrint(os.Stdout, doc.NonConformingKeys, oB.IsTty)
	}

	return doc, nil
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
	tmpl := textTemplate(doc.Vars)
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
