package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ErrMsg struct {
	err error
	msg string
}

func EWrap(err error, msg string) ErrMsg {
	return ErrMsg{err: err, msg: msg}
}

func (e ErrMsg) Error() string {
	return fmt.Sprintf("[%s] %v", e.msg, e.err)
}

func (e ErrMsg) Unwrap() error {
	return e.err
}

func (e ErrMsg) Message() string {
	return e.msg
}

func ErrRpt(err error, isTty bool) {
	if err != nil {
		if isTty {
			fmt.Fprint(os.Stderr, "\x1b[91;1mERROR\x1b[0m: ")
		} else {
			fmt.Fprint(os.Stderr, "ERROR: ")
		}

		if ew, ok := err.(ErrMsg); ok {
			msg := ew.Message()
			hd, err := os.UserHomeDir()
			if err == nil {
				if strings.HasPrefix(msg, hd) {
					msg = "~" + strings.TrimPrefix(msg, hd)
				}
			}
			fmt.Fprintf(os.Stderr, "[%s] %v\n", msg, ew.Unwrap())
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}
}

/*
copies if:
  - destination does not exist, OR
  - destination has different size than source, OR
  - destination has different mtime than source
*/
func CopyOnDirty(dst, src string, fileMode os.FileMode) error {

	fSrc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fSrc.Close()

	iSrc, err := fSrc.Stat()
	if err != nil {
		return err
	}

	fnCopy := func() error {
		err := func() error {
			flags := os.O_CREATE | os.O_TRUNC | os.O_WRONLY
			fDst, err := os.OpenFile(dst, flags, fileMode)
			if err != nil {
				return err
			}
			defer fDst.Close()
			_, err = io.Copy(fDst, fSrc)
			return err
		}()
		if err != nil {
			return err
		}
		mtime := iSrc.ModTime()
		return os.Chtimes(dst, mtime, mtime)
	}

	iDst, err := os.Stat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			return fnCopy()
		} else {
			return err
		}
	}

	// copy if modified (fuzzy check)
	if (iDst.Size() != iSrc.Size()) ||
		(iDst.ModTime() != iSrc.ModTime()) {
		return fnCopy()
	}

	return nil
}

func searchDirAncestors(start, needle string) (found string, err error) {

	start, err = filepath.Abs(start)
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			err = EWrap(err,
				fmt.Sprintf("search for `%s` in `%s` ancestors", needle, start),
			)
		}
	}()

	cur := start
	inf, err := os.Stat(cur)
	if err != nil {
		return
	}

	if !inf.IsDir() {
		cur = filepath.Dir(cur)
	}

	if filepath.Base(cur) == needle {
		found = cur
		return
	}

	for {

		// check siblings
		var sD []os.DirEntry
		if sD, err = os.ReadDir(cur); err != nil {
			return
		}
		for ix := range sD {
			if sD[ix].IsDir() {
				sname := sD[ix].Name()
				if sname == needle {
					found = filepath.Join(cur, sname)
					return
				}
			}
		}

		// up a dir
		cur = filepath.Dir(cur)

		// exit at root
		if strings.HasSuffix(cur, string(filepath.Separator)) {
			break
		}
	}

	err = os.ErrNotExist
	return
}

func OpenBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}

func HeadHandler(hDir http.Dir, oHandler http.Handler, rwm *sync.RWMutex) http.Handler {

	return http.HandlerFunc(func(iWri http.ResponseWriter, pRq *http.Request) {

		// mutexing between HTTP:HEAD and writes to /.pub/
		// (for live.js issues w/ files in the process of being written)
		rwm.RLock()
		defer rwm.RUnlock()

		if pRq.Method != "HEAD" {
			oHandler.ServeHTTP(iWri, pRq)
			return
		}

		var err error

		defer func() {
			if err == nil {
				iWri.WriteHeader(http.StatusOK)
				return
			}
			iWri.WriteHeader(http.StatusInternalServerError)
			iWri.Write([]byte(err.Error()))
		}()

		// PROCESS URI
		szPath := path.Clean(pRq.URL.Path)
		if szPath == "/" {
			szPath = "/index.html"
		}

		// OPEN FILE
		oFile, err := hDir.Open(szPath)
		if err != nil {
			return
		}
		defer oFile.Close()

		// GET THE CONTENT-TYPE OF THE FILE
		FileHeader := make([]byte, 512)
		oFile.Read(FileHeader)
		FileContentType := http.DetectContentType(FileHeader)

		// GET FILE SIZE
		FileStat, err := oFile.Stat()
		if err != nil {
			return
		}

		// WRITE HEADER
		iWri.Header().Set("Content-Type", FileContentType)
		iWri.Header().Set("Content-Length", strconv.FormatInt(FileStat.Size(), 10))
		iWri.Header().Set("Last-Modified", FileStat.ModTime().Format(time.RFC1123))
	})
}
