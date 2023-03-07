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

	"github.com/pkg/errors"
)

type DownloadItem struct {
	Host    string
	SrcPath string
	DstPath string
}

type RIRKey int

const (
	RkRipe RIRKey = iota
	RkLacnic
	RkAfrinic
	RkApnic
	RkArin
	RkMAX
)

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

func GetRIRDownloadItems(dbPath string) map[RIRKey]DownloadItem {
	return map[RIRKey]DownloadItem{
		RkRipe:    defaultRIRItem(dbPath, "ftp.ripe.net", "ripencc"),
		RkLacnic:  defaultRIRItem(dbPath, "ftp.lacnic.net", "lacnic"),
		RkAfrinic: defaultRIRItem(dbPath, "ftp.afrinic.net", "afrinic"),
		RkApnic:   defaultRIRItem(dbPath, "ftp.apnic.net", "apnic"),
		RkArin:    defaultRIRItem(dbPath, "ftp.arin.net", "arin"),
	}
}

// download extended delegations list from an RIR
func DownloadAll(out io.Writer, oR DownloadItem, dirTmp string) error {

	const DEBUG = false

	url := fmt.Sprintf("https://%s/%s", oR.Host, oR.SrcPath)

	if DEBUG {
		url = "http://localhost:9090/delegated-afrinic-extended-latest.txt"
	}

	fmt.Fprintln(out, "\x1b[1mDOWNLOADING:\x1b[0m", url)

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
	var nLen int64 = -1
	szLen := rsp.Header.Get("Content-Length")
	if len(szLen) > 0 {
		nLen, err = strconv.ParseInt(szLen, 10, 64)
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
	var n int64
	for {
		ntmp, err := io.CopyN(gzF, rsp.Body, 1024*64)
		if err != nil && err != io.EOF {
			return err
		}
		n += ntmp

		if nLen > 0 {
			pct := (float32(n) / float32(nLen)) * 100.0
			fmt.Fprintf(out, "\t\x1b[2K%d/%d (%5.1f%%)\r", n, nLen, pct)
		} else {
			fmt.Fprintf(out, "\t\x1b[2K%d\r", n)
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
