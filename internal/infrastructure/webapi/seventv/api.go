package seventv

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	maxDownloadSize = 10 << 20
	defaultTimeout  = time.Second * 10
)

func New(saveDir string) *API {
	client := &http.Client{Timeout: defaultTimeout}

	return &API{
		saveDir: saveDir,
		client:  client,
	}
}

type API struct {
	saveDir string
	client  *http.Client
}

func (a *API) DownloadWebp(emoteID string) (string, error) {
	const errMsg = "SevenTvAPI.DownloadWebp"

	cdnUrl := fmt.Sprintf("https://cdn.7tv.app/emote/%s/4x.webp", emoteID)

	resp, err := a.client.Get(cdnUrl)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = errors.New("response status code " + resp.Status)

		return "", errors.Wrap(err, errMsg)
	}

	if resp.Header.Get("Content-Type") != "image/webp" {
		err = errors.New("invalid content type")

		return "", errors.Wrap(err, errMsg)
	}

	limitedReader := io.LimitReader(resp.Body, maxDownloadSize+1)

	outPath := filepath.Join(a.saveDir, uuid.NewString()+".webp")

	outFile, err := os.Create(outPath)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, limitedReader)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	if written > maxDownloadSize {
		_ = os.Remove(outPath)
		err = fmt.Errorf("input file too large (>%dMB)", maxDownloadSize>>20)

		return "", errors.Wrap(err, errMsg)
	}

	return outPath, nil
}
