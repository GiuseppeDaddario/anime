package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Set at release build time. No credentials or private keys are embedded.
var defaultAPIURL string
var version = "dev"

func apiBaseURL() (string, error) {
	base := strings.TrimRight(os.Getenv("ANIMEWATCHER_API_URL"), "/")
	if base == "" {
		base = strings.TrimRight(defaultAPIURL, "/")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("The service address is not configured in this build. Ask the app maintainer for a released build")
	}
	loopback := parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback) {
		return "", fmt.Errorf("The service requires HTTPS")
	}
	return base, nil
}

var apiHTTP = &http.Client{
	Timeout: 6 * time.Minute,
	// Never forward a saved session to a redirected host.
	CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
}

type apiStatusError struct {
	status  int
	message string
}

func (e *apiStatusError) Error() string { return e.message }

func apiRequest(ctx context.Context, method, path string, payload any, authenticated bool) (*http.Response, error) {
	base, err := apiBaseURL()
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "anime/"+version)
	if authenticated {
		config, err := readUserConfig()
		if err != nil {
			return nil, err
		}
		if config.Token == "" || config.APIURL != base {
			return nil, fmt.Errorf("Open anime and choose Link Telegram account before continuing")
		}
		req.Header.Set("Authorization", "Bearer "+config.Token)
	}
	response, err := apiHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach the anime service: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		var failure struct {
			Error string `json:"error"`
		}
		if response.StatusCode == http.StatusUnauthorized && authenticated {
			return nil, &apiStatusError{http.StatusUnauthorized, "Your session is no longer valid. Open anime and link your Telegram account again"}
		}
		if json.Unmarshal(data, &failure) == nil && failure.Error != "" {
			return nil, fmt.Errorf("%s", failure.Error)
		}
		return nil, fmt.Errorf("The anime service returned HTTP %d", response.StatusCode)
	}
	return response, nil
}

func apiJSON(ctx context.Context, method, path string, payload any, authenticated bool, target any) error {
	response, err := apiRequest(ctx, method, path, payload, authenticated)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return json.NewDecoder(io.LimitReader(response.Body, 16*1024*1024)).Decode(target)
}

type playResult struct {
	URL          string `json:"url"`
	Title        string `json:"title"`
	Episode      int    `json:"episode"`
	Rewound      bool   `json:"rewound"`
	MalID        int    `json:"mal_id"`
	IntroEpisode int    `json:"intro_episode"`
}

// requestPlay asks the service which episode to play (or rewinds progress).
func requestPlay(ctx context.Context, title, language, episode string) (playResult, error) {
	payload := map[string]any{}
	if title != "" {
		payload["title"] = title
	}
	if language != "" {
		payload["language"] = language
	}
	var result playResult
	if episode != "" {
		n, err := strconv.Atoi(episode)
		if err != nil {
			return result, err
		}
		payload["episode"] = n
	}
	if err := apiJSON(ctx, http.MethodPost, "/v1/rpc/play", payload, true, &result); err != nil {
		return result, err
	}
	if result.Rewound {
		return result, nil
	}
	parsed, err := url.Parse(result.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return result, fmt.Errorf("The service returned an invalid video URL")
	}
	return result, nil
}

func (r playResult) mpvArgs() []string {
	args := []string{}
	if r.MalID > 0 {
		args = append(args, "--script-opts-append=animewatcher-mal_id="+strconv.Itoa(r.MalID), "--script-opts-append=animewatcher-episode="+strconv.Itoa(r.IntroEpisode))
	}
	return append(args, "--", r.URL)
}

func (r playResult) rewoundText() string {
	return fmt.Sprintf("%s: progress set to episode %d", r.Title, r.Episode)
}

// remotePlay is the plain command-line flow (anime --continue / --ep).
func remotePlay(ctx context.Context, title, language, episode string) error {
	result, err := requestPlay(ctx, title, language, episode)
	if err != nil {
		return err
	}
	if result.Rewound {
		fmt.Println(result.rewoundText())
		return nil
	}
	fmt.Printf("Playing %s, episode %d\n", result.Title, result.Episode)
	return runMPV(result.mpvArgs()...)
}

func unauthorized(err error) bool {
	var status *apiStatusError
	return errors.As(err, &status) && status.status == http.StatusUnauthorized
}
