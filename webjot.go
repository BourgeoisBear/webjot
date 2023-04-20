package main

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

const (
	CFGDIR        = ".webjot"
	PUBDIR        = ".pub"
	DEFAULT_DELIM = "@@@@@@@"
	ENVVAR_PREFIX = "ZS_"
)

//go:embed all:default_conf
var SiteCfgFS embed.FS

const SiteCfgDirName = "default_conf"

func buildAll(oB Builder, srcDir string) (Layout2Docs, Layouts, error) {

	vinit := GetEnvGlobals()
	mL2D := make(Layout2Docs)
	mLayouts := make(Layouts)

	// recurse through source dir
	wdFunc := func(path string, info fs.DirEntry, eWalk error) (eout error) {

		if eWalk != nil {
			return errors.WithMessage(eWalk, path)
		}

		fname := info.Name()
		if info.IsDir() {
			// don't recurse hidden dirs except for ConfDir
			if strings.HasPrefix(fname, ".") {
				if path != oB.ConfDir {
					return filepath.SkipDir
				}
			}
			// recurse into ordinary dirs
			return nil
		} else {
			// skip hidden files
			if strings.HasPrefix(fname, ".") {
				return nil
			}
		}

		_, _, err := oB.buildFile(path, vinit, mL2D, mLayouts)
		if err != nil {
			ErrRpt(err, oB.IsTty)
		}
		return nil
	}
	err := filepath.WalkDir(srcDir, wdFunc)
	return mL2D, mLayouts, err
}

/*
watches for changes to source and config files
re-builds on change
NOTE: blocking channel-select loop
*/
func watch(oB Builder, srcDir string, mL2D Layout2Docs, mLo Layouts) error {

	// create new pW
	pW, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer pW.Close()

	// add src dirs to watch
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
			return pW.Add(src)
		},
	)
	if err != nil {
		return err
	}

	// add conf dir to watch
	if err = pW.Add(oB.ConfDir); err != nil {
		return err
	}

	vinit := GetEnvGlobals()

	// listen for events
	for {
		select {
		case evt, ok := <-pW.Events:
			if !ok {
				return nil
			}
			/*
				TODO: treat both Remove & Rename as deletes (NOTE: rename is followed by a create)
				TODO: file exclusive between HTTP:HEAD and writes (for live.js issues)
			*/

			if evt.Has(fsnotify.Rename) {
				fmt.Println(evt)
			}

			// rebuild file
			if evt.Has(fsnotify.Write) || evt.Has(fsnotify.Create) {

				// skip dirs and hidden files
				fi, err := os.Stat(evt.Name)
				if err != nil {
					ErrRpt(errors.WithMessage(err, evt.Name), oB.IsTty)
					continue
				}
				if fi.IsDir() {
					continue
				} else if strings.HasPrefix(fi.Name(), ".") {
					continue
				}

				// skip PubDir changes
				if filepath.HasPrefix(evt.Name, oB.PubDir) {
					continue
				}

				fmt.Println(evt)

				_, _, err = oB.buildFile(evt.Name, vinit, mL2D, mLo)
				if err != nil {
					ErrRpt(err, oB.IsTty)
					continue
				}

				// TODO: track dependency graph, only re-build dirty
				// parse layouts, render nested templates
				oB.ApplyLayouts(mL2D, mLo, func(err error, msg string) {
					ErrRpt(errors.WithMessage(err, msg), oB.IsTty)
				})
			}
		case err, ok := <-pW.Errors:
			ErrRpt(err, oB.IsTty)
			if !ok {
				return nil
			}
		}
	}

	return nil
}

