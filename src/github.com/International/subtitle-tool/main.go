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
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/lestrrat/go-libxml2"
	"github.com/lestrrat/go-libxml2/xpath"
)

type Subtitle struct {
	Title    string
	Releases []string
	Season   string
	Episode  string
	Language string
	URL      string
}

type ShowSearchParams struct {
	Name         string
	Season       string
	Episode      string
	Download     bool
	Language     string
	Limit        int
	OutputFolder string
	EditorName   string
}

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "ALL"
var REQUIRED_INT_NOT_PASSED = "0"
var NO_LIMIT = 0
var CURRENT_FOLDER = "."
var NO_EDITOR = ""

func parseParams() (*ShowSearchParams, error) {
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

	return &ShowSearchParams{
		*showName, *season, *episode, *download,
		*language, *limit, *writeTo, *editorName}, nil
}

func parseSubtitles(input []byte) ([]Subtitle, error) {
	d, err := libxml2.ParseString(string(input))
	if err != nil {
		return nil, err
	}
	ctx, err := xpath.NewContext(d)
	if err != nil {
		return nil, err
	}

	subtitles := make([]Subtitle, 0)

	subtitleNodes := xpath.NodeList(ctx.Find("//subtitle"))

	for _, subtitle := range subtitleNodes {
		subCtx, err := xpath.NewContext(subtitle)

		if err != nil {
			return nil, err
		}

		titleNode, err := subCtx.Find("./title")
		if err != nil {
			return nil, err
		}

		title := titleNode.String()
		urlNode := xpath.NodeList(subCtx.Find("./url"))
		languageNode, err := subCtx.Find("./language")
		if err != nil {
			return nil, nil
		}

		seasonNode, err := subCtx.Find("./tvSeason")
		if err != nil {
			return nil, nil
		}

		episodeNode, err := subCtx.Find("./tvEpisode")
		if err != nil {
			return nil, nil
		}

		language := languageNode.String()
		url := urlNode.NodeValue() + "/download"
		releases := xpath.NodeList(subCtx.Find(".//releases/release"))
		releaseCollection := make([]string, 0)
		season := seasonNode.String()
		episode := episodeNode.String()

		for _, release := range releases {
			releaseCollection = append(releaseCollection, release.NodeValue())
		}

		subtitles = append(subtitles,
			Subtitle{Title: title, Releases: releaseCollection, Season: season,
				Episode: episode, Language: language, URL: url})
	}

	return subtitles, nil
}

func searchSubtitles(searchParams ShowSearchParams) ([]byte, error) {
	params := make(map[string]string)
	params["sK"] = searchParams.Name
	params["sTS"] = searchParams.Season
	params["sTE"] = searchParams.Episode

	if searchParams.Language != ALL_LANGUAGES {
		params["sL"] = searchParams.Language
	}
	params["sXML"] = "1"
	requestUrl := "https://www.podnapisi.net/subtitles/search/old?"
	queryString := ""

	requestParams := make([]string, 0)

	for key, value := range params {
		requestParams = append(requestParams, key+"="+url.QueryEscape(value))
	}

	queryString = strings.Join(requestParams, "&")
	fullUrl := requestUrl + queryString

	response, err := http.Get(fullUrl)

	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	return data, nil
}

func downloadSubtitle(cmdLineOpts ShowSearchParams, sub Subtitle) (string, error) {
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

func main() {
	params, err := parseParams()
	if err != nil {
		log.Fatalf(err.Error())
		log.Fatalf("usage: subtitle_tool -name name -season season_number -episode episode_number -download")
	}
	subtitleData, err := searchSubtitles(*params)
	if err != nil {
		log.Fatalf(err.Error())
	}
	subtitles, err := parseSubtitles(subtitleData)
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
