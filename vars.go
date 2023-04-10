package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Vars map[string]string

func (mV Vars) ClearDelims() {
	delete(mV, "ldelim")
	delete(mV, "rdelim")
}

func (mV Vars) PrettyPrint(iWri io.Writer, bColor bool) error {

	klen := 0
	for k, _ := range mV {
		l := len(k)
		if l > klen {
			klen = l
		}
	}

	lblFmt := "%" + strconv.Itoa(klen) + "s"
	if bColor {
		lblFmt = "\x1b[92;1m" + lblFmt + "\x1b[0m"
	}

	for k, v := range mV {
		_, err := fmt.Fprintf(iWri, "  "+lblFmt+": %s\n", k, v)
		if err != nil {
			return err
		}
	}

	return nil
}

/*
split each line inside the header section into (key, value) pairs
append each found pair into mV
*/
func ParseHeaderVars(header []byte) Vars {

	/*
		TODO:
			- invalid header checking/reporting ( non comment, no key, not envvar
			compatible [A-Z][a-z]_ ) on parse
			- force header to use env-var compatible keys
			- accept newlines
			- handle \r\n
	*/
	lines := bytes.Split(header, []byte("\n"))
	mV := make(Vars, len(lines))
	for _, line := range lines {

		// skip comment lines beginning with #
		if bytes.HasPrefix(line, []byte("#")) {
			continue
		}

		key, val, found := bytes.Cut(line, []byte(":"))
		if found {
			sk := strings.TrimSpace(string(key))
			sv := strings.TrimSpace(string(val))
			mV[sk] = sv
		}
	}
	return mV
}

/*
GetEnvGlobals returns list of global OS environment variables that start
with ENVVAR_PREFIX as Vars, so the values can be used inside templates
*/
func GetEnvGlobals() Vars {
	ret := Vars{}
	for _, e := range os.Environ() {
		pair := strings.Split(e, "=")
		if len(pair) < 2 {
			continue
		}
		if !strings.HasPrefix(pair[0], ENVVAR_PREFIX) {
			continue
		}
		k := strings.TrimPrefix(pair[0], ENVVAR_PREFIX)
		if len(k) > 0 {
			ret[strings.ToLower(k)] = pair[1]
		}
	}
	// omit [ldelim, rdelim], since those are per-template
	ret.ClearDelims()
	return ret
}

func MergeVars(sv ...Vars) Vars {
	ret := make(Vars)
	for ix := range sv {
		for k, v := range sv[ix] {
			ret[k] = v
		}
	}
	return ret
}
