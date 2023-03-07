package main

import "io"

type ColCfg struct {
	Wid int
	Rt  bool
}

type ColWriterCfg struct {
	Spacer []byte
	Pad    bool
}

type ColWriter struct {
	buf []byte
}

func (cw *ColWriter) WriteCols(
	out io.Writer,
	wriCfg ColWriterCfg,
	colCfg []ColCfg,
	sTxt ...[]byte,
) error {

	// reset line buffer
	if cw.buf == nil {
		cw.buf = make([]byte, 0, 80)
	} else {
		cw.buf = cw.buf[:0]
	}

	nLastCol := len(sTxt) - 1

	for ix, col := range colCfg {

		if ix >= len(sTxt) {
			break
		}

		txt := sTxt[ix]

		pad := col.Wid - len(txt)

		if !col.Rt {
			cw.buf = append(cw.buf, txt...)
		}

		// padding
		if wriCfg.Pad {
			for i := pad; i > 0; i -= 1 {
				cw.buf = append(cw.buf, ' ')
			}
		}

		if col.Rt {
			cw.buf = append(cw.buf, txt...)
		}

		// column separator
		if (len(wriCfg.Spacer) > 0) && (ix < nLastCol) {
			cw.buf = append(cw.buf, wriCfg.Spacer...)
		}
	}

	cw.buf = append(cw.buf, '\n')
	_, err := out.Write(cw.buf)
	return err
}
