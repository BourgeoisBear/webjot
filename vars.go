package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type VarPair struct {
	K string
	V interface{}
}

type Vars map[string]interface{}

func (mV Vars) GetStr(k string) string {
	if i := mV[k]; i != nil {
		if s, ok := i.(string); ok {
			return s
		} else {
			return fmt.Sprint(i)
		}
	}
	return ""
}

func (mV Vars) GetPairs(bSort bool) []VarPair {
	ret := make([]VarPair, 0, len(mV))
	for k, v := range mV {
		ret = append(ret, VarPair{K: k, V: v})
	}
	if bSort {
		sort.Slice(ret, func(i, j int) bool {
			return ret[i].K < ret[j].K
		})
	}
	return ret
}

func (mV Vars) ClearDelims() {
	delete(mV, "ldelim")
	delete(mV, "rdelim")
}

func (mV Vars) PrettyPrint(iWri io.Writer, bColor bool) error {
	// get as a sorted array for consistent ordering
	pairs := mV.GetPairs(true)
	// find max key len
	klen := 0
	for _, p := range pairs {
		l := len(p.K)
		if l > klen {
			klen = l
		}
	}
	// construct format string from max key len
	lblFmt := "%" + strconv.Itoa(klen) + "s"
	if bColor {
		lblFmt = "\x1b[92;1m" + lblFmt + "\x1b[0m"
	}
	// write values
	for _, p := range pairs {
		_, err := fmt.Fprintf(iWri, "  "+lblFmt+": %+v\n", p.K, p.V)
		if err != nil {
			return err
		}
	}
	return nil
}

var rxEnvVarName *regexp.Regexp

func init() {
	rxEnvVarName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
}

/*
splits each line inside the header section into (key, value) pairs
adds each found pair into a Vars map and returns it
*/
func ParseHeaderVars(header []byte) (Vars, error) {
	// parse as YAML
	mV := make(Vars)
	if err := yaml.Unmarshal(header, &mV); err != nil {
		return nil, err
	}
	// drop all non-conforming keys
	for k := range mV {
		if !rxEnvVarName.MatchString(k) {
			// TODO: report issue to STDERR as prettyprint
			fmt.Fprintf(os.Stderr, "INVALID HEADER KEY `%s`: OMITTING\n", k)
			delete(mV, k)
		}
	}
	return mV, nil
}

/*
returns global OS environment variables that start with ENVVAR_PREFIX as Vars,
so the values can be used inside templates
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
