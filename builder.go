package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	tmpl "text/template"
	"text/template/parse"
	"time"

	"github.com/yosssi/gcss"
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
	ParseTree *parse.Tree
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
// TODO: test mixed delimiters
// TODO: only re-parse dirty templates

type ErrFunc func(err error, msg string)

/*
Create/Truncate destination file.
Copy source file contents to destination file.
*/
func (oB Builder) renderToDst(dp *DocProps) error {
	// compile template
	tmpl := NewTemplate("", dp.Vars.GetDelims())
	tmpl, err := tmpl.Parse(string(dp.Source))
	if err != nil {
		return err
	}
	// re-bind func map vars
	tmpl = tmpl.Funcs(funcMap(tmpl, dp, nil))
	// output file
	fDst, err := oB.CreateDstFile(dp.DstPath)
	if err != nil {
		return err
	}
	defer fDst.Close()
	// special handling
	ext := strings.ToLower(filepath.Ext(dp.SrcPath))
	switch ext {
	case ".md":
		var buf bytes.Buffer
		if err = tmpl.Execute(&buf, dp.Vars); err != nil {
			return err
		}
		return FromMarkdown(fDst, buf.Bytes())
	case ".gcss":
		var buf bytes.Buffer
		if err = tmpl.Execute(&buf, dp.Vars); err != nil {
			return err
		}
		_, err = gcss.Compile(fDst, &buf)
		return err
	}

	return tmpl.Execute(fDst, dp.Vars)
}

type DocVar struct {
	Path string
	Vars Vars
}

/*
Parse given layout templates.
Nest pre-parsed document templates.
Render nested templates.
*/
func (oB Builder) ApplyLayouts(mLayout Layouts, fnErr ErrFunc) {

	vinit := GetEnvGlobals()
	base := filepath.Dir(oB.ConfDir)

	// build global docs list
	sDV := make([]DocVar, 0)
	for docLayout, sDocs := range mLayout {
		if len(docLayout) == 0 {
			continue
		}
		for _, doc := range sDocs {
			tname, e2 := filepath.Rel(base, doc.SrcPath)
			if e2 != nil {
				fnErr(e2, doc.SrcPath)
				continue
			}
			// TODO: ensure that DocVar.Path & template name replace slashes
			sDV = append(sDV, DocVar{
				Path: tname,
				Vars: doc.Vars,
			})
		}
	}
	sort.Slice(sDV, func(i, j int) bool {
		return sDV[i].Path < sDV[j].Path
	})

	// iterate layouts
	for docLayout, sDocs := range mLayout {

		// skip layout application when not supplied
		if len(docLayout) == 0 {
			for _, doc := range sDocs {
				// TODO: don't mutate on merge if we end up recycling Layouts
				doc.Vars = MergeVars(vinit, doc.Vars)
				if err := oB.renderToDst(&doc.DocProps); err != nil {
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
		tmplLayout := NewTemplate("", dlay.Vars.GetDelims())
		tmplLayout, eLayout = tmplLayout.Parse(string(dlay.Source))
		if eLayout != nil {
			fnErr(eLayout, layoutSrc)
			continue
		}

		// clear delims from vars
		dlay.Vars.ClearDelims()

		// render documents
		for _, doc := range sDocs {
			// document's src path, relative to document root, as tname
			tname, e2 := filepath.Rel(base, doc.SrcPath)
			if e2 != nil {
				fnErr(e2, doc.SrcPath)
				continue
			}
			doc.Vars = MergeVars(vinit, dlay.Vars, doc.Vars)
			if e2 = oB.applyLayoutToDoc(tmplLayout, tname, &doc, sDV); e2 != nil {
				fnErr(e2, doc.SrcPath)
			}
		}
	}
}

func (oB Builder) applyLayoutToDoc(
	pLayout *tmpl.Template,
	tname string,
	doc *Doc,
	sDV []DocVar,
) error {
	// open dst file
	fDst, err := oB.CreateDstFile(doc.DstPath)
	if err != nil {
		return err
	}
	defer fDst.Close()

	// key child ParseTree under tname
	if _, err = pLayout.AddParseTree(tname, doc.ParseTree); err != nil {
		return err
	}
	doc.Vars["DOC_KEY"] = tname

	// NOTE: re-populate Funcs() to bind updated Vars
	return pLayout.Funcs(funcMap(pLayout, &doc.DocProps, sDV)).
		Execute(fDst, doc.Vars)
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
	switch strings.ToLower(ext) {
	case ".md":
		rel = strings.TrimSuffix(rel, ext) + ".html"
	case ".gcss":
		rel = strings.TrimSuffix(rel, ext) + ".css"
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

func progressIndicator(msg string, bColor bool) {
	if bColor {
		fmt.Print("\x1b[96;1m>\x1b[0m ")
	} else {
		fmt.Print("> ")
	}
	fmt.Println(msg)
}

func (oB Builder) build(
	path string, info fs.DirEntry, mLayout Layouts,
) (Layouts, error) {

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

	// template expansion
	tmpl := NewTemplate("", dp.Vars.GetDelims())
	tmpl, err = tmpl.Parse(string(dp.Source))
	if err != nil {
		return mLayout, err
	}

	// layout determination
	docLayout := vars.GetStr("layout")
	switch ext {
	case ".md", ".htm", ".html", ".xml":
		// disable layout if key is specified, but value is empty
		// use default layout if unspecified
		if len(docLayout) == 0 {
			if _, ok := vars["layout"]; !ok {
				docLayout = "layout.html"
			}
		}
	default:
		// disable layouts for all others
		docLayout = ""
	}

	// append doc to its parent layout
	sDocs := mLayout[docLayout]
	sDocs = append(sDocs, Doc{
		DocProps:  dp,
		ParseTree: tmpl.Tree,
	})
	mLayout[docLayout] = sDocs
	return mLayout, nil
}
