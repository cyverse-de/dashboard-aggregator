package apis

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/cyverse-de/dashboard-aggregator/config"
	"github.com/samber/lo"
	"go.opentelemetry.io/otel"
)

type PermissionsAPI struct {
	permissionsURL *url.URL
}

func NewPermissionsAPI(permissionsURL *url.URL) *PermissionsAPI {
	return &PermissionsAPI{
		permissionsURL: permissionsURL,
	}
}

type Permission struct {
	ResourceName string `json:"resource_name"`
}
type PermissionsResponse struct {
	Permissions []Permission `json:"permissions"`
}

type group struct {
	GroupID *string `json:"id"`
}

func GetGroupID(ctx context.Context, config *config.ServiceConfiguration) (*string, error) {
	// Setting up Open Telemetry tracer
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetGroupID")
	defer span.End()

	groupName := config.Permissions.PublicGroup

	fullURL, err := url.Parse(config.Permissions.GroupURL)

	if err != nil {
		return nil, err
	}

	fullURL = fullURL.JoinPath("groups", groupName)

	q := fullURL.Query()
	q.Set("user", "de_grouper")

	fullURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("status code was not 200")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var body group

	if err = json.Unmarshal(b, &body); err != nil {
		return nil, err
	}

	return body.GroupID, nil
}

func (p *PermissionsAPI) GetPublicIDS(ctx context.Context, publicGroupID *string) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "GetPublicIDS")
	defer span.End()

	fullURL := *p.permissionsURL
	fullURL = *fullURL.JoinPath("permissions", "abbreviated", "subjects", "group", *publicGroupID, "app")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("status code was not 200")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var body PermissionsResponse
	if err = json.Unmarshal(b, &body); err != nil {
		return nil, err
	}
	retval := lo.Map(body.Permissions, func(item Permission, index int) string {
		return item.ResourceName
	})
	return retval, nil
}
