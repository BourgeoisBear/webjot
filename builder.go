package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	tt "text/template"
	"time"

	"github.com/pkg/errors"
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
	TmplName   string
	LayoutName string
	Tmpl       *tt.Template
}

type Layout2Docs map[string][]Doc
type Layouts map[string]Doc

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

	doc, err := LoadDocProps(path, oB.rxHdrDelim)
	if err != nil {
		return doc, err
	}

	// get relative path of dst
	_, dstRel, err := oB.SrcPath2DstRel(path)
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

type ErrFunc func(err error, msg string)

/*
Render each document in mLayout inside its specified layout.
*/
func (oB Builder) ApplyLayouts(mLayout Layout2Docs, mLo Layouts, fnErr ErrFunc) {

	// get total document count
	nDocs := 0
	for _, sDocs := range mLayout {
		nDocs += len(sDocs)
	}

	// build vars list & templates map for all docs
	sDocsOrdered := make([]Vars, 0, nDocs)
	mDocs := make(DocsMap, nDocs)
	vinit := GetEnvGlobals()
	for loName, sDocs := range mLayout {
		// vars hierarchy (global < layout < document)
		vbase := vinit
		if docLo, ok := mLo[loName]; ok {
			vbase = MergeVars(vinit, docLo.Vars)
		}
		for _, doc := range sDocs {
			doc.Vars = MergeVars(vbase, doc.Vars)
			if IsLayoutableExt(filepath.Ext(doc.TmplName)) {
				sDocsOrdered = append(sDocsOrdered, doc.Vars)
			}
			mDocs[doc.TmplName] = doc
		}
	}

	// sort list of doc vars by document (title, URI_PATH)
	sort.Slice(sDocsOrdered, func(i, j int) bool {
		sT := make([]string, 2)
		for strIx, dvIx := range []int{i, j} {
			s, _ := sDocsOrdered[dvIx]["title"].(string)
			if len(s) == 0 {
				s, _ = sDocsOrdered[dvIx]["URI_PATH"].(string)
			}
			sT[strIx] = s
		}
		return sT[0] < sT[1]
	})

	ptDefault := NewTemplate("", DefaultDelims())
	ptDefault.Parse(`{{ doTmpl .DOC_KEY . }}`)

	// iterate layouts
	for docLayout, sDocs := range mLayout {

		// get layout template
		var pLayoutTmpl *tt.Template
		if len(docLayout) == 0 {
			pLayoutTmpl = ptDefault
		} else {
			docLo := mLo[docLayout]
			if docLo.Tmpl == nil {
				fnErr(errors.New("layout not found"), docLayout)
				continue
			}
			pLayoutTmpl = docLo.Tmpl
		}

		// render documents
		for _, doc := range sDocs {

			// don't render to /.pub docs marked as skip: true
			if bSkip, _ := doc.Vars["skip"].(bool); bSkip {
				continue
			}

			// render document to destination file
			if e2 := func() error {
				fDst, err := oB.CreateDstFile(doc.DstPath)
				if err != nil {
					return err
				}
				defer fDst.Close()

				/*
					TODO:
						- ability to call {{template "name" pipeline}}
							(add parse trees)?
						- OR remove {{ template ... }} nodes from parse trees

						- check if there are recursion issues with doc.TmplName & doCmd
				*/

				// NOTE: re-populate Funcs() on each doc to bind updated Vars
				doc.Vars["DOC_KEY"] = doc.TmplName
				return pLayoutTmpl.
					Funcs(funcMap(doc.TmplName, mDocs, sDocsOrdered)).
					Execute(fDst, doc.Vars)
			}(); e2 != nil {
				fnErr(e2, doc.TmplName)
			}
		}
	}
}

