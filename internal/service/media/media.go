package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"seventv2tg/internal/domain"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	defaultFramerate = 30
	defaultBitRate   = 250
	overlayedBitRate = defaultBitRate + 150 // since we'll have more details we might also increase bitrate

	maxResultSize = 256 << 10

	frameMask = "frame_%03d.png"

	autoHeight = 0
	autoWidth  = 0
)

type (
	videoStream struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	}
	ffprobeOutput struct {
		Streams []videoStream `json:"streams"`
	}
)

func NewMediaConverter(jobsDir, resDir string, videoRendererThreads int) *Converter {
	return &Converter{
		jobsDir:              jobsDir,
		resDir:               resDir,
		videoRendererThreads: videoRendererThreads,
	}
}

type Converter struct {
	jobsDir              string
	resDir               string
	videoRendererThreads int
}

func (c *Converter) ConvertToVideo(inpFilePath string) (resPath string, err error) {
	const errMsg = "Converter.ConvertToVideo"

	jobID := uuid.NewString()

	defer func() {
		errFs := os.RemoveAll(filepath.Join(c.jobsDir, jobID))
		if errFs != nil {
			slog.Error("Failed to remove job folder", slog.String("jobID", jobID), slog.Any("err", err))
		}
	}()

	framesDirPath := filepath.Join(c.jobsDir, jobID, "frames")

	err = os.MkdirAll(framesDirPath, os.ModePerm)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	framerate, _, err := c.getVideoInfo(inpFilePath)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	err = c.createSequence(inpFilePath, framesDirPath, frameMask)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	resPath = filepath.Join(c.resDir, jobID+".webm")

	err = c.createVideoFromSequence(filepath.Join(framesDirPath, frameMask), resPath, framerate, autoHeight, autoWidth)
	if err != nil {
		_ = os.Remove(resPath)
		return "", errors.Wrap(err, errMsg)
	}

	return resPath, nil
}

func (c *Converter) OverlayVideos(inpFilePaths []string) (resPath string, err error) {
	const errMsg = "Converter.OverlayVideos"

	jobID := uuid.NewString()
	resPath = filepath.Join(c.resDir, jobID+".webm")

	defer func() {
		errFs := os.RemoveAll(filepath.Join(c.jobsDir, jobID))
		if errFs != nil {
			slog.Error("Failed to remove job folder", slog.String("jobID", jobID), slog.Any("err", err))
		}
	}()

	var layers []domain.EmoteLayer

	height, width := autoHeight, autoWidth

	for i := range inpFilePaths {
		framesDirPath := filepath.Join(c.jobsDir, jobID, fmt.Sprintf("frames-%d", i))

		err = os.MkdirAll(framesDirPath, os.ModePerm)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		framerate, duration, err := c.getVideoInfo(inpFilePaths[i])
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		err = c.createSequence(inpFilePaths[i], framesDirPath, frameMask)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		seqPath := filepath.Join(framesDirPath, frameMask)
		webmPath := filepath.Join(c.jobsDir, jobID, fmt.Sprintf("layer-%d.webm", i))

		err = c.createVideoFromSequence(seqPath, webmPath, framerate, width, height)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		// use base layer dimensions as reference
		if i == 0 {
			width, height, err = c.getVideoDimensions(webmPath)
			if err != nil {
				return "", errors.Wrap(err, errMsg)
			}
		}

		layers = append(layers, domain.EmoteLayer{
			WebmPath: webmPath,
			Duration: duration,
		})
	}

	err = c.createOverlayedVideo(layers, resPath)
	if err != nil {
		_ = os.Remove(resPath)
		return "", errors.Wrap(err, errMsg)
	}

	return resPath, nil
}

func (c *Converter) createSequence(inpPath, outPath, frameMask string) error {
	outSeqPath := filepath.Join(outPath, frameMask)

	cmd := exec.Command(
		"magick",
		inpPath,
		"-coalesce",
		"+repage",
		outSeqPath,
	)
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return errors.Wrap(err, "createSequence")
	}

	frames, err := os.ReadDir(outPath)
	if err != nil {
		return errors.Wrap(err, "createSequence")
	}

	for i := range frames {
		filePath := filepath.Join(outPath, frames[i].Name())

		isEmpty, err := c.isEmptyFrame(filePath)
		if err != nil {
			return errors.Wrap(err, "createSequence")
		}

		if !isEmpty {
			continue
		}

		err = c.processEmptyFrame(filePath)
		if err != nil {
			return errors.Wrap(err, "createSequence")
		}
	}

	return nil
}

func (c *Converter) isEmptyFrame(filepath string) (bool, error) {
	cmd := exec.Command(
		"magick",
		filepath,
		"-format", "%[fx:mean]",
		"info:",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return false, errors.Wrap(err, "getImageMean")
	}

	meanStr := strings.TrimSpace(out.String())
	mean, err := strconv.ParseFloat(meanStr, 64)
	if err != nil {
		return false, errors.Wrap(err, "getImageMean")
	}

	return mean == 0, nil
}

func (c *Converter) processEmptyFrame(filepath string) error {
	// Костыль. Полностью прозрачные кадры при сборке webm превращаются в черные,
	// поэтому ставим в углу полупрозрачную точку.
	cmd := exec.Command(
		"magick",
		filepath,
		"-stroke", "rgba(255,0,0,0.1)",
		"-strokewidth", "1",
		"-draw", "point 0,0",
		filepath,
	)

	return errors.Wrap(cmd.Run(), "processEmptyFrame")
}

