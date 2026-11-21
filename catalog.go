package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/list"
)

var Verbose bool

func logMsg(level, module, message string) {
	if Verbose {
		fmt.Fprintf(os.Stderr, "[%s] (%s) %s\n", level, module, message)
	}
}

func fetchAnimeList(ctx context.Context) ([]list.Item, error) {
	var catalogue map[string]string
	if err := apiJSON(ctx, "POST", "/v1/rpc/catalog", map[string]string{"language": "sub"}, true, &catalogue); err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(catalogue))
	for title := range catalogue {
		titles = append(titles, title)
	}
	sort.Strings(titles)
	items := make([]list.Item, 0, len(titles))
	for _, title := range titles {
		items = append(items, myListItem{title: title, coverUrl: catalogue[title]})
	}
	return items, nil
}

func showQuickLook(imageURL string) {
	client := http.Client{Timeout: 10 * time.Second}
	response, err := client.Get(imageURL)
	if err != nil {
		logMsg("ERROR", "preview", err.Error())
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return
	}
	file, err := os.CreateTemp("", "anime-preview-*.jpg")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, copyErr := io.Copy(file, io.LimitReader(response.Body, 10*1024*1024))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return
	}
	if err := exec.Command("qlmanage", "-p", file.Name()).Run(); err != nil {
		logMsg("ERROR", "preview", err.Error())
	}
}
