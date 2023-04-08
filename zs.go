package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	tp_html "html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	tp_txt "text/template"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/yosssi/gcss"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed all:default_conf
var defaultSiteCfg embed.FS

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
run executes a command or a script. Vars define the command environment,
each zs var is converted into OS environemnt variable with ZS_ prefix
prepended.  Additional variable $ZS contains path to the zs binary. Command
stderr is printed to zs stderr, command output is returned as a string.
*/
func (oB Builder) run(mV Vars, cmd string, args ...string) (string, error) {

	var errbuf, outbuf bytes.Buffer
	c := exec.Command(cmd, args...)

	// TODO: shell escape
	env := []string{"ZS=" + os.Args[0], "ZS_OUTDIR=" + oB.PubDir}
	env = append(env, os.Environ()...)
	for k, v := range mV {
		env = append(env, "ZS_"+strings.ToUpper(k)+"="+v)
	}
	c.Env = env
	c.Stdout = &outbuf
	c.Stderr = &errbuf

	err := c.Run()

	// TODO: error reporting
	if errbuf.Len() > 0 {
		fmt.Fprintf(os.Stderr, "Command Error `%s`:\n", cmd)
		_, err = io.Copy(os.Stderr, &errbuf)
	}
	if err != nil {
		return "", err
	}
	return string(outbuf.Bytes()), nil
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

func (oB Builder) applyLayout(content string, iWri io.Writer, mV Vars) error {

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
		Delims(oB.Ldelim, oB.Rdelim).
		Parse(string(bsLayout))
	if err != nil {
		return err
	}

	// clone vars
	m := make(map[string]interface{}, len(mV))
	for k, v := range mV {
		m[k] = v
	}
	m["content"] = tp_html.HTML(content)

	// render
	return tmpl.Execute(iWri, m)
}

func (oB Builder) buildMarkdown(body []byte, iWri io.Writer, mV Vars) error {

	// render vars
	tmpl, err := tp_txt.New("").
		Delims(oB.delims(mV)).
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
	return oB.applyLayout(strOut.String(), iWri, mV)
}

func (oB Builder) buildHTML(body []byte, iWri io.Writer, mV Vars) error {

	// render html
	tmpl, err := tp_html.New("").
		Delims(oB.delims(mV)).
		Parse(string(body))
	if err != nil {
		return err
	}
	var strOut strings.Builder
	if err = tmpl.Execute(&strOut, mV); err != nil {
		return err
	}

	// wrap inside layout
	return oB.applyLayout(strOut.String(), iWri, mV)
}

func (oB Builder) delims(mV Vars) (string, string) {
	l, r := oB.Ldelim, oB.Rdelim
	if v := mV["ldelim"]; len(v) > 0 {
		l = v
	}
	if v := mV["rdelim"]; len(v) > 0 {
		r = v
	}
	return l, r
}

func (oB Builder) buildCSS(body []byte, iWri io.Writer, mV Vars, isGCSS bool) error {

	// render vars
	tmpl, err := tp_txt.New("").
		Delims(oB.delims(mV)).
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

func (oB Builder) build(path string, iWri io.Writer, mV Vars) error {
	err := oB.innerBuild(path, iWri, mV)
	if err != nil && err != fs.SkipDir {
		err = errors.WithMessage(err, path)
	}
	return err
}

func (oB Builder) innerBuild(path string, iWri io.Writer, mV Vars) error {

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

func (oB Builder) buildAll(srcDir string) error {

	vars := globals()

	// recurse through source dir
	wdFunc := func(path string, info fs.DirEntry, eWalk error) error {
		if eWalk != nil {
			return errors.WithMessage(eWalk, path)
		} else {
			return oB.build(path, nil, vars)
		}
	}

	return filepath.WalkDir(srcDir, wdFunc)
}

func (oB Builder) watch(srcDir string) error {

	// create new watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	// listen for events
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {

		defer wg.Done()

		vars := globals()
		for {
			select {
			case evt, ok := <-watcher.Events:
				if !ok {
					return
				}
				// rebuild file
				if evt.Has(fsnotify.Write) {

					modDir := filepath.Dir(evt.Name)

					// skip PubDir changes
					if filepath.HasPrefix(modDir, oB.PubDir) {
						break
					}

					fmt.Println(evt)

					if filepath.HasPrefix(modDir, oB.ConfDir) {
						// rebuild all on ConfDir changes
						errRpt(oB.buildAll(filepath.Dir(oB.ConfDir)), oB.IsTty)
					} else {
						// otherwise, rebuild dirty file only
						errRpt(oB.build(evt.Name, nil, vars), oB.IsTty)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				errRpt(err, oB.IsTty)
			}
		}
	}()

	// add dirs to watch
	err = filepath.WalkDir(
		srcDir,
		func(src string, info fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// only watch non-hidden dirs
			if !info.IsDir() || strings.HasPrefix(info.Name(), ".") {
				return nil
			}
			return watcher.Add(src)
		},
	)
	if err != nil {
		return err
	}

	// add conf dir
	if err = watcher.Add(oB.ConfDir); err != nil {
		return err
	}

	wg.Wait()
	return nil
}

func (oB Builder) initSite(tgtDir string) error {

	tgtDir, err := filepath.Abs(tgtDir)
	if err != nil {
		return err
	}

	cfgDir := filepath.Join(tgtDir, ".zs")
	if err = os.MkdirAll(cfgDir, oB.DirMode); err != nil {
		return err
	}

	sD, err := defaultSiteCfg.ReadDir("default_conf")
	if err != nil {
		return err
	}

	for ix := range sD {

		if sD[ix].IsDir() {
			continue
		}
		fname := sD[ix].Name()

		// open src
		fSrc, err := defaultSiteCfg.Open("default_conf/" + fname)
		if err != nil {
			return err
		}
		defer fSrc.Close()

		// determine dst dir
		dstDir := tgtDir
		if fname == "layout.html" {
			dstDir = cfgDir
		}

		// open dst
		fDst, err := os.OpenFile(
			filepath.Join(dstDir, fname),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
			oB.FileMode,
		)
		if err != nil {
			return err
		}
		defer fDst.Close()

		// copy from src to dst
		if _, err = io.Copy(fDst, fSrc); err != nil {
			return err
		}
	}

	return nil
}

func main() {

	bIsTty := isatty.IsTerminal(os.Stdout.Fd())

	var err error
	defer func() {
		errRpt(err, bIsTty)
	}()

	oB := Builder{
		DirMode:  0755,
		FileMode: 0644,
		IsTty:    bIsTty,
	}

	flag.StringVar(&oB.Ldelim, "ldelim", "{{", "left template delimiter")
	flag.StringVar(&oB.Rdelim, "rdelim", "}}", "right template delimiter")
	flag.StringVar(&oB.Vdelim, "vdelim", "---", "vars/body delimiter")
	flag.BoolVar(&oB.IsShowVars, "vshow", false, "show per-page render vars on build")

	var httpPort int
	flag.BoolVar(&oB.IsWatchMode, "watch", false, "rebuild on file change")
	flag.IntVar(&httpPort, "port", 8080, "HTTP port for watch-mode web server")

	bInit := false
	flag.BoolVar(&bInit, "init", false, "create a new site configuration inside the given directory")

	flag.Parse()
	args := flag.Args()

	var tgt string
	if len(args) > 0 {
		tgt = args[0]
	}

	// create new site
	if bInit {
		err = oB.initSite(tgt)
		return
	}

	if len(tgt) == 0 {
		if tgt, err = os.Getwd(); err != nil {
			return
		}
	}

	// lookup conf dir parent
	conf, err := searchDirAncestors(tgt, ".zs")
	if err != nil {
		return
	}

	webRoot := filepath.Dir(conf)

	// settings
	oB.PubDir = filepath.Join(webRoot, ".pub")
	oB.ConfDir = conf

	// absolute paths
	for _, ps := range []*string{&tgt, &oB.PubDir, &oB.ConfDir} {
		if *ps, err = filepath.Abs(*ps); err != nil {
			return
		}
	}

	// prepend .zs to $PATH, so plugins will be found before OS commands
	// p := os.Getenv("PATH")
	// p = ZSDIR + ":" + p
	// os.Setenv("PATH", p)

	if err = oB.buildAll(tgt); err != nil {
		return
	}

	if oB.IsWatchMode {

		// start watch webserver
		go func() {

			szPort := strconv.Itoa(httpPort)
			fmt.Printf("serving %s on port %d\n", oB.PubDir, httpPort)

			htdocs := http.Dir(oB.PubDir)
			hdl := HeadHandler(htdocs, http.FileServer(htdocs))
			http.Handle("/", hdl)

			// open web browser
			go func() {
				time.Sleep(time.Second)
				errRpt(OpenBrowser("http://localhost:"+szPort), bIsTty)
			}()

			// start http server
			e2 := http.ListenAndServe(":"+szPort, nil)
			if e2 != nil {
				errRpt(e2, bIsTty)
			}

		}()

		// rebuild on change
		err = oB.watch(webRoot)
	}
}
