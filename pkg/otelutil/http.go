// Package otelutil provides OpenTelemetry utilities for HTTP instrumentation.
package otelutil

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	httputil "github.com/aspirit/xsocial/pkg/http"
)

var (
	wrappedTransport = otelhttp.NewTransport(http.DefaultTransport)
	client           = http.Client{Transport: wrappedTransport}
)

type HTTPRequest struct {
	ctx      context.Context
	header   http.Header
	query    url.Values
	formData url.Values
	body     any
}

func R(ctx context.Context) *HTTPRequest {
	return &HTTPRequest{
		ctx:      ctx,
		header:   http.Header{},
		query:    url.Values{},
		formData: url.Values{},
	}
}

func (request *HTTPRequest) request(method, url string) (*http.Request, error) {
	var body io.Reader
	if request.body != nil {
		s, err := jsoniter.MarshalToString(request.body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = strings.NewReader(s)
	} else if len(request.formData) > 0 {
		body = strings.NewReader(request.formData.Encode())
	}
	if len(request.query) > 0 {
		divder := "?"
		if len(strings.Split(url, "?")) > 1 {
			divder = "&"
		}
		url = fmt.Sprintf("%s%s%s", url, divder, request.query.Encode())
	}
	req, err := http.NewRequestWithContext(
		request.ctx,
		method,
		url,
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header = request.header

	return req, nil
}

func (request *HTTPRequest) SetHeader(key, value string) *HTTPRequest {
	request.header.Set(key, value)

	return request
}

func (request *HTTPRequest) SetQuery(key, value string) *HTTPRequest {
	request.query.Set(key, value)

	return request
}

func (request *HTTPRequest) SetForm(key, value string) *HTTPRequest {
	request.formData.Set(key, value)

	return request
}

func (request *HTTPRequest) SetFormData(data map[string]string) *HTTPRequest {
	for k, v := range data {
		request.formData.Set(k, v)
	}

	return request
}

func (request *HTTPRequest) SetJSON(data any) *HTTPRequest {
	request.SetHeader("Content-Type", "application/json")
	request.body = data

	return request
}

func (request *HTTPRequest) SetAuthToken(token string) *HTTPRequest {
	request.SetHeader(httputil.HeaderAuthorization, fmt.Sprintf("Bearer %s", token))

	return request
}

func (request *HTTPRequest) Execute(method string, url string) (*HTTPResponse, error) {
	req, err := request.request(method, url)
	if err != nil {
		return nil, err
	}

	return HTTPDo(req)
}

func (request *HTTPRequest) Get(url string) (*HTTPResponse, error) {
	return request.Execute(http.MethodGet, url)
}

func (request *HTTPRequest) Post(url string) (*HTTPResponse, error) {
	return request.Execute(http.MethodPost, url)
}

func (request *HTTPRequest) Put(url string) (*HTTPResponse, error) {
	return request.Execute(http.MethodPut, url)
}

type HTTPResponse struct {
	statusCode int
	status     string
	header     http.Header
	body       []byte
}

func (response *HTTPResponse) StatusCode() int {
	return response.statusCode
}

func (response *HTTPResponse) Status() string {
	return response.status
}

func (response *HTTPResponse) Header() http.Header {
	return response.header
}

func (response *HTTPResponse) Body() []byte {
	return response.body
}

func HTTPDo(req *http.Request) (*HTTPResponse, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't fail the request
			// This is a common pattern when deferring Close()
			_ = closeErr // Suppress unused variable warning
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &HTTPResponse{
		statusCode: resp.StatusCode,
		status:     resp.Status,
		header:     resp.Header,
		body:       body,
	}, nil
}
