package media

import (
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
	defaultBitRate   = 250
	overlayedBitRate = defaultBitRate + 150 // since we'll have more details we might also increase bitrate
	maxFrameRate     = 30
	maxResultSize    = 256 << 10
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

func (m *Converter) ConvertToVideo(inpFilePath string) (resPath string, err error) {
	const errMsg = "Converter.ConvertToVideo"

	const frameMask = "frame_%03d.png"

	jobID := uuid.NewString()

	defer func() {
		errFs := os.RemoveAll(filepath.Join(m.jobsDir, jobID))
		if errFs != nil {
			slog.Error("Failed to remove job folder", slog.String("jobID", jobID), slog.Any("err", err))
		}
	}()

	framesDirPath := filepath.Join(m.jobsDir, jobID, "frames")

	err = os.RemoveAll(framesDirPath)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	err = os.MkdirAll(framesDirPath, os.ModePerm)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	framerate, err := m.getInputFramerate(inpFilePath)
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	err = m.createSequence(inpFilePath, filepath.Join(framesDirPath, frameMask))
	if err != nil {
		return "", errors.Wrap(err, errMsg)
	}

	resPath = filepath.Join(m.resDir, jobID+".webm")

	err = m.createVideoFromSequence(filepath.Join(framesDirPath, frameMask), resPath, framerate)
	if err != nil {
		_ = os.Remove(resPath)
		return "", errors.Wrap(err, errMsg)
	}

	return resPath, nil
}

func (m *Converter) OverlayVideos(inpVideosPaths []string) (resPath string, err error) {
	const errMsg = "Converter.OverlayVideos"

	jobID := uuid.NewString()
	resPath = filepath.Join(m.resDir, jobID+".webm")

	err = m.overlayVideos(inpVideosPaths, resPath)
	if err != nil {
		_ = os.Remove(resPath)
		return "", errors.Wrap(err, errMsg)
	}

	return resPath, nil
}

func (m *Converter) createSequence(inpPath, outPath string) error {
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

func (m *Converter) createVideoFromSequence(inpPath, outPath string, framerate int) error {
	const errMessage = "createVideoFromSequence"
	var err error

	bitrate := defaultBitRate

	for {
		err = m.assembleSequence(inpPath, outPath, framerate, bitrate)
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

		bitrate, framerate, err = m.downscaleVideoParameters(bitrate, framerate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}
	}
}

func (m *Converter) downscaleVideoParameters(bitrate, framerate int) (newBitrate, newFramerate int, err error) {
	const errMessage = "downscaleVideoParameters"
	const (
		bitrateFirstThreshold    = 150
		bitrateSecondThreshold   = 100
		framerateFirstThreshold  = 25
		framerateSecondThreshold = 20

		bitrateDropRate   = 20
		framerateDropRate = 2
	)

	if bitrate >= bitrateFirstThreshold {
		return bitrate - bitrateDropRate, framerate, nil
	}

	if framerate >= framerateFirstThreshold {
		return framerate, framerate - framerateDropRate, nil
	}

	if bitrate >= bitrateSecondThreshold {
		return bitrate - bitrateDropRate, framerate, nil
	}

	if framerate >= framerateSecondThreshold {
		return framerate, framerate - framerateDropRate, nil
	}

	err = errors.New("lower quality limit exceeded")

	return 0, 0, errors.Wrap(err, errMessage)
}

func (m *Converter) assembleSequence(inpPath, outPath string, framerate, bitrate int) error {
	const errMessage = "assembleSequence"

	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-loglevel", "error",
		"-framerate", fmt.Sprintf("%d", framerate),
		"-i", inpPath,
		"-vf", "scale='if(gte(iw,ih),512,-1)':'if(gte(ih,iw),512,-1)',format=yuva420p,loop=0",
		"-c:v", "libvpx-vp9",
		"-b:v", fmt.Sprintf("%dK", bitrate),
		"-an",
		"-threads", "3", // TODO: в конфиг
		"-t", "3",
		outPath,
	)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), errMessage)
}

func (m *Converter) overlayVideos(inpPaths []string, outPath string) error {
	const errMessage = "overlayVideos"

	if len(inpPaths) < 2 {
		return errors.Wrap(errors.New("not enough videos to merge"), errMessage)
	}

	args := []string{
		"-y",
		"-loglevel", "error",
		"-i", inpPaths[0],
		"-c:v", "libvpx-vp9",
	}

	mergeString := strings.Builder{}
	prevLayer := "0"

	for i := 1; i < len(inpPaths); i++ {
		inpArgs := []string{
			"-stream_loop", "-1",
			"-i", inpPaths[i],
			"-c:v", "libvpx-vp9",
		}

		args = append(args, inpArgs...)

		currLayer := fmt.Sprintf("tmp%d", i)

		mergeString.WriteString(fmt.Sprintf("[%s][%d] overlay=shortest=1", prevLayer, i))
		if i != len(inpPaths)-1 {
			mergeString.WriteString(" [" + currLayer + "]; ")
		}

		prevLayer = currLayer
	}

	args = append(
		args,
		"-lavfi", mergeString.String(),
		"-c:v", "libvpx-vp9",
		"-an",
		"-threads", "3",
		"-b:v", fmt.Sprintf("%dK", overlayedBitRate),
		outPath,
	)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stderr = os.Stderr

	return errors.Wrap(cmd.Run(), errMessage)
}

func (m *Converter) getInputFramerate(path string) (int, error) {
	const errMessage = "getInputFramerate"

	cmd := exec.Command("magick", "identify", "-format", "%T\n", path)
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
		err = errors.New("invalid animation timing")

		return 0, errors.Wrap(err, errMessage)
	}

	duration := float64(totalTime) / 100.0
	fps := float64(len(lines)) / duration

	return min(maxFrameRate, int(math.Round(fps))), nil
}
