package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/International/podnapisi-go"
	"github.com/oz/osdb"
)

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "ALL"
var REQUIRED_INT_NOT_PASSED = "0"
var NO_LIMIT = 0
var CURRENT_FOLDER = "."
var NO_EDITOR = ""

type cliParams struct {
	OutputFolder string
	EditorName   string
	podnapisi.ShowSearchParams
}

func parseParams() (*cliParams, error) {
	showName := flag.String("name", SHOW_NOT_PASSED, "name of show")
	season := flag.String("season", REQUIRED_INT_NOT_PASSED, "season number")
	episode := flag.String("episode", REQUIRED_INT_NOT_PASSED, "episode number")
	language := flag.String("language", ALL_LANGUAGES, "language name")
	download := flag.Bool("download", false, "download subtitles")
	writeTo := flag.String("output", CURRENT_FOLDER, "where to write subtitles")
	editorName := flag.String("editor", NO_EDITOR, "open in editor")
	limit := flag.Int("limit", NO_LIMIT, "download subtitles")

	flag.Parse()

	if *showName == SHOW_NOT_PASSED {
		return nil, errors.New("name of show is required")
	}
	if *season == REQUIRED_INT_NOT_PASSED || *episode == REQUIRED_INT_NOT_PASSED {
		return nil, errors.New("make sure to send a parameter for season and episode")
	}

	return &cliParams{
		ShowSearchParams: podnapisi.ShowSearchParams{Name: *showName, Season: *season, Episode: *episode,
			Download: *download, Language: *language,
			Limit: *limit}, OutputFolder: *writeTo, EditorName: *editorName}, nil
}

func downloadSubtitle(cmdLineOpts cliParams, sub podnapisi.Subtitle) (string, error) {
	response, err := http.Get(sub.URL)
	if err != nil {
		return "", err
	}
	bodyContents, err := ioutil.ReadAll(response.Body)

	defer response.Body.Close()

	if err != nil {
		return "", err
	}

	body := bytes.NewReader(bodyContents)
	archive, err := zip.NewReader(body, int64(len(bodyContents)))

	if err != nil {
		return "", err
	}

	outputDest := ""

	for _, file := range archive.File {
		log.Println("preparing to download file", file.Name)
		if fileHandle, err := file.Open(); err == nil {
			outputDest = path.Join(cmdLineOpts.OutputFolder, file.Name)
			if diskSub, createErr := os.Create(outputDest); createErr == nil {
				_, copyErr := io.Copy(diskSub, fileHandle)
				if copyErr != nil {
					return "", copyErr
				}
			} else {
				return "", createErr
			}
		} else {
			return "", err
		}
	}

	if outputDest == "" {
		return "", errors.New("no files in the archive")
	} else {
		return outputDest, nil
	}
}

func SearchIMDB(q string) (osdb.Movies, error) {
	if c, err := osdb.NewClient(); err == nil {

		if err = c.LogIn("", "", ""); err != nil {
			return nil, err
		}

		return c.IMDBSearch(q)

	} else {
		return nil, err
	}
}

func main() {
	params, err := parseParams()
	if err != nil {
		log.Fatalf(err.Error())
		log.Fatalf("usage: subtitle_tool -name name -season season_number -episode episode_number -download")
	}

	subtitles, err := podnapisi.Search(params.ShowSearchParams)
	if err != nil {
		log.Fatalf(err.Error())
	}

	if params.Limit != NO_LIMIT {
		subtitles = subtitles[0:params.Limit]
	}

	if len(subtitles) == 0 {
		log.Fatalf("no subtitles found")
	} else {
		for _, subtitle := range subtitles {
			if params.Download {
				log.Println("downloading subtitles:", len(subtitles))

				savedTo, err := downloadSubtitle(*params, subtitle)
				if err != nil {
					log.Fatalf(err.Error())
				} else {
					log.Println("succesfully downloaded", subtitle.URL)
					if params.EditorName != NO_EDITOR {
						cmd := exec.Command(params.EditorName, savedTo)
						err = cmd.Run()
						if err != nil {
							log.Fatalf(err.Error())
						}
					}
				}
			} else {
				log.Println("Subtitle for:", subtitle.Title, "available in lang:", subtitle.Language)
			}
		}
	}
}