// Determine destination filename from source filename.
func (oB Builder) SrcPath2DstRel(srcPath string) (
	srcrel, dstrel string, err error,
) {
	// get relative path of src
	srcrel, err = filepath.Rel(filepath.Dir(oB.ConfDir), srcPath)
	if err != nil {
		return
	}
	// extension changes (i.e. md -> html), if any
	ext := filepath.Ext(srcrel)
	switch strings.ToLower(ext) {
	case ".md":
		dstrel = strings.TrimSuffix(srcrel, ext) + ".html"
	case ".gcss":
		dstrel = strings.TrimSuffix(srcrel, ext) + ".css"
	default:
		dstrel = srcrel
	}
	return
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

func (oB Builder) compileLayout(path string) (*Doc, error) {

	// get relative path for layout reference in templates
	relPath, err := filepath.Rel(oB.ConfDir, path)
	if err != nil {
		return nil, err
	}

	pdoc := &Doc{
		TmplName: filepath.ToSlash(relPath), // NOTE: uniform slashing
	}

	// get layout & its header
	pdoc.DocProps, err = LoadDocProps(path, oB.rxHdrDelim)
	if err != nil {
		return pdoc, err
	}

	// create layout tmpl, get/set layout delims
	delete(pdoc.Vars, "layout")
	pdoc.Tmpl = NewTemplate("", pdoc.Vars.GetDelims())
	pdoc.Tmpl, err = pdoc.Tmpl.Parse(string(pdoc.DocProps.Source))
	return pdoc, err
}

func (oB Builder) compileOrCopyFile(path string, vinit Vars) (*Doc, error) {

	// create dst dir
	srcrel, dstrel, err := oB.SrcPath2DstRel(path)
	if err != nil {
		return nil, err
	}

	// NOTE: always make dirs, in case of file/dir addition under watch mode
	//       (i.e. partial re-build)
	dstdir := filepath.Dir(filepath.Join(oB.PubDir, dstrel))
	err = os.MkdirAll(dstdir, oB.DirMode)
	if err != nil && !os.IsExist(err) {
		return nil, err
	}

	/*
		fmt.Printf("%#v\n", map[string]string{
			"path":   path,
			"srcrel": srcrel,
			"dstrel": dstrel,
			"dstdir": dstdir,
			"dst":    filepath.Join(oB.PubDir, dstrel),
		})
	*/

	// simple copy & early-exit for non-template extensions
	ext := filepath.Ext(path)
	if !IsTemplateExt(ext) {
		dstfile := filepath.Join(oB.PubDir, dstrel)
		err := CopyOnDirty(dstfile, path, oB.FileMode)
		if err != nil {
			return nil, err
		}
	}

	// get doc and vars
	dp, err := oB.getDocAndAutoVars(path)
	if err != nil {
		return nil, err
	}

	// template expansion
	tmpl := NewTemplate("", dp.Vars.GetDelims())
	tmpl, err = tmpl.Parse(string(dp.Source))
	if err != nil {
		return nil, err
	}

	// layout determination
	vars := MergeVars(vinit, dp.Vars)
	docLayout := vars.GetStr("layout")
	if IsLayoutableExt(ext) {
		// disable layout if key is specified, but value is empty
		// use default layout if unspecified
		if len(docLayout) == 0 {
			if _, ok := vars["layout"]; !ok {
				docLayout = "layout.html"
			}
		}
	} else {
		// disable layouts for all others
		docLayout = ""
	}

	return &Doc{
		DocProps:   dp,
		Tmpl:       tmpl,
		TmplName:   filepath.ToSlash(srcrel), // NOTE: uniform slashing
		LayoutName: docLayout,
	}, nil
}

type DocType uint

const (
	DT_NIL DocType = iota
	DT_DOC
	DT_LAYOUT
)

func (oB Builder) buildFile(
	path string,
	vinit Vars,
	mL2D Layout2Docs,
	mLo Layouts,
) (pdoc *Doc, dt DocType, err error) {

	defer func() {
		if err != nil {
			err = errors.WithMessage(err, path)
		}
		if oB.IsShowVars && (pdoc != nil) {
			pdoc.Vars.PrettyPrint(
				os.Stdout, pdoc.NonConformingKeys, rxPprintExcl, oB.IsTty,
			)
		}
	}()

	// get relative path of src
	srcrel, err := filepath.Rel(filepath.Dir(oB.ConfDir), path)
	if err != nil {
		return
	}

	// compile layout
	if filepath.HasPrefix(path, oB.ConfDir) {
		ext := filepath.Ext(path)
		switch strings.ToLower(ext) {
		case ".html", ".htm", ".xml":
			progressIndicator(srcrel+" (LAYOUT)", oB.IsTty)
			pdoc, err = oB.compileLayout(path)
			if err != nil {
				return
			}
			mLo[pdoc.TmplName] = *pdoc
			dt = DT_LAYOUT
		}
		return
	}

	// compile templates / copy others into `.pub/`
	progressIndicator(srcrel+" (DOCUMENT)", oB.IsTty)
	pdoc, err = oB.compileOrCopyFile(path, vinit)
	if err != nil {
		return
	}

	// append doc to layout map
	if pdoc != nil {
		sDocs := mL2D[pdoc.LayoutName]
		// if exists, overwrite
		bFound := false
		for ix := range sDocs {
			if sDocs[ix].TmplName == pdoc.TmplName {
				sDocs[ix] = *pdoc
				bFound = true
				break
			}
		}
		// if not found, append
		if !bFound {
			sDocs = append(sDocs, *pdoc)
		}
		mL2D[pdoc.LayoutName] = sDocs
		dt = DT_DOC
	}

	return
}