func initSite(oB Builder, tgtDir string) error {

	tgtDir, err := filepath.Abs(tgtDir)
	if err != nil {
		return err
	}

	return fs.WalkDir(SiteCfgFS, SiteCfgDirName, func(src string, de fs.DirEntry, err error) error {

		// generate target path from source path
		rel := filepath.FromSlash(strings.TrimPrefix(src, SiteCfgDirName))
		dst := filepath.Join(tgtDir, rel)

		// make directory
		if de.IsDir() {
			err := os.Mkdir(dst, oB.DirMode)
			if os.IsExist(err) {
				return nil
			}
			return err
		}

		// report progress
		fmt.Println(dst)

		// open dst (file must not exist)
		fDst, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, oB.FileMode)
		if err != nil {
			if os.IsExist(err) {
				err = errors.New("Init does not overwrite existing files.  Operation terminated.")
			}
			return err
		}
		defer fDst.Close()

		// open src
		fSrc, err := SiteCfgFS.Open(src)
		if err != nil {
			return err
		}
		defer fSrc.Close()

		// copy from src to dst
		_, err = io.Copy(fDst, fSrc)
		return err
	})
}

func main() {

	bIsTty := isatty.IsTerminal(os.Stdout.Fd())

	var err error
	defer func() {
		ErrRpt(err, bIsTty)
		if err != nil {
			os.Exit(1)
		}
	}()

	oB := Builder{
		DirMode:  0755,
		FileMode: 0644,
		IsTty:    bIsTty,
	}

	var szDelim string
	flag.StringVar(&szDelim, "vdelim", DEFAULT_DELIM, "vars/body delimiter")
	flag.BoolVar(&oB.IsShowVars, "vshow", false, "show document vars for file(s) on build")

	var httpPort int
	flag.BoolVar(&oB.IsWatchMode, "watch", false, "rebuild on file change")
	flag.IntVar(&httpPort, "port", 8080, "HTTP port for watch-mode web server")

	bInit := false
	flag.BoolVar(&bInit, "init", false, "create a new site configuration inside the given directory")

	// help message
	var iWri io.Writer = os.Stdout
	flag.CommandLine.SetOutput(iWri)
	flag.Usage = func() {

		fmt.Fprintf(iWri, `USAGE
  webjot [FLAG]... <source dir>

Static site template renderer.
Templates in <source dir> are rendered to the '<source dir>/%s' directory.

The default delimiters '{{' and '}}' are escaped thus:

  {{ "{{" }}
  {{ "}}" }}

FLAG
`, PUBDIR)
		flag.PrintDefaults()

		fmt.Fprint(iWri, `
EXAMPLES
  create new site:
    webjot -init <new_site_path>

  re-build site:
    webjot <site_source_path>

  update site contents w/ live refresh:
    webjot -watch <site_source_path>
`)

		fmt.Fprint(iWri, "\n")
	}
	flag.Parse()
	args := flag.Args()

	if len(szDelim) == 0 {
		err = errors.New("empty vars/body delimiter")
		return
	} else {
		if err = oB.SetHdrDelim(szDelim); err != nil {
			return
		}
	}

	var tgt string
	if len(args) > 0 {
		tgt = args[0]
	}

	// create new site
	if bInit {
		err = initSite(oB, tgt)
		return
	}

	if len(tgt) == 0 {
		if tgt, err = os.Getwd(); err != nil {
			return
		}
	}

	// lookup conf dir parent
	conf, err := searchDirAncestors(tgt, CFGDIR)
	if err != nil {
		return
	}

	webRoot := filepath.Dir(conf)

	// settings
	oB.PubDir = filepath.Join(webRoot, PUBDIR)
	oB.ConfDir = conf

	// absolute paths
	for _, ps := range []*string{&tgt, &oB.PubDir, &oB.ConfDir} {
		if *ps, err = filepath.Abs(*ps); err != nil {
			return
		}
	}

	// initial site build
	mL2D, mLo, err := buildAll(oB, tgt)
	if err != nil {
		return
	}
	oB.ApplyLayouts(mL2D, mLo, func(err error, msg string) {
		ErrRpt(errors.WithMessage(err, msg), oB.IsTty)
	})

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
				ErrRpt(OpenBrowser("http://localhost:"+szPort), bIsTty)
			}()

			// start http server
			e2 := http.ListenAndServe(":"+szPort, nil)
			if e2 != nil {
				ErrRpt(e2, bIsTty)
			}

		}()

		// rebuild on change
		err = watch(oB, webRoot, mL2D, mLo)

	}
}
