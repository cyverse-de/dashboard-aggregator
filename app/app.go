package app

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/cyverse-de/dashboard-aggregator/apis"
	"github.com/cyverse-de/dashboard-aggregator/config"
	"github.com/cyverse-de/dashboard-aggregator/db"
	"github.com/cyverse-de/dashboard-aggregator/feeds"
	"github.com/cyverse-de/go-mod/httperror"
	"github.com/cyverse-de/go-mod/logging"
	"github.com/labstack/echo/v4"
	//nolint:staticcheck // TODO: migrate to github.com/labstack/echo-otel/v4; deferred because span names/attributes differ.
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/otel"
)

var log = logging.Log.WithField("package", "app")

const otelName = "github.com/cyverse-de/dashboard-aggregator/app"

const DefaultStartDateInterval = "1 year"
const DefaultLimit = int64(10)

func normalizeLimit(c echo.Context) (int, error) {
	var (
		limit int64
		err   error
	)
	limit = DefaultLimit
	limitStr := c.QueryParam("limit")
	if limitStr != "" {
		limit, err = strconv.ParseInt(limitStr, 10, 64)
		if err != nil {
			return -1, echo.NewHTTPError(http.StatusBadRequest, "could not parse limit as an integer")
		}
	}
	return int(limit), nil
}

func normalizeStartDateInterval(c echo.Context) string {
	startDateInterval := c.QueryParam("start-date-interval")
	if startDateInterval == "" {
		startDateInterval = DefaultStartDateInterval
	}
	return startDateInterval
}

func normalizeUsername(c echo.Context) (string, error) {
	username := c.Param("username")
	if username == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "username must be in the requested path")
	}
	return username, nil
}

type App struct {
	db             *db.Database
	ec             *echo.Echo
	pf             *feeds.PublicFeeds
	ilFeedURL      *url.URL
	appsURL        *url.URL
	metadataURL    *url.URL
	permissionsURL *url.URL
	config         *config.ServiceConfiguration
	publicGroupID  *string
}

func (a *App) SetPublicID(ctx context.Context) error {
	publicGroupID, err := apis.GetGroupID(ctx, a.config)
	if err != nil {
		return err
	}

	a.publicGroupID = publicGroupID

	return nil
}

func New(db *db.Database, pf *feeds.PublicFeeds, cfg *config.ServiceConfiguration) (*App, error) {
	ilURL, err := url.Parse(cfg.AppExposer.URL)
	if err != nil {
		return nil, err
	}
	appsURL, err := url.Parse(cfg.Apps.URL)
	if err != nil {
		return nil, err
	}
	metadataURL, err := url.Parse(cfg.Metadata.URL)
	if err != nil {
		return nil, err
	}
	permissionsURL, err := url.Parse(cfg.Permissions.URL)
	if err != nil {
		return nil, err
	}
	return &App{
		db:             db,
		ec:             echo.New(),
		pf:             pf,
		ilFeedURL:      ilURL,
		appsURL:        appsURL,
		metadataURL:    metadataURL,
		permissionsURL: permissionsURL,
		config:         cfg,
	}, nil
}

func (a *App) Echo() *echo.Echo {
	a.ec.Use(otelecho.Middleware("dashboard-aggregator"))

	a.ec.HTTPErrorHandler = httperror.HTTPErrorHandler

	a.ec.GET("/", a.LoggedOutHandler)
	a.ec.GET("/healthz", a.HealthzHandler)
	a.ec.GET("/feeds", a.PublicFeedsHandler)

	users := a.ec.Group("/users")
	users.GET("/:username", a.UserDashboardHandler)
	users.GET("/:username/apps/public", a.PublicAppsForUserHandler)
	users.GET("/:username/apps/recently-added", a.RecentAddedAppsForUserHandler)
	users.GET("/:username/apps/popular-featured", a.PopularFeaturedAppsForUserHandler)
	users.GET("/:username/apps/recently-used", a.RecentlyUsedAppsForUser)
	users.GET("/:username/analyses/recent", a.RecentAnalysesForUser)
	users.GET("/:username/analyses/running", a.RunningAnalysesForUser)

	apps := a.ec.Group("/apps")
	apps.GET("/public", a.PublicAppsHandler)
	apps.GET("/recently-ran", a.RecentlyRunAppsHandler)

	return a.ec
}

func (a *App) HealthzHandler(c echo.Context) error {
	ctx := c.Request().Context()
	if err := a.db.Healthz(ctx); err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

func (a *App) PublicFeedsHandler(c echo.Context) error {
	ctx := c.Request().Context()
	result := a.pf.Marshallable(ctx)
	return c.JSON(http.StatusOK, &result)
}

func (a *App) featuredAppIDs(ctx context.Context, username string, publicAppIDs []string) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "featuredAppIDs")
	defer span.End()

	log := log.WithField("context", "featured app ids lookup")

	metadataAPI := apis.NewMetadataAPI(a.metadataURL)

	featuredAppsAVUs := []map[string]string{
		{
			"attr":  a.config.Metadata.FeaturedAppsAttribute,
			"value": a.config.Metadata.FeaturedAppsValue,
		},
	}

	log.Debug("getting featured app ids")
	featuredAppIDs, err := metadataAPI.GetFilteredTargetIDs(ctx, username, []string{"app"}, featuredAppsAVUs, publicAppIDs)
	if err != nil {
		return nil, err
	}
	log.Debug("done getting featured app ids")

	return featuredAppIDs, nil
}

func (a *App) featuredAppIDsAsync(ctx context.Context, idsChan chan []string, errChan chan error, username string, publicAppIDs []string) {
	log.Debug("getting featured app IDs (async)")
	featuredAppIDs, err := a.featuredAppIDs(ctx, username, publicAppIDs)
	if err != nil {
		log.Debug("error getting featured app IDs (async)")
		errChan <- err
		return
	}
	log.Debug("got featured app IDs (async)")
	errChan <- nil
	idsChan <- featuredAppIDs
}

func (a *App) publicAppIDs(ctx context.Context) ([]string, error) {
	ctx, span := otel.Tracer(otelName).Start(ctx, "publicAppIDs")
	defer span.End()

	log := log.WithField("context", "public app ids lookup")

	permissionsAPI := apis.NewPermissionsAPI(a.permissionsURL)

	log.Debug("getting public app ids")
	publicAppIDs, err := permissionsAPI.GetPublicIDS(ctx, a.publicGroupID)
	if err != nil {
		return nil, err
	}
	log.Debug("done getting public app ids")

	return publicAppIDs, nil
}

func (a *App) publicAppIDsAsync(ctx context.Context, idsChan chan []string, errChan chan error) {
	publicAppIDs, err := a.publicAppIDs(ctx)
	if err != nil {
		errChan <- err
		return
	}
	errChan <- nil
	idsChan <- publicAppIDs
}
