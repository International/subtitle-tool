package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/International/podnapisi-go"
	"github.com/oz/osdb"
)

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "all"
var REQUIRED_INT_NOT_PASSED = "0"
var NO_LIMIT = 0
var CURRENT_FOLDER = "."
var NO_EDITOR = ""

type SubtitleSearcher interface {
	Search(podnapisi.ShowSearchParams) ([]podnapisi.Subtitle, error)
}

type PodnapisiSearch struct {
}

func (p PodnapisiSearch) Search(searchParams podnapisi.ShowSearchParams) ([]podnapisi.Subtitle, error) {
	return podnapisi.Search(searchParams)
}

type OSDBSearch struct {
}

func (p OSDBSearch) Search(searchParams podnapisi.ShowSearchParams) ([]podnapisi.Subtitle, error) {
	if c, err := osdb.NewClient(); err == nil {

		if err = c.LogIn("", "", ""); err != nil {
			return []podnapisi.Subtitle{}, err
		}

		langAdaptation, ok := languageAdaptation[searchParams.Language]
		if !ok {
			return []podnapisi.Subtitle{}, fmt.Errorf("language %s not supported", searchParams.Language)
		}

		params := []interface{}{
			c.Token,
			[]map[string]string{
				{
					"query":         searchParams.Name,
					"season":        searchParams.Season,
					"episode":       searchParams.Episode,
					"sublanguageid": langAdaptation,
				},
			},
		}

		log.Println("OSDB query: %v", params)

		subz, err := c.SearchSubtitles(&params)
		if err != nil {
			return []podnapisi.Subtitle{}, err
		}
		to_podnapisi_sub := make([]podnapisi.Subtitle, 0)

		for _, sub := range subz {
			// log.Println("sub iz %v", sub)
			podsub := podnapisi.Subtitle{
				Title: sub.MovieReleaseName, Releases: []string{}, Season: sub.SeriesSeason,
				Episode: sub.SeriesEpisode, Language: sub.LanguageName, URL: sub.ZipDownloadLink,
			}
			to_podnapisi_sub = append(to_podnapisi_sub, podsub)
		}
		return to_podnapisi_sub, nil

	} else {
		return []podnapisi.Subtitle{}, err
	}
}

var subtitleSearchEngines = []SubtitleSearcher{OSDBSearch{}, PodnapisiSearch{}}

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

func isRelevantSubtitle(fileName string) bool {
	irrelevantExtensions := []string{".nfo"}
	isRelevant := true
	lowerCasedName := strings.ToLower(fileName)

	for _, sub := range irrelevantExtensions {
		if strings.HasSuffix(lowerCasedName, sub) {
			isRelevant = false
			break
		}
	}

	return isRelevant
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
		if !isRelevantSubtitle(file.Name) {
			continue
		}

		log.Println("preparing to download file", file.Name)
		if fileHandle, err := file.Open(); err == nil {
			archivedFile := path.Join(cmdLineOpts.OutputFolder, file.Name)
			outputDest = archivedFile

			if diskSub, createErr := os.Create(archivedFile); createErr == nil {
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

func SearchSubs(searchParams podnapisi.ShowSearchParams) ([]podnapisi.Subtitle, error) {
	subz := make([]podnapisi.Subtitle, 0)
	subSearchErrors := make([]error, 0)

	for _, subSearchEngine := range subtitleSearchEngines {
		subs, subError := subSearchEngine.Search(searchParams)
		if subError == nil {
			subz = append(subz, subs...)
		} else {
			subSearchErrors = append(subSearchErrors, subError)
		}
	}

	errorString := ""

	for _, subSearchError := range subSearchErrors {
		errorString += fmt.Sprintf("error:%s ", subSearchError.Error())
	}

	if errorString != "" {
		return subz, errors.New(errorString)
	} else {
		return subz, nil
	}

}

var languageAdaptation = map[string]string{
	"en":  "eng",
	"pl":  "pol",
	"pol": "pol",
	"all": "all",
}

func main() {
	params, err := parseParams()
	if err != nil {
		log.Fatalf(err.Error())
		log.Fatalf("usage: subtitle_tool -name name -season season_number -episode episode_number -download")
	}

	subtitles, err := SearchSubs(params.ShowSearchParams)
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
