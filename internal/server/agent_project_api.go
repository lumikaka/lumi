package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"

	"lumi/internal/agent"

	"github.com/labstack/echo/v4"
)

const maxAgentProjectAPIResponseBytes = 16 << 20

func configureAgentProjectAPIGateway(e *echo.Echo, service *agent.Service) {
	if e == nil || service == nil {
		return
	}
	service.WithProjectAPIGateway(projectAPIRouteSpecs(e), echoProjectAPIDispatcher(e))
}

func projectAPIRouteSpecs(e *echo.Echo) []agent.ProjectAPIRouteSpec {
	if e == nil {
		return nil
	}
	routes := e.Routes()
	result := make([]agent.ProjectAPIRouteSpec, 0, len(routes))
	for _, route := range routes {
		if route == nil || !strings.HasPrefix(route.Path, "/api/v1/projects/:project_uuid") {
			continue
		}
		switch route.Method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			result = append(result, agent.ProjectAPIRouteSpec{Method: route.Method, Path: route.Path})
		}
	}
	return result
}

func echoProjectAPIDispatcher(e *echo.Echo) agent.ProjectAPIDispatcher {
	return func(ctx context.Context, input agent.ProjectAPIDispatchRequest) (response agent.ProjectAPIDispatchResponse, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				response = agent.ProjectAPIDispatchResponse{}
				err = fmt.Errorf("dispatch project API: %v", recovered)
			}
		}()

		target, err := projectAPIRequestTarget(input.Path, input.Query)
		if err != nil {
			return response, err
		}
		var body *bytes.Reader
		if input.HasBody {
			encoded, err := json.Marshal(input.Body)
			if err != nil {
				return response, fmt.Errorf("encode project API request body: %w", err)
			}
			body = bytes.NewReader(encoded)
		} else {
			body = bytes.NewReader(nil)
		}
		request, err := http.NewRequestWithContext(ctx, input.Method, target, body)
		if err != nil {
			return response, fmt.Errorf("create project API request: %w", err)
		}
		if input.HasBody {
			request.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		}

		recorder := httptest.NewRecorder()
		echoContext := e.NewContext(request, recorder)
		e.Router().Find(input.Method, request.URL.Path, echoContext)
		if handlerErr := echoContext.Handler()(echoContext); handlerErr != nil {
			e.HTTPErrorHandler(handlerErr, echoContext)
		}
		if recorder.Body.Len() > maxAgentProjectAPIResponseBytes {
			return response, fmt.Errorf("project API response exceeds %d bytes", maxAgentProjectAPIResponseBytes)
		}
		return agent.ProjectAPIDispatchResponse{Status: recorder.Code, Body: append([]byte(nil), recorder.Body.Bytes()...)}, nil
	}
}

func projectAPIRequestTarget(path string, query map[string]any) (string, error) {
	values := url.Values{}
	for key, value := range query {
		if strings.TrimSpace(key) == "" {
			return "", fmt.Errorf("project API query key is empty")
		}
		if err := appendProjectAPIQueryValue(values, key, value); err != nil {
			return "", err
		}
	}
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded, nil
	}
	return path, nil
}

func appendProjectAPIQueryValue(values url.Values, key string, value any) error {
	switch typed := value.(type) {
	case string:
		values.Add(key, typed)
	case bool:
		values.Add(key, strconv.FormatBool(typed))
	case float64:
		values.Add(key, strconv.FormatFloat(typed, 'f', -1, 64))
	case json.Number:
		values.Add(key, typed.String())
	case []string:
		for _, item := range typed {
			values.Add(key, item)
		}
	case []any:
		for _, item := range typed {
			if err := appendProjectAPIQueryValue(values, key, item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("project API query %q has unsupported value type %T", key, value)
	}
	return nil
}
