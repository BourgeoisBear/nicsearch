package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/BourgeoisBear/nicsearch/rdap"
	"github.com/pkg/errors"
)

type DownloadItem struct {
	Host    string
	SrcPath string
	DstPath string
}

func defaultRIRItem(dbPath, host, key string) DownloadItem {
	return DownloadItem{
		Host:    host,
		SrcPath: fmt.Sprintf("pub/stats/%[1]s/delegated-%[1]s-extended-latest", key),
		DstPath: filepath.Join(
			dbPath,
			fmt.Sprintf("delegated-%s-extended-latest.txt.gz", key),
		),
	}
}

func GetRIRDownloadItems(dbPath string) map[rdap.RIRKey]DownloadItem {
	return map[rdap.RIRKey]DownloadItem{
		rdap.RkRipe:    defaultRIRItem(dbPath, "ftp.ripe.net", "ripencc"),
		rdap.RkLacnic:  defaultRIRItem(dbPath, "ftp.lacnic.net", "lacnic"),
		rdap.RkAfrinic: defaultRIRItem(dbPath, "ftp.afrinic.net", "afrinic"),
		rdap.RkApnic:   defaultRIRItem(dbPath, "ftp.apnic.net", "apnic"),
		rdap.RkArin:    defaultRIRItem(dbPath, "ftp.arin.net", "arin"),
	}
}

// download extended delegations list from an RIR
func (m *Modes) DownloadAll(
	out io.Writer, oR DownloadItem, dirTmp string,
) error {

	const DEBUG = false

	url := fmt.Sprintf("https://%s/%s", oR.Host, oR.SrcPath)

	if DEBUG {
		url = "http://localhost:9090/delegated-afrinic-extended-latest.txt"
	}

	_, err := m.AnsiMsg(os.Stderr, "DOWNLOADING", url, []uint8{1, 96})
	if err != nil {
		return err
	}

	// request list
	rsp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer rsp.Body.Close()

	// error non non-200
	if rsp.StatusCode != 200 {
		return fmt.Errorf("%s: %s", rsp.Status, url)
	}

	// get reported file length
	var nBytesTot int64 = -1
	szLen := rsp.Header.Get("Content-Length")
	if len(szLen) > 0 {
		nBytesTot, err = strconv.ParseInt(szLen, 10, 64)
		if err != nil {
			return errors.WithMessage(err, "parsing content length")
		}
	}

	// create tempfile for download
	dstFname := filepath.Base(oR.DstPath)
	pF, err := os.CreateTemp(dirTmp, "*-"+dstFname)
	if err != nil {
		return err
	}

	// gzip contents
	gzF := gzip.NewWriter(pF)

	// cleanup
	defer func() {
		tmpname := pF.Name()
		gzF.Close()
		pF.Close()
		if err != nil {
			// delete tempfile
			os.Remove(tmpname)
		} else {
			// move to correct path
			err = os.Rename(tmpname, oR.DstPath)
		}
	}()

	// save downloaded file, report progress
	var nCopied int64
	for {
		ntmp, err := io.CopyN(gzF, rsp.Body, 1024*64)
		if err != nil && err != io.EOF {
			return err
		}
		nCopied += ntmp

		if nBytesTot > 0 {
			pct := (float64(nCopied) / float64(nBytesTot)) * 100.0
			fmt.Fprintf(
				out,
				"\x1b[2K(%5.1f%%) %9d/%-9d bytes\r",
				pct, nCopied, nBytesTot,
			)
		} else {
			fmt.Fprintf(out, "\x1b[2K%d bytes\r", nCopied)
		}

		if DEBUG {
			time.Sleep(time.Millisecond * 10)
		}

		if err == io.EOF {
			fmt.Fprintln(out, "")
			break
		}
	}

	return nil
}
