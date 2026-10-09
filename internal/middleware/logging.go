package middleware

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	httputil "github.com/aspirit/xsocial/pkg/http"
	"github.com/aspirit/xsocial/pkg/logger"
	"github.com/aspirit/xsocial/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Sensitive field constants for masking.
const (
	Email    = "email"
	Phone    = "phone"
	Password = "password"
)

// Log field constants.
const (
	FieldRequestID    = "request_id"
	FieldStatus       = "status"
	FieldCode         = "code"
	FieldMessage      = "message"
	FieldClientIP     = "client_ip"
	FieldHost         = "host"
	FieldMethod       = "method"
	FieldPath         = "path"
	FieldContentType  = "content_type"
	FieldRawQuery     = "raw_query"
	FieldReferer      = "referer"
	FieldResponseTime = "response_time"
	FieldDeviceID     = "device_id"
	FieldTraceID      = "trace_id"
	FieldSpanID       = "span_id"
)

// HTTP method constants.
const (
	MethodPOST  = "POST"
	MethodPUT   = "PUT"
	MethodPATCH = "PATCH"
)

// Content type constants.
const (
	ContentTypeJSON = "application/json"
)

// Request header constants.
const (
	HeaderRequestID   = "X-Request-ID"
	HeaderContentType = "Content-Type"
)

// Log message constants.
const (
	LogMessageHTTPRequest     = "HTTP request"
	LogMessageParseBodyFailed = "Failed to parse request body, continuing without body logging"
)

// LogFields holds data for structured logging.
type LogFields struct {
	RequestID    string
	Status       int
	Code         int
	Message      string
	ClientIP     string
	Host         string
	Method       string
	Path         string
	ContentType  string
	RawQuery     string
	referer      string
	ResponseTime float64
	Body         map[string]any
	DeviceID     string
	TraceID      string
	SpanID       string
}

// Parses JSON request body and masks sensitive fields.
func parseRequestBody(c *gin.Context) (map[string]any, error) {
	body, err := c.GetRawData()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw data: %w", err)
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	const maxBodySize = 1024 * 1024 // 1MB
	if len(body) > maxBodySize {
		return map[string]any{"truncated": true}, nil
	}

	var bd map[string]any
	if err := jsoniter.Unmarshal(body, &bd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request body: %w", err)
	}

	// mask fields: email, phone, password
	if _, ok := bd[Email]; ok {
		if email, ok := bd[Email].(string); ok {
			bd[Email] = utils.StringUtil.MaskEmail(email)
		}
	}

	if _, ok := bd[Phone]; ok {
		if phone, ok := bd[Phone].(string); ok {
			bd[Phone] = utils.StringUtil.MaskPhone(phone)
		}
	}

	if _, ok := bd[Password]; ok {
		bd[Password] = "******"
	}

	return bd, nil
}

// Constructs zap fields for logging.
func buildLogFields(fields LogFields) []zap.Field {
	zapFields := []zap.Field{
		zap.String(FieldRequestID, fields.RequestID),
		zap.Int(FieldStatus, fields.Status),
		zap.Int(FieldCode, fields.Code),
		zap.String(FieldMessage, fields.Message),
		zap.String(FieldClientIP, fields.ClientIP),
		zap.String(FieldHost, fields.Host),
		zap.String(FieldMethod, fields.Method),
		zap.String(FieldPath, fields.Path),
		zap.String(FieldContentType, fields.ContentType),
		zap.String(FieldRawQuery, fields.RawQuery),
		zap.String(FieldReferer, fields.referer),
		zap.Float64(FieldResponseTime, fields.ResponseTime),
		zap.String(FieldDeviceID, fields.DeviceID),
		zap.String(FieldTraceID, fields.TraceID),
		zap.String(FieldSpanID, fields.SpanID),
	}

	return zapFields
}

// Logging creates a logging middleware that logs HTTP requests and responses.
func Logging(baseLogger *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		requestID := c.GetHeader(HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}

		reqLogger := baseLogger.With(zap.String(FieldRequestID, requestID))

		// Inject logger into context
		ctx := logger.WithLogger(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)

		// Only parse request body for POST/PUT/PATCH methods with JSON content
		var bd map[string]any
		method := c.Request.Method
		contentType := c.GetHeader(HeaderContentType)
		if (method == MethodPOST || method == MethodPUT || method == MethodPATCH) &&
			(strings.Contains(contentType, ContentTypeJSON) || contentType == "") {
			var err error
			bd, err = parseRequestBody(c)
			if err != nil {
				reqLogger.Debug(LogMessageParseBodyFailed, zap.Error(err))
			}
		}

		c.Next()

		var traceID, spanID string
		if span := oteltrace.SpanFromContext(c.Request.Context()); span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
			spanID = span.SpanContext().SpanID().String()
		}

		responseTime := float64(time.Since(startTime))

		clientIP := c.ClientIP()
		code := c.Writer.Status()
		message := c.GetString(FieldMessage)
		status := c.GetInt(FieldStatus)
		path := c.Request.URL.Path

		deviceID := c.GetHeader(httputil.HeaderDeviceID)

		logFields := LogFields{
			RequestID:    requestID,
			Status:       status,
			Code:         code,
			Message:      message,
			ClientIP:     clientIP,
			Host:         c.Request.Host,
			Method:       method,
			Path:         path,
			ContentType:  contentType,
			RawQuery:     c.Request.URL.RawQuery,
			referer:      c.Request.Referer(),
			ResponseTime: responseTime,
			Body:         bd,
			DeviceID:     deviceID,
			TraceID:      traceID,
			SpanID:       spanID,
		}
		zapFields := buildLogFields(logFields)
		reqLogger.Info(LogMessageHTTPRequest, zapFields...)
	}
}
