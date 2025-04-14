package main

import (
	"fmt"
	"log"
	"os"
	"seventv2tg/infrastructure/webapi/seventv"
	"seventv2tg/service/media"
)

const (
	inputDirName  = "input"
	jobsDirName   = "jobs"
	resultDirName = "output"
)

func main() {
	var err error

	err = setupDirs()
	if err != nil {
		log.Fatal(err)
	}

	emoteUrl := "https://7tv.app/emotes/01GV82B6VR000CGWVNBSKSXVCB"

	seventvAPI := seventv.NewAPI(inputDirName)
	mediaService := media.NewMediaConverter(jobsDirName, resultDirName)

	inpFilePath, err := seventvAPI.DownloadWebp(emoteUrl)

	resFilePath, err := mediaService.ConvertToTelegramVideo(inpFilePath)
	if err != nil {
		log.Fatal(err)
	}

	_ = resFilePath

	fmt.Println("Success!")
}

func setupDirs() error {
	for _, dir := range []string{inputDirName, jobsDirName, resultDirName} {
		err := os.RemoveAll(dir)
		if err != nil {
			return err
		}

		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	return nil
}
