package colwriter

import (
	"fmt"
	"io"
	"strings"
)

type Cfg struct {
	Spacer string
	Pad    bool
}

type ColCfg struct {
	Title string
	Wid   uint16
	Rt    bool
}

type RowWriter func(io.Writer, ...interface{}) (int, error)

func (wc Cfg) NewWriterFuncs(sCfg []ColCfg) RowWriter {

	sParts := make([]string, len(sCfg))
	for i, cfg := range sCfg {
		if wc.Pad && (cfg.Wid > 0) {
			if cfg.Rt {
				sParts[i] = fmt.Sprintf("%%%d.%ds", cfg.Wid, cfg.Wid)
			} else {
				sParts[i] = fmt.Sprintf("%%-%d.%ds", cfg.Wid, cfg.Wid)
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

	return func(iWri io.Writer, sFields ...interface{}) (int, error) {
		return fmt.Fprintf(iWri, szFmt, sFields...)
	}
}
