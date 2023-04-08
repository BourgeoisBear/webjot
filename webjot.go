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
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
)

const CFGDIR = ".webjot"
const PUBDIR = ".pub"

//go:embed all:default_conf
var defaultSiteCfg embed.FS

func build2(oB Builder, path string, iWri io.Writer, mV Vars) error {
	err := oB.build(path, iWri, mV)
	if err != nil && err != fs.SkipDir {
		err = errors.WithMessage(err, path)
	}
	return err
}

func buildAll(oB Builder, srcDir string) error {

	vars := globals()

	// recurse through source dir
	wdFunc := func(path string, info fs.DirEntry, eWalk error) error {
		if eWalk != nil {
			return errors.WithMessage(eWalk, path)
		} else {
			return build2(oB, path, nil, vars)
		}
	}

	return filepath.WalkDir(srcDir, wdFunc)
}

func watch(oB Builder, srcDir string) error {

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
						errRpt(buildAll(oB, filepath.Dir(oB.ConfDir)), oB.IsTty)
					} else {
						// TODO: handle skipdir err
						// otherwise, rebuild dirty file only
						errRpt(build2(oB, evt.Name, nil, vars), oB.IsTty)
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

func initSite(oB Builder, tgtDir string) error {

	tgtDir, err := filepath.Abs(tgtDir)
	if err != nil {
		return err
	}

	cfgDir := filepath.Join(tgtDir, CFGDIR)
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

	// prepend CFGDIR to $PATH, so plugins will be found before OS commands
	// p := os.Getenv("PATH")
	// p = ZSDIR + ":" + p
	// os.Setenv("PATH", p)

	if err = buildAll(oB, tgt); err != nil {
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
		err = watch(oB, webRoot)
	}
}
