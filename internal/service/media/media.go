package media

import (
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
	defaultBitRate   = 250
	overlayedBitRate = defaultBitRate + 150 // since we'll have more details we might also increase bitrate
	maxFrameRate     = 30
	maxResultSize    = 256 << 10
	frameMask        = "frame_%03d.png"
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

	jobID := uuid.NewString()

	defer func() {
		errFs := os.RemoveAll(filepath.Join(m.jobsDir, jobID))
		if errFs != nil {
			slog.Error("Failed to remove job folder", slog.String("jobID", jobID), slog.Any("err", err))
		}
	}()

	framesDirPath := filepath.Join(m.jobsDir, jobID, "frames")

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

func (m *Converter) OverlayVideos(inpFilePaths []string) (resPath string, err error) {
	const errMsg = "Converter.OverlayVideos"

	jobID := uuid.NewString()
	resPath = filepath.Join(m.resDir, jobID+".webm")

	defer func() {
		errFs := os.RemoveAll(filepath.Join(m.jobsDir, jobID))
		if errFs != nil {
			slog.Error("Failed to remove job folder", slog.String("jobID", jobID), slog.Any("err", err))
		}
	}()

	var sequences []domain.SequenceInput

	for i := range inpFilePaths {
		framesDirPath := filepath.Join(m.jobsDir, jobID, "frames")

		err = os.MkdirAll(framesDirPath, os.ModePerm)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		framerate, err := m.getInputFramerate(inpFilePaths[i])
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		seqPath := filepath.Join(framesDirPath, frameMask)

		err = m.createSequence(inpFilePaths[i], seqPath)
		if err != nil {
			return "", errors.Wrap(err, errMsg)
		}

		sequences = append(sequences, domain.SequenceInput{
			Path:      seqPath,
			Framerate: framerate,
		})
	}

	err = m.createOverlayedVideoFromSequences(sequences, resPath)
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

	inp := domain.SequenceInput{
		Path:      inpPath,
		Framerate: framerate,
	}

	for {
		err = m.assembleSequence(inp, outPath, bitrate)
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

		bitrate, err = m.downscaleVideoParameters(bitrate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}
	}
}

func (m *Converter) createOverlayedVideoFromSequences(inputs []domain.SequenceInput, outPath string) error {
	const errMessage = "createOverlayedVideoFromSequences"
	var err error

	bitrate := defaultBitRate

	for {
		err = m.assembleSequences(inputs, outPath, bitrate)
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

		bitrate, err = m.downscaleVideoParameters(bitrate)
		if err != nil {
			return errors.Wrap(err, errMessage)
		}
	}
}

func (m *Converter) downscaleVideoParameters(bitrate int) (newBitrate int, err error) {
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

func (m *Converter) assembleSequence(inp domain.SequenceInput, outPath string, bitrate int) error {
	const errMessage = "assembleSequence"

	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-loglevel", "error",
		"-framerate", fmt.Sprintf("%d", inp.Framerate),
		"-i", inp.Path,
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

func (m *Converter) assembleSequences(inputs []domain.SequenceInput, outPath string, bitrate int) error {
	const errMessage = "assembleSequences"

	if len(inputs) < 2 {
		return errors.Wrap(errors.New("not enough videos to merge"), errMessage)
	}

	args := []string{
		"-y",
		"-loglevel", "error",
		"-framerate", fmt.Sprintf("%d", inputs[0].Framerate),
		"-i", inputs[0].Path,
	}

	scaleString := strings.Builder{}
	overlayString := strings.Builder{}

	for i := 1; i < len(inputs); i++ {
		inpArgs := []string{
			"-stream_loop", "-1",
			"-framerate", fmt.Sprintf("%d", inputs[i].Framerate),
			"-i", inputs[i].Path,
		}

		args = append(args, inpArgs...)
	}

	for i := 0; i < len(inputs); i++ {
		currScaleLayer := fmt.Sprintf("scale%d", i)
		nextScaleLayer := fmt.Sprintf("scale%d", i+1)

		scaleStr := fmt.Sprintf(
			"[%d:v] scale='if(gte(iw,ih),512,-1)':'if(gte(ih,iw),512,-1)',format=yuva420p,loop=0 [%s]; ",
			i, currScaleLayer,
		)

		scaleString.WriteString(scaleStr)

		currOverlayLayer := fmt.Sprintf("overlay%d", i)
		prevOverlayLayer := fmt.Sprintf("overlay%d", i-1)
		var overlayStr string

		switch i {
		case 0:
			overlayStr = fmt.Sprintf(
				"[%s][%s] overlay=shortest=1 [%s]; ",
				currScaleLayer, nextScaleLayer, currOverlayLayer,
			)
		case len(inputs) - 1:
			overlayStr = fmt.Sprintf(
				"[%s][%s] overlay=shortest=1",
				prevOverlayLayer, currScaleLayer,
			)
		default:
			overlayStr = fmt.Sprintf(
				"[%s][%s] overlay=shortest=1 [%s]; ",
				prevOverlayLayer, currScaleLayer, currOverlayLayer,
			)
		}

		overlayString.WriteString(overlayStr)
	}

	args = append(
		args,
		"-filter_complex", scaleString.String()+overlayString.String(),
		"-c:v", "libvpx-vp9",
		"-an",
		"-b:v", fmt.Sprintf("%dK", overlayedBitRate),
		"-t", "3",
		"-threads", "3",
		outPath,
	)

	//for i := 1; i < len(inpPaths); i++ {
	//	inpArgs := []string{
	//		"-stream_loop", "-1",
	//		"-i", inpPaths[i],
	//		"-c:v", "libvpx-vp9",
	//	}
	//
	//	args = append(args, inpArgs...)
	//
	//	currLayer := fmt.Sprintf("tmp%d", i)
	//
	//	mergeString.WriteString(fmt.Sprintf("[%s][%d] overlay=shortest=1", prevLayer, i))
	//	if i != len(inpPaths)-1 {
	//		mergeString.WriteString(" [" + currLayer + "]; ")
	//	}
	//
	//	prevLayer = currLayer
	//}
	//
	//args = append(
	//	args,
	//	"-lavfi", mergeString.String(),
	//	"-c:v", "libvpx-vp9",
	//	"-an",
	//	"-threads", "3",
	//	"-b:v", fmt.Sprintf("%dK", overlayedBitRate),
	//	outPath,
	//)

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
