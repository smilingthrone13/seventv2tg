package media

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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

func NewMediaConverter(jobsDir, resDir string) *Converter {
	return &Converter{
		jobsDir: jobsDir,
		resDir:  resDir,
	}
}

type Converter struct {
	jobsDir string
	resDir  string
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

	framerate, err := c.getVideoFramerate(inpFilePath)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	err = c.createSequence(inpFilePath, filepath.Join(framesDirPath, frameMask))
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

	var layers []string

	height, width := autoHeight, autoWidth

	for i := range inpFilePaths {
		framesDirPath := filepath.Join(c.jobsDir, jobID, fmt.Sprintf("frames-%d", i))

		err = os.MkdirAll(framesDirPath, os.ModePerm)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		seqPath := filepath.Join(framesDirPath, frameMask)

		framerate, err := c.getVideoFramerate(inpFilePaths[i])
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		err = c.createSequence(inpFilePaths[i], seqPath)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

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

		layers = append(layers, webmPath)
	}

	err = c.createOverlayedVideo(layers, resPath)
	if err != nil {
		_ = os.Remove(resPath)
		return "", errors.Wrap(err, errMsg)
	}

	return resPath, nil
}

func (c *Converter) createSequence(inpPath, outPath string) error {
	cmd := exec.Command(
		"magick",
		inpPath,
		"-coalesce",
		"+repage",
		outPath,
	)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), "createSequence")
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

func (c *Converter) createOverlayedVideo(inpPaths []string, outPath string) error {
	const errMessage = "createOverlayedVideo"
	var err error

	bitrate := overlayedBitRate

	for {
		err = c.assembleLayers(inpPaths, outPath, bitrate)
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
		//"-vf", "scale="+scaleStr+",format=yuva420p,loop=0",
		"-vf", "scale="+scaleStr+",format=yuva420p",
		"-c:v", "libvpx-vp9",
		"-b:v", fmt.Sprintf("%dK", bitrate),
		"-auto-alt-ref", "0",
		"-an",
		"-threads", "3", // TODO: в конфиг
		"-t", "3",
		outPath,
	)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), errMessage)
}

func (c *Converter) assembleLayers(inpPaths []string, outPath string, bitrate int) error {
	const errMessage = "assembleLayers"

	if len(inpPaths) < 2 {
		return errors.Wrap(errors.New("not enough videos to merge"), errMessage)
	}

	args := []string{
		"-y",
		"-loglevel", "error",
		"-c:v", "libvpx-vp9",
		"-i", inpPaths[0],
	}

	overlayString := strings.Builder{}
	prevLayer := "0"

	for i := 1; i < len(inpPaths); i++ {
		inpArgs := []string{
			"-stream_loop", "-1",
			"-c:v", "libvpx-vp9",
			"-i", inpPaths[i],
		}

		args = append(args, inpArgs...)

		currLayer := fmt.Sprintf("tmp%d", i)

		overlayString.WriteString(fmt.Sprintf("[%s][%d] overlay=shortest=1", prevLayer, i))
		if i != len(inpPaths)-1 {
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
		"-threads", "3", // TODO: в конфиг
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

func (m *Converter) getVideoFramerate(inpPath string) (int, error) {
	const errMessage = "getVideoFramerate"

	cmd := exec.Command("magick", "identify", "-format", "%T\n", inpPath)
	output, err := cmd.Output()
	if err != nil {
		return 0, errors.Wrap(err, errMessage)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	totalTime := 0
	for _, line := range lines {
		delay, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil {
			return 0, errors.Wrap(err, errMessage)
		}
		totalTime += delay
	}

	if totalTime == 0 || len(lines) == 0 {
		return 1, nil
	}

	duration := float64(totalTime) / 100.0
	fps := float64(len(lines)) / duration

	return min(defaultFramerate, max(int(math.Round(fps)), 10)), nil
}
