package qrz

import (
	"context"
	"github.com/Station-Manager/errors"
	"github.com/Station-Manager/utils"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Station-Manager/config"
	"github.com/Station-Manager/database"
	"github.com/Station-Manager/logging"
	"github.com/Station-Manager/types"
	"net/http"
	"sync/atomic"
)

// ServiceName is the name of the service and is used to look up the service in the container.
const ServiceName = types.QrzForwardingServiceName

// Service represents a core FORWARDING structure facilitating interaction between logging, configuration, database,
// and HTTP services. It allows for dependency injection and manages initialization state effectively.
type Service struct {
	LoggerService   *logging.Service  `di.inject:"loggingservice"`
	ConfigService   *config.Service   `di.inject:"configservice"`
	DatabaseService database.Database `di.inject:"databaseservice"`
	Config          *types.ForwarderConfig
	client          *http.Client

	isInitialized atomic.Bool
	initOnce      sync.Once
}

type Response struct {
	Result string
	Reason string
	LogIDS string
	LogID  string
	Count  string
	Data   string
}

func (s *Service) Initialize() error {
	const op errors.Op = "forwarder.qrz.Service.Initialize"
	if s.isInitialized.Load() {
		return nil
	}

	var initErr error
	s.initOnce.Do(func() {
		if s.LoggerService == nil {
			initErr = errors.New(op).Msg("logger service has not been set/injected")
			return
		}

		if s.ConfigService == nil {
			initErr = errors.New(op).Msg("application config has not been set/injected")
			return
		}

		if s.DatabaseService == nil {
			initErr = errors.New(op).Msg("database service has not been set/injected")
			return
		}

		cfg, err := s.ConfigService.ForwarderConfig(ServiceName)
		if err != nil {
			initErr = errors.New(op).Err(err).Msg("getting forwarder config")
			return
		}
		s.Config = &cfg

		if s.Config.Enabled {
			s.client = utils.NewHTTPClient(s.Config.HttpTimeout * time.Second)
		} else {
			s.LoggerService.InfoWith().Msg("QRZ.com callsign lookup is disabled in the config")
		}

		s.isInitialized.Store(true)
	})

	return initErr
}

const (
	ActionInsert  = "INSERT"
	ActionReplace = "REPLACE"
)

func (s *Service) Forward(qso types.Qso, param ...string) error {
	const op errors.Op = "forwarder.qrz.Forward"
	if !s.isInitialized.Load() {
		return errors.New(op).Msg("service not initialized")
	}

	//replace := ""
	//if len(param) > 0 {
	//	if param[0] != ActionReplace {
	//		return errors.New(op).Msgf("unsupported action (%s only): %s", param[0], ActionReplace)
	//	}
	//	replace = ActionReplace
	//}

	u, err := url.Parse(s.Config.URL)
	if err != nil {
		return errors.New(op).Err(err).Msg("invalid QRZ base URL")
	}

	payload := "Test"
	form := url.Values{
		"KEY":    {s.Config.APIKey},
		"ACTION": {ActionInsert},
		"ADIF":   {payload},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, u.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New(op).Err(err).Msg("Failed to create HTTP GET request")
	}
	req.Header.Set("User-Agent", s.Config.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New(op).Err(err).Msg("performing HTTP POST request")
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	var body []byte
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return errors.New(op).Err(err).Msg("reading the body")
	}

	var r *Response
	if r, err = parseInsertResponse(body); err != nil {
		return errors.New(op).Err(err).Msg("parsing response data")
	}

	if r.Result == "FAIL" || r.Result == "AUTH" {
		return errors.New(op).Msgf("insert failed for QRZ.com: %s", r.Reason)
	}

	if r.Result == "OK" {
		s.LoggerService.InfoWith().Str("callsign", qso.Call).Str("action", ActionInsert).Msg("QRZ: successful")
	}
	if r.Result == "REPLACE" {
		s.LoggerService.InfoWith().Str("callsign", qso.Call).Str("action", ActionReplace).Msg("QRZ: successful")
	}

	if err = s.updateDatabase(qso); err != nil {
		return errors.New(op).Err(err).Msg("updating database")
	}

	s.LoggerService.DebugWith().Str("callsign", qso.Call).Msg("Successfully forwarded QSO to QRZ.com")

	return nil
}
