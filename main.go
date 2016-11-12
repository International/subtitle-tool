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
	"encoding/json"
	"sort"
)

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "all"
var REQUIRED_INT_NOT_PASSED = "0"
var NO_LIMIT = 0
var CURRENT_FOLDER = "."
var NO_EDITOR = ""
var NO_SPECIAL_OUTPUT = "normal"
var SUPPORTED_FORMATS = []string{"json"}
var VERBOSE = true

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
			//return []podnapisi.Subtitle{}, fmt.Errorf("language %s not supported", searchParams.Language)
			langAdaptation = searchParams.Language
		}

		osdbSearchParams := map[string]string{
			"query":         searchParams.Name,
			"sublanguageid": langAdaptation,
		}

		if searchParams.Season != REQUIRED_INT_NOT_PASSED {
			osdbSearchParams["season"] = searchParams.Season
		}

		if searchParams.Episode != REQUIRED_INT_NOT_PASSED {
			osdbSearchParams["episode"] = searchParams.Episode
		}

		params := []interface{}{
			c.Token,
			[]map[string]string{
				osdbSearchParams,
			},
		}

		subz, err := c.SearchSubtitles(&params)
		if err != nil {
			return []podnapisi.Subtitle{}, err
		}
		to_podnapisi_sub := make([]podnapisi.Subtitle, 0)

		for _, sub := range subz {
			podsub := podnapisi.Subtitle{
				Title: sub.MovieReleaseName, Releases: []string{sub.MovieReleaseName}, Season: sub.SeriesSeason,
				Episode: sub.SeriesEpisode, Language: sub.LanguageName, URL: sub.ZipDownloadLink,
			}
			to_podnapisi_sub = append(to_podnapisi_sub, podsub)
		}
		return to_podnapisi_sub, nil

	} else {
		return []podnapisi.Subtitle{}, err
	}
}

type SubtitleSorter []podnapisi.Subtitle

func (a SubtitleSorter) Len() int           { return len(a) }
func (a SubtitleSorter) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SubtitleSorter) Less(i, j int) bool { return a[i].Language < a[j].Language }


var subtitleSearchEngines = []SubtitleSearcher{OSDBSearch{}, PodnapisiSearch{}}

type cliParams struct {
	OutputFolder string
	EditorName   string
	Download     bool
	OutputFormat string
	Quiet bool
	podnapisi.ShowSearchParams
}

func (c cliParams) d(v ...interface{}) {
	if !c.Quiet {
		log.Println(v...)
	}
}

func parseParams() (*cliParams, error) {
	showName := flag.String("name", SHOW_NOT_PASSED, "name of show")
	season := flag.String("season", REQUIRED_INT_NOT_PASSED, "season number")
	episode := flag.String("episode", REQUIRED_INT_NOT_PASSED, "episode number")
	language := flag.String("language", ALL_LANGUAGES, "language name")
	download := flag.Bool("download", false, "where to download subtitles")
	quiet := flag.Bool("verbose", false, "output extra info")
	outputFormat := flag.String("format", NO_SPECIAL_OUTPUT, "format to write ( JSON supported )")
	writeTo := flag.String("output", CURRENT_FOLDER, "where to write subtitles")
	editorName := flag.String("editor", NO_EDITOR, "open in editor")
	limit := flag.Int("limit", NO_LIMIT, "download subtitles")

	flag.Parse()

	if *showName == SHOW_NOT_PASSED {
		return nil, errors.New("name of show is required")
	}

	return &cliParams{
		ShowSearchParams: podnapisi.ShowSearchParams{Name: *showName, Season: *season, Episode: *episode,
			Language: *language,
			Limit:    *limit}, OutputFormat: *outputFormat,
		OutputFolder: *writeTo, Quiet: *quiet, Download: *download, EditorName: *editorName}, nil
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

		cmdLineOpts.d("preparing to download file", file.Name)
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
	"ro": "ro",
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

	sort.Sort(SubtitleSorter(subtitles))

	if len(subtitles) == 0 {
		log.Fatalf("no subtitles found")
	} else {
		if params.OutputFormat == NO_SPECIAL_OUTPUT {
			for _, subtitle := range subtitles {
				if params.Download {
					params.d("downloading subtitles:", len(subtitles))

					savedTo, err := downloadSubtitle(*params, subtitle)
					if err != nil {
						log.Fatalf(err.Error())
					} else {
						params.d("succesfully downloaded", subtitle.URL)
						if params.EditorName != NO_EDITOR {
							cmd := exec.Command(params.EditorName, savedTo)
							err = cmd.Run()
							if err != nil {
								log.Fatalf(err.Error())
							}
						}
					}
				} else {
					params.d("Subtitle for:", subtitle.Title, "available in lang:", subtitle.Language)
				}
			}
		} else {
			err := formatSubtitles(subtitles, params.OutputFormat)
			if err != nil {
				log.Fatalf(err.Error())
			}
		}
	}
}

func formatSubtitles(subtitles []podnapisi.Subtitle, format string) error {
	validFormatType := false
	for _, supportedFormat := range SUPPORTED_FORMATS {
		if supportedFormat == format {
			validFormatType = true
			break
		}
	}
	if !validFormatType {
		return fmt.Errorf("format %s not supported", format)
	}

	b, err := json.Marshal(subtitles)
	if err != nil {
		return err
	}

	fmt.Println(string(b))
	return nil

}
