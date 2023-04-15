package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	tmpl "text/template"
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

type Doc struct {
	DocProps
	Body []byte
}

type Layouts map[string][]Doc

var rxPprintExcl *regexp.Regexp

func init() {
	rxPprintExcl = regexp.MustCompile(`DIR$|WATCHMODE`)
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

func (oB Builder) getDocAndAutoVars(path string) (DocProps, error) {

	doc, err := GetDoc(path, oB.rxHdrDelim)
	if err != nil {
		return doc, err
	}

	// get relative path of dst
	dstRel, err := oB.SrcPath2DstRel(path)
	if err != nil {
		return doc, err
	}

	// set absolute path of dst
	doc.DstPath = filepath.Join(oB.PubDir, dstRel)

	// auto vars
	doc.Vars["URI_PATH"] = filepath.ToSlash(dstRel)
	doc.Vars["CFGDIR"] = oB.ConfDir
	doc.Vars["SRC"] = path
	doc.Vars["SRCMOD"] = doc.Info.ModTime().Format(time.RFC3339)
	srcRoot := filepath.Dir(oB.ConfDir)
	doc.Vars["SRCDIR"] = srcRoot
	doc.Vars["PUBDIR"] = filepath.Join(srcRoot, PUBDIR)
	if oB.IsWatchMode {
		doc.Vars["WATCHMODE"] = "enabled"
	}

	return doc, nil
}

// TODO: separate modules?

type ErrFunc func(err error, msg string)

/*
Parse given layout templates.
Nest pre-parsed document templates.
Render nested templates.
*/
func (oB Builder) ApplyLayouts(mLayout Layouts, fnErr ErrFunc) {

	vinit := GetEnvGlobals()

	// iterate layouts
	for docLayout, sDocs := range mLayout {

		// skip layout application when not supplied
		if len(docLayout) == 0 {
			for _, doc := range sDocs {
				if err := oB.copyToDst(&doc); err != nil {
					fnErr(err, doc.SrcPath)
				}
			}
			continue
		}

		// get layout & layout header
		layoutSrc := filepath.Join(oB.ConfDir, docLayout)
		progressIndicator(docLayout+" (LAYOUT)", oB.IsTty)
		dlay, eLayout := GetDoc(layoutSrc, oB.rxHdrDelim)
		if eLayout != nil {
			fnErr(eLayout, layoutSrc)
			continue
		}
		dlay.Vars = MergeVars(vinit, dlay.Vars)

		// report
		if oB.IsShowVars {
			dlay.Vars.PrettyPrint(os.Stdout, dlay.NonConformingKeys, rxPprintExcl, oB.IsTty)
		}

		// create layout tmpl, get/set layout delims
		tmplLayout := textTemplate(dlay.Vars)
		tmplLayout, eLayout = tmplLayout.Parse(string(dlay.Source))
		if eLayout != nil {
			fnErr(eLayout, layoutSrc)
			continue
		}

		// clear delims from vars
		dlay.Vars.ClearDelims()

		// TODO: pass-in all pages and per-layout pages as separate map items
		for _, doc := range sDocs {
			if e2 := oB.applyLayoutToDoc(tmplLayout, &doc); e2 != nil {
				fnErr(e2, doc.SrcPath)
			}
		}
	}
}

func (oB Builder) applyLayoutToDoc(pLayout *tmpl.Template, pDoc *Doc) error {

	// open dst file
	fDst, err := oB.CreateDstFile(pDoc.DstPath)
	if err != nil {
		return err
	}
	defer fDst.Close()

	// TODO: all-files map for menus

	pDoc.Vars["HTML_CONTENT"] = string(pDoc.Body)

	// NOTE: re-populate Funcs() to bind updated Vars
	return pLayout.Funcs(funcMap(pDoc.Vars)).Execute(fDst, pDoc.Vars)
}

// Determine destination filename from source filename.
func (oB Builder) SrcPath2DstRel(srcPath string) (string, error) {
	// get relative path of src
	rel, err := filepath.Rel(filepath.Dir(oB.ConfDir), srcPath)
	if err != nil {
		return "", err
	}
	// extension changes (i.e. md -> html), if any
	ext := filepath.Ext(rel)
	if strings.ToLower(ext) == ".md" {
		rel = strings.TrimSuffix(rel, ext) + ".html"
	}
	return rel, nil
}

/*
Create/Truncate destination file.
*/
func (oB Builder) CreateDstFile(path string) (*os.File, error) {
	flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
	return os.OpenFile(path, flags, oB.FileMode)
}

/*
Determine destination filename from source filename.
Create/Truncate destination file.
Copy source file contents to destination file.
*/
func (oB Builder) copyToDst(pDoc *Doc) error {
	fDst, err := oB.CreateDstFile(pDoc.DstPath)
	if err != nil {
		return err
	}
	defer fDst.Close()
	_, err = fDst.Write(pDoc.Body)
	return err
}

func BuildCss(iWri io.Writer, body []byte, vars Vars, ext string) error {
	tmpl := textTemplate(vars)
	tmpl, err := tmpl.Parse(string(body))
	if err != nil {
		return err
	}
	var bufTmpl bytes.Buffer
	if err = tmpl.Execute(&bufTmpl, vars); err != nil {
		return err
	}
	if ext == ".gcss" {
		_, err = gcss.Compile(iWri, &bufTmpl)
	} else {
		_, err = io.Copy(iWri, &bufTmpl)
	}
	return err
}

func BuildHtml(iWri io.Writer, body []byte, vars Vars) error {
	tmpl := textTemplate(vars)
	tmpl, err := tmpl.Parse(string(body))
	if err != nil {
		return err
	}
	return tmpl.Execute(iWri, vars)
}

func BuildMd(iWri io.Writer, body []byte, vars Vars) error {
	tmpl := textTemplate(vars)
	tmpl, err := tmpl.Parse(string(body))
	if err != nil {
		return err
	}
	var bufTmpl bytes.Buffer
	err = tmpl.Execute(&bufTmpl, vars)
	if err != nil {
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
	return md.Convert(bufTmpl.Bytes(), iWri)
}

func progressIndicator(msg string, bColor bool) {
	if bColor {
		fmt.Print("\x1b[96;1m>\x1b[0m ")
	} else {
		fmt.Print("> ")
	}
	fmt.Println(msg)
}

func (oB Builder) build(path string, info fs.DirEntry, mLayout Layouts) (Layouts, error) {

	if mLayout == nil {
		mLayout = make(Layouts)
	}

	// skip hidden
	if strings.HasPrefix(info.Name(), ".") {
		if info.IsDir() {
			return mLayout, filepath.SkipDir
		}
		return mLayout, nil
	}

	relpath, err := filepath.Rel(filepath.Dir(oB.ConfDir), path)
	if err != nil {
		return mLayout, err
	}

	// create destination dirs
	if info.IsDir() {
		eDir := os.MkdirAll(filepath.Join(oB.PubDir, relpath), oB.DirMode)
		if eDir != nil && !os.IsExist(eDir) {
			return mLayout, eDir
		}
		return mLayout, nil
	}

	// progress indicator
	progressIndicator(relpath+" (SOURCE)", oB.IsTty)

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".htm", ".html", ".xml", ".css", ".gcss", ".md":
		// no-op
	default:
		// simple copy & early-exit for all other extensions
		fSrc, err := os.Open(path)
		if err != nil {
			return mLayout, err
		}
		defer fSrc.Close()

		dstPath, err := oB.SrcPath2DstRel(path)
		if err != nil {
			return mLayout, err
		}

		fDst, err := oB.CreateDstFile(filepath.Join(oB.PubDir, dstPath))
		if err != nil {
			return mLayout, err
		}
		defer fDst.Close()

		_, err = io.Copy(fDst, fSrc)
		return mLayout, err
	}

	// get doc and vars
	dp, err := oB.getDocAndAutoVars(path)
	if err != nil {
		return mLayout, err
	}
	vars := MergeVars(GetEnvGlobals(), dp.Vars)

	// report
	if oB.IsShowVars {
		vars.PrettyPrint(os.Stdout, dp.NonConformingKeys, rxPprintExcl, oB.IsTty)
	}

	// build (template expansion)
	pBuf := bytes.NewBuffer(make([]byte, 0, len(dp.Source)))
	DEFAULT_LAYOUT := "layout.html"
	switch ext {
	case ".md":
		err = BuildMd(pBuf, dp.Source, vars)
	case ".htm", ".html", ".xml":
		err = BuildHtml(pBuf, dp.Source, vars)
	case ".gcss", ".css":
		err = BuildCss(pBuf, dp.Source, vars, ext)
		DEFAULT_LAYOUT = ""
	}
	if err != nil {
		return mLayout, err
	}

	// append doc to its parent layout
	docLayout := vars.GetStr("layout")
	if len(docLayout) == 0 {
		docLayout = DEFAULT_LAYOUT
	}
	sDocs := mLayout[docLayout]
	sDocs = append(sDocs, Doc{
		DocProps: dp,
		Body:     pBuf.Bytes(),
	})
	mLayout[docLayout] = sDocs
	return mLayout, nil
}
