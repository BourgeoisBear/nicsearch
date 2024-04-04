package main

import (
	"fmt"
	"io"
	"strings"
)

type ColWriterCfg struct {
	Spacer string
	Pad    bool
}

type ColCfg struct {
	Title string
	Wid   int
	Rt    bool
}

type ColWriterFunc func(...interface{}) (int, error)

func (wc ColWriterCfg) NewWriterFunc(iWri io.Writer, sCfg []ColCfg) ColWriterFunc {

	sParts := make([]string, len(sCfg))
	for i, cfg := range sCfg {
		if wc.Pad {
			if cfg.Rt {
				sParts[i] = fmt.Sprintf("%%%ds", cfg.Wid)
			} else {
				sParts[i] = fmt.Sprintf("%%-%ds", cfg.Wid)
			}
		} else {
			sParts[i] = "%s"
		}
	}

	spcr := wc.Spacer
	if wc.Pad {
		spcr = " " + wc.Spacer + " "
	}

	szFmt := strings.Join(sParts, spcr) + "\n"

	return func(sFields ...interface{}) (int, error) {
		return fmt.Fprintf(iWri, szFmt, sFields...)
	}
}
