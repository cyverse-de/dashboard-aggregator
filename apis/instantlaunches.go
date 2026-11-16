package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/cyverse-de/dashboard-aggregator/config"
	"go.opentelemetry.io/otel"
)

type InstantLaunchesAPI struct {
	appExposerURL  *url.URL
	appExposerUser string
	attribute      string
	value          string
}

func NewInstantLaunchesAPI(config *config.ServiceConfiguration) (*InstantLaunchesAPI, error) {
	u, err := url.Parse(config.AppExposer.URL)
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("instantlaunches", "metadata", "full")
	return &InstantLaunchesAPI{
		appExposerURL:  u,
		appExposerUser: config.AppExposer.User,
		attribute:      "ui_location",
		value:          "dashboard",
	}, nil
}

func (i *InstantLaunchesAPI) PullItems(ctx context.Context) ([]map[string]interface{}, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "PullItems")
	defer span.End()

	u := i.appExposerURL

	q := u.Query()
	q.Set("user", i.appExposerUser)
	q.Set("attribute", i.attribute)
	q.Set("value", i.value)
	u.RawQuery = q.Encode()

	log.Infof("pulling items from %s", u.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	msg, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("url %s; status code %d; msg %s", u.String(), resp.StatusCode, string(msg))
	}

	items := make([]map[string]interface{}, 0)
	if err = json.Unmarshal(msg, &items); err != nil {
		return nil, err
	}

	return items, nil
}

func (i *InstantLaunchesAPI) PullItemsAsync(ctx context.Context, itemsChan chan []map[string]interface{}, errChan chan error) {
	items, err := i.PullItems(ctx)
	if err != nil {
		errChan <- err
		return
	}
	errChan <- nil
	itemsChan <- items
}
