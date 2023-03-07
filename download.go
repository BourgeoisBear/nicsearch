package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

type RIR struct {
	Host     string
	Path     string
	Filename string
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

func defaultRIR(host, key string) RIR {
	return RIR{
		Host:     host,
		Path:     fmt.Sprintf("pub/stats/%s", key),
		Filename: fmt.Sprintf("delegated-%s-extended-latest", key),
	}
}

func GetRIRs() map[RIRKey]RIR {
	return map[RIRKey]RIR{
		RkRipe:    defaultRIR("ftp.ripe.net", "ripencc"),
		RkLacnic:  defaultRIR("ftp.lacnic.net", "lacnic"),
		RkAfrinic: defaultRIR("ftp.afrinic.net", "afrinic"),
		RkApnic:   defaultRIR("ftp.apnic.net", "apnic"),
		RkArin:    defaultRIR("ftp.arin.net", "arin"),
	}
}

// download extended delegations list from an RIR
func DownloadRIRD(oR RIR, dirTmp, dirDst string) error {

	const DEBUG = false

	url := fmt.Sprintf("https://%s/%s/%s", oR.Host, oR.Path, oR.Filename)

	if DEBUG {
		url = "http://localhost:9090/delegated-afrinic-extended-latest.txt"
	}

	fmt.Println("\x1b[1mDOWNLOADING:\x1b[0m", url)

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
	pF, err := os.CreateTemp(dirTmp, oR.Filename+"-*.gz")
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
			err = os.Rename(tmpname, dirDst+"/"+oR.Filename+".gz")
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
			fmt.Printf("\t\x1b[2K%d/%d (%5.1f%%)\r", n, nLen, pct)
		} else {
			fmt.Printf("\t\x1b[2K%d\r", n)
		}

		if DEBUG {
			time.Sleep(time.Millisecond * 10)
		}

		if err == io.EOF {
			fmt.Println("")
			break
		}
	}

	return nil
}
