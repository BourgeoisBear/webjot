package main

import (
	"io"
	"os"
	"regexp"
)

type DocProps struct {
	SrcPath           string
	DstPath           string
	Info              os.FileInfo
	Source            []byte
	Vars              Vars
	NonConformingKeys []string
}

/*
Retrieves file contents.
Parses YAML above first headerDelim into DocProps.Vars.
Returns text below headerDelim as DocProps.Source.
If no headerDelim is found, full contents are returned in DocProps.Source.
*/
func GetDoc(path string, rxHdrDelim *regexp.Regexp) (DocProps, error) {

	ret := DocProps{SrcPath: path, Vars: make(Vars)}
	pf, err := os.Open(path)
	if err != nil {
		return ret, err
	}
	defer pf.Close()

	ret.Info, err = pf.Stat()
	if err != nil {
		return ret, err
	}

	ret.Source, err = io.ReadAll(pf)
	if err != nil {
		return ret, err
	}

	// skip header/body processing on empty delimiter
	if rxHdrDelim == nil {
		return ret, nil
	}

	// find `headerDelim`
	// NOTE: deferring CR/CR-LF/first-line/last-line handling to regexp
	hdrPos := rxHdrDelim.FindIndex(ret.Source)

	// not found, leave body as-is
	if hdrPos == nil {
		return ret, err
	}

	// found, parse vars from header info
	ret.Vars, ret.NonConformingKeys, err = ParseHeaderVars(ret.Source[:hdrPos[0]])
	ret.Source = ret.Source[hdrPos[1]:]
	return ret, err
}
