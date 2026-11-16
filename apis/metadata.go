package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel"
)

type MetadataAPI struct {
	metadataURL *url.URL
}

func NewMetadataAPI(metadataURL *url.URL) *MetadataAPI {
	return &MetadataAPI{
		metadataURL: metadataURL,
	}
}

type TargetIDs struct {
	TargetIDs []string `json:"target-ids"`
}

func (m *MetadataAPI) GetFilteredTargetIDs(ctx context.Context, username string, targetTypes []string, avus []map[string]string, targetIDs []string) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetFilteredTargetIDs")
	defer span.End()

	u := fixUsername(username)

	fullURL := *m.metadataURL.JoinPath("avus", "filter-targets")
	q := fullURL.Query()
	q.Set("user", u)
	fullURL.RawQuery = q.Encode()

	body := map[string]interface{}{}
	body["target-types"] = targetTypes
	body["target-ids"] = targetIDs
	body["avus"] = avus

	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		msg := string(rb)
		return nil, fmt.Errorf("url %s; status code %d; msg %s", fullURL.String(), resp.StatusCode, msg)
	}

	var data TargetIDs

	if err = json.Unmarshal(rb, &data); err != nil {
		return nil, err
	}

	retval := data.TargetIDs
	return retval, nil
}
