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

type Delims struct {
	L, R string
}

func (mV Vars) GetDelims() Delims {
	ret := Delims{L: "{{", R: "}}"}
	ol, or := mV.GetStr("ldelim"), mV.GetStr("rdelim")
	if len(ol) > 0 {
		ret.L = ol
	}
	if len(or) > 0 {
		ret.R = or
	}
	return ret
}

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

func (mV Vars) PrettyPrint(
	iWri io.Writer, nonConforming []string, rxExcl *regexp.Regexp, bColor bool,
) error {

	// get as a sorted array for consistent ordering
	pairs := mV.GetPairs(true)

	// skip vars with keys that match rxExcl
	if rxExcl != nil {
		filtered := make([]VarPair, 0, len(pairs))
		for _, p := range pairs {
			if rxExcl.MatchString(p.K) {
				continue
			} else {
				filtered = append(filtered, p)
			}
		}
		pairs = filtered
	}

	// find max key len
	klen := 0
	for _, p := range pairs {
		if l := len(p.K); l > klen {
			klen = l
		}
	}

	// construct format string from max key len
	var AON, AOFF string
	if bColor {
		AON, AOFF = "\x1b[92;1m", "\x1b[0m"
	}

	// write conforming key:val pairs
	for _, p := range pairs {
		_, err := fmt.Fprintf(iWri,
			"%s%"+strconv.Itoa(klen)+"s%s: %+v\n",
			AON, p.K, AOFF, p.V,
		)
		if err != nil {
			return err
		}
	}

	// write non-conforming keys
	if len(nonConforming) > 0 {
		if bColor {
			AON = "\x1b[91;1m"
		}
		_, err := fmt.Fprintf(iWri,
			"%sWARNING%s: non-conforming keys = %v\n",
			AON, AOFF, nonConforming,
		)
		return err
	}

	return nil
}

var rxEnvVarName *regexp.Regexp

func init() {
	rxEnvVarName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
}

/*
splits each line inside the header section into (key, value) pairs
adds each found pair into a Vars map and returns it.

any non-conforming keys are returned as []string.
*/
func ParseHeaderVars(header []byte) (Vars, []string, error) {
	// parse as YAML
	ret := make(Vars)
	if err := yaml.Unmarshal(header, &ret); err != nil {
		return nil, nil, err
	}
	// drop all non-conforming keys
	var sNC []string
	for k := range ret {
		if !rxEnvVarName.MatchString(k) {
			sNC = append(sNC, k)
			delete(ret, k)
		}
	}
	return ret, sNC, nil
}

/*
returns global OS environment variables that start with ENVVAR_PREFIX as Vars,
so the values can be used inside templates
*/
func GetEnvGlobals() Vars {
	ret := make(Vars)
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