func (c *Converter) createVideoFromSequence(inpPath, outPath string, framerate, width, height int) error {
	const errMessage = "createVideoFromSequence"
	var err error

	bitrate := defaultBitRate

	for {
		err = c.assembleSequence(inpPath, outPath, framerate, bitrate, width, height)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}

		fInfo, err := os.Stat(outPath)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}

		if fInfo.Size() <= maxResultSize {
			return nil
		}

		bitrate, err = c.downscaleVideoParameters(bitrate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}
	}
}

func (c *Converter) createOverlayedVideo(inpLayers []domain.EmoteLayer, outPath string) error {
	const errMessage = "createOverlayedVideo"
	var err error

	bitrate := overlayedBitRate

	for {
		err = c.assembleLayers(inpLayers, outPath, bitrate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}

		fInfo, err := os.Stat(outPath)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}

		if fInfo.Size() <= maxResultSize {
			return nil
		}

		bitrate, err = c.downscaleVideoParameters(bitrate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}
	}
}

func (c *Converter) downscaleVideoParameters(bitrate int) (newBitrate int, err error) {
	const errMessage = "downscaleVideoParameters"
	const (
		bitrateThreshold = 150
		bitrateDropRate  = 20
	)

	if bitrate > bitrateThreshold {
		return bitrate - bitrateDropRate, nil
	}

	err = errors.New("lower quality limit exceeded")

	return 0, errors.Wrap(err, errMessage)
}

func (c *Converter) assembleSequence(inpPath, outPath string, framerate, bitrate, width, height int) error {
	const errMessage = "assembleSequence"

	var scaleStr string
	if height == autoHeight || width == autoWidth {
		scaleStr = "'if(gte(iw,ih),512,-1)':'if(gte(ih,iw),512,-1)'"
	} else {
		scaleStr = fmt.Sprintf("%d:%d", width, height)
	}

	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-loglevel", "error",
		"-framerate", fmt.Sprintf("%d", framerate),
		"-i", inpPath,
		"-vf", "scale="+scaleStr+",format=yuva420p",
		"-c:v", "libvpx-vp9",
		"-b:v", fmt.Sprintf("%dK", bitrate),
		"-auto-alt-ref", "0",
		"-an",
		"-threads", fmt.Sprintf("%d", c.videoRendererThreads),
		"-t", "3",
		outPath,
	)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), errMessage)
}

func (c *Converter) assembleLayers(inpLayers []domain.EmoteLayer, outPath string, bitrate int) error {
	const errMessage = "assembleLayers"

	if len(inpLayers) < 2 {
		return errors.Wrap(errors.New("not enough videos to merge"), errMessage)
	}

	args := []string{
		"-y",
		"-loglevel", "error",
		"-c:v", "libvpx-vp9",
	}

	// Если основа короче, чем оверлей - зацикливаем основу.
	for i := 1; i < len(inpLayers); i++ {
		if inpLayers[i].Duration > inpLayers[0].Duration {
			args = append(args, "-stream_loop", "-1")
			break
		}
	}

	args = append(args, "-i", inpLayers[0].WebmPath)

	overlayString := strings.Builder{}
	prevLayer := "0"

	for i := 1; i < len(inpLayers); i++ {
		inpArgs := []string{
			"-stream_loop", "-1",
			"-c:v", "libvpx-vp9",
			"-i", inpLayers[i].WebmPath,
		}

		args = append(args, inpArgs...)

		currLayer := fmt.Sprintf("tmp%d", i)

		overlayString.WriteString(fmt.Sprintf("[%s][%d] overlay=shortest=1", prevLayer, i))
		if i != len(inpLayers)-1 {
			overlayString.WriteString(" [" + currLayer + "]; ")
		}

		prevLayer = currLayer
	}

	args = append(
		args,
		"-filter_complex", overlayString.String(),
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuva420p",
		"-b:v", fmt.Sprintf("%dK", bitrate),
		"-auto-alt-ref", "0",
		"-an",
		"-t", "3",
		"-threads", fmt.Sprintf("%d", c.videoRendererThreads),
		outPath,
	)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), errMessage)
}

func (c *Converter) getVideoDimensions(inpPath string) (width, height int, err error) {
	const errMessage = "getVideoDimensions"

	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "json",
		inpPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, errMessage)
	}

	var probeOutput ffprobeOutput
	if err = json.Unmarshal(output, &probeOutput); err != nil {
		return 0, 0, errors.Wrap(err, errMessage)
	}

	if len(probeOutput.Streams) == 0 {
		return 0, 0, errors.Wrap(errors.New("no video stream found"), errMessage)
	}

	return probeOutput.Streams[0].Width, probeOutput.Streams[0].Height, nil
}

func (c *Converter) getVideoInfo(inpPath string) (framerate int, duration float64, err error) {
	const errMessage = "getVideoInfo"

	cmd := exec.Command("magick", "identify", "-format", "%T\n", inpPath)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, errMessage)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	totalTime := 0
	for _, line := range lines {
		delay, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return 0, 0, errors.Wrap(err, errMessage)
		}
		totalTime += delay
	}

	duration = float64(totalTime) / 100.0

	if totalTime == 0 || len(lines) == 0 {
		return 1, 0, nil
	}

	framerate = min(defaultFramerate, int(math.Round(float64(len(lines))/duration)))

	return framerate, duration, nil
}
