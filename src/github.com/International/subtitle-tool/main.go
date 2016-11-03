package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
)

type Subtitle struct {
}

type ShowSearchParams struct {
	Name     string
	Season   string
	Episode  string
	Download bool
	Language string
}

var SHOW_NOT_PASSED = "MISSING"
var ALL_LANGUAGES = "ALL"
var REQUIRED_INT_NOT_PASSED = "0"

func parseParams() (*ShowSearchParams, error) {
	showName := flag.String("name", SHOW_NOT_PASSED, "name of show")
	season := flag.String("season", REQUIRED_INT_NOT_PASSED, "season number")
	episode := flag.String("episode", REQUIRED_INT_NOT_PASSED, "episode number")
	language := flag.String("language", ALL_LANGUAGES, "language name")
	download := flag.Bool("download", false, "download subtitles")

	flag.Parse()

	if *showName == SHOW_NOT_PASSED {
		return nil, errors.New("name of show is required")
	}
	if *season == REQUIRED_INT_NOT_PASSED || *episode == REQUIRED_INT_NOT_PASSED {
		return nil, errors.New("make sure to send a parameter for season and episode")
	}

	return &ShowSearchParams{*showName, *season, *episode, *download, *language}, nil
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
	log.Println(string(subtitleData))
}
