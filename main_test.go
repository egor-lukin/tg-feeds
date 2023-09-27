package main

import (
	"github.com/jarcoal/httpmock"
	"io/ioutil"
	"testing"
)

func readFixture(path string) (string, error) {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func TestParseChannel(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	fixture, err := readFixture("fixtures/feed.html")
	if err != nil {
		t.Errorf("Invalid fixture")
	}

	httpmock.RegisterResponder("GET", "https://t.me/s/lexfridman",
		httpmock.NewStringResponder(200, fixture))

	channelName := "lexfridman"
	fetcher := &TelegramWebFetcher{}
	channel, _ := fetcher.FetchChannel(channelName)

	channelLastId := 293
	if channel.LastId != channelLastId {
		t.Errorf("Invalid channel last id, expected - %d, actual - %d", channel.LastId, channelLastId)
	}

	title := "Lex Fridman"
	if channel.Title != title {
		t.Errorf("Invalid channel title, expected - %s, actual - %s", channel.Title, title)
	}

	description := "Host of Lex Fridman Podcast.Research Scientist at MIT.Interested in robots and humans."
	if channel.Description != description {
		t.Errorf("Invalid description, expected - %s, actual - %s", channel.Description, description)
	}
}

func TestFetchPost(t *testing.T) {
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	fixture, err := readFixture("fixtures/post.html")
	if err != nil {
		t.Errorf("Invalid fixture")
	}

	expectedLink := "https://t.me/lexfridman/272?embed=1&mode=tme"
	httpmock.RegisterResponder("GET", expectedLink,
		httpmock.NewStringResponder(200, fixture))

	channelName := "lexfridman"
	fetcher := &TelegramWebFetcher{}
	post, err := fetcher.FetchPost(channelName, 272)

	if err != nil {
		t.Errorf("Invalid header, actual - %s", err.Error())
	}

	expectedHeader := "All humans are capable of both good and evil. And most who do evil believe they are doing good. Hist..."
	if post.Header != expectedHeader {
		t.Errorf("Invalid post header, expected - %s, actual - %s", expectedHeader, post.Header)
	}

	if post.Link != expectedLink {
		t.Errorf("Invalid post link, expected - %s, actual - %s", expectedLink, post.Link)
	}

	createdAt := "2023-06-16 17:37:03 +0000 +0000"
	if post.CreatedAt.String() != createdAt {
		t.Errorf("Invalid time, expected - %s, actual - %s", post.CreatedAt.String(), createdAt)
	}
}
