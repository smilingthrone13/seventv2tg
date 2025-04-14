package seventv

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const maxDownloadSize = 15 << 20 // 15 MB

func NewAPI(saveDir string) *API {
	client := &http.Client{
		Timeout: time.Second * 10,
	}

	return &API{
		saveDir: saveDir,
		client:  client,
	}
}

type API struct {
	saveDir string
	client  *http.Client
}

func (a *API) DownloadWebp(emoteUrl string) (string, error) {
	const errMsg = "SevenTvAPI.DownloadWebp"

	u, err := url.ParseRequestURI(emoteUrl)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	parts := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
	if len(parts) != 2 {
		err = errors.New("invalid emote url")

		return "", errors.Wrap(err, errMsg)
	}

	cdnUrl := fmt.Sprintf("https://cdn.7tv.app/emote/%s/4x.webp", parts[1])

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
		err = errors.New("input file too large (>15MB)")

		return "", errors.Wrap(err, errMsg)
	}

	return outPath, nil
}
