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
func parseVarsHeader(header []byte, mV Vars) {
	lines := bytes.Split(header, []byte("\n"))
	for _, line := range lines {
		key, val, found := bytes.Cut(line, []byte(":"))
		if found {
			sk := strings.TrimSpace(string(key))
			sv := strings.TrimSpace(string(val))
			mV[sk] = sv
		}
	}
}

/*
globals returns list of global OS environment variables that start
with ZS_ prefix as Vars, so the values can be used inside templates
*/
func globals() Vars {
	vars := Vars{}
	for _, e := range os.Environ() {
		pair := strings.Split(e, "=")
		if strings.HasPrefix(pair[0], "ZS_") {
			vars[strings.ToLower(pair[0][3:])] = pair[1]
		}
	}
	return vars
}
