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
	"strings"

	"github.com/lestrrat/go-libxml2"
	"github.com/lestrrat/go-libxml2/xpath"
)

type Subtitle struct {
	Releases []string
	Season   string
	Episode  string
	Language string
	URL      string
}

type ShowSearchParams struct {
	Name     string
	Season   string
	Episode  string
	Download bool
	Language string
	Limit    int
}

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "ALL"
var REQUIRED_INT_NOT_PASSED = "0"
var NO_LIMIT = 0

func parseParams() (*ShowSearchParams, error) {
	showName := flag.String("name", SHOW_NOT_PASSED, "name of show")
	season := flag.String("season", REQUIRED_INT_NOT_PASSED, "season number")
	episode := flag.String("episode", REQUIRED_INT_NOT_PASSED, "episode number")
	language := flag.String("language", ALL_LANGUAGES, "language name")
	download := flag.Bool("download", false, "download subtitles")
	limit := flag.Int("limit", NO_LIMIT, "download subtitles")

	flag.Parse()

	if *showName == SHOW_NOT_PASSED {
		return nil, errors.New("name of show is required")
	}
	if *season == REQUIRED_INT_NOT_PASSED || *episode == REQUIRED_INT_NOT_PASSED {
		return nil, errors.New("make sure to send a parameter for season and episode")
	}

	return &ShowSearchParams{*showName, *season, *episode, *download, *language, *limit}, nil
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
	log.Println("number of subtitle nodes", len(subtitleNodes))

	for _, subtitle := range subtitleNodes {
		subCtx, err := xpath.NewContext(subtitle)

		if err != nil {
			return nil, err
		}

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
			Subtitle{Releases: releaseCollection, Season: season,
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

	log.Println(requestParams, "len:", len(requestParams))
	queryString = strings.Join(requestParams, "&")
	log.Println("queryString", queryString)
	fullUrl := requestUrl + queryString

	log.Println("requesting url", fullUrl)
	response, err := http.Get(fullUrl)

	if err != nil {
		return nil, err
	}

	log.Println("status code:", response.StatusCode)
	data, err := ioutil.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	return data, nil
}

func downloadSubtitle(sub Subtitle) error {
	response, err := http.Get(sub.URL)
	if err != nil {
		return err
	}
	bodyContents, err := ioutil.ReadAll(response.Body)

	defer response.Body.Close()

	if err != nil {
		return err
	}

	body := bytes.NewReader(bodyContents)
	archive, err := zip.NewReader(body, int64(len(bodyContents)))

	if err != nil {
		return err
	}

	for _, file := range archive.File {
		log.Println("preparing to download file", file.Name)
		if fileHandle, err := file.Open(); err == nil {
			if diskSub, createErr := os.Create(file.Name); createErr == nil {
				_, copyErr := io.Copy(diskSub, fileHandle)
				if copyErr != nil {
					return copyErr
				} else {
					log.Println("wrote", file.Name)
				}
			} else {
				return createErr
			}
		} else {
			return err
		}
	}

	return nil
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

	log.Println(subtitles)

	if params.Download {
		log.Println("downloading subtitles:", len(subtitles))

		for _, subtitle := range subtitles {
			err := downloadSubtitle(subtitle)
			if err != nil {
				log.Fatalf(err.Error())
			} else {
				log.Println("succesfully downloaded", subtitle.URL)
			}
		}
	} else {
		log.Println("not downloading subtitles")
	}

	// args := os.Args[1:]
	// file, err := os.Open(args[0])
	// if err != nil {
	// 	log.Fatalf(err.Error())
	// }
	// data, err := ioutil.ReadAll(file)
	// if err != nil {
	// 	log.Fatalf(err.Error())
	// }
	// subtitles, err := parseSubtitles(data)
	// log.Println(subtitles)
}
