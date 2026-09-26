package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const srsAPITimeout = 3 * time.Second

// srsStreamsPageSize is how many streams one GET /api/v1/streams asks for;
// srsStreamsMaxPages bounds the listing so a misbehaving server cannot loop.
const srsStreamsPageSize = 500
const srsStreamsMaxPages = 50

// srsAPI calls the SRS HTTP API (http_api in deploy/srs.conf, port 1985).
type srsAPI struct {
	base     string
	client   *http.Client
	pageSize int
}

func newSRSAPI(base string) *srsAPI {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil
	}
	return &srsAPI{base: base, client: &http.Client{Timeout: srsAPITimeout}, pageSize: srsStreamsPageSize}
}

// srsStream is the part of an SRS 5 stream entry the reconciler needs.
type srsStream struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Publish struct {
		Active bool   `json:"active"`
		CID    string `json:"cid"`
	} `json:"publish"`
}

// activePublishers returns the streams SRS currently has a publisher for, as
// stream name -> publisher client id (the client_id its hooks report).
//
// SRS 5 answers GET /api/v1/streams?start=N&count=M with
// {"code":0,"streams":[{"id":..,"name":..,"publish":{"active":true,"cid":..}}]},
// listing at most max(M,10) streams from offset N (count defaults to 10), so
// the list is paged until a short page. Any failure returns an error rather
// than a partial list: callers treat a missing stream as a gone publisher.
func (a *srsAPI) activePublishers(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string)
	seen := make(map[string]struct{})
	start := 0
	for page := 0; page < srsStreamsMaxPages; page++ {
		streams, err := a.listStreams(ctx, start, a.pageSize)
		if err != nil {
			return nil, err
		}
		for _, st := range streams {
			key := st.ID + "/" + st.Name
			if _, dup := seen[key]; dup {
				// Offsets were ignored or the list shifted under us.
				return nil, errors.New("srs list streams: inconsistent pages")
			}
			seen[key] = struct{}{}
			if st.Publish.Active && st.Name != "" {
				out[st.Name] = st.Publish.CID
			}
		}
		if len(streams) < a.pageSize {
			return out, nil
		}
		start += len(streams)
	}
	return nil, fmt.Errorf("srs list streams: more than %d pages", srsStreamsMaxPages)
}

func (a *srsAPI) listStreams(ctx context.Context, start, count int) ([]srsStream, error) {
	q := url.Values{}
	q.Set("start", strconv.Itoa(start))
	q.Set("count", strconv.Itoa(count))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/api/v1/streams/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("srs list streams: http %d", resp.StatusCode)
	}
	var body struct {
		Code    int          `json:"code"`
		Streams *[]srsStream `json:"streams"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("srs list streams: decode response: %w", err)
	}
	if body.Code != 0 {
		return nil, fmt.Errorf("srs list streams: code %d", body.Code)
	}
	if body.Streams == nil {
		return nil, errors.New("srs list streams: response has no streams")
	}
	return *body.Streams, nil
}

// kickClient disconnects the SRS client with the id SRS reported as client_id
// in its hooks. SRS 5 answers DELETE /api/v1/clients/{id} with HTTP 200 and a
// JSON "code"; a non-zero code (e.g. client not found) means nothing was kicked.
func (a *srsAPI) kickClient(ctx context.Context, clientID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, a.base+"/api/v1/clients/"+url.PathEscape(clientID), nil)
	if err != nil {
		return err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("srs kick client: http %d", resp.StatusCode)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil {
		return fmt.Errorf("srs kick client: decode response: %w", err)
	}
	if body.Code != 0 {
		return fmt.Errorf("srs kick client: code %d", body.Code)
	}
	return nil
}
