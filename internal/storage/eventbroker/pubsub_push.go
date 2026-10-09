// Package eventbroker provides event broker publisher factory and Pub/Sub Push endpoint.
package eventbroker

import (
	"encoding/base64"
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	pkgeventbroker "github.com/aspirit/xsocial/pkg/eventbroker"
	"github.com/aspirit/xsocial/pkg/logger"
)

// pubSubPushMessage is the request body format from GCP Pub/Sub Push.
// See https://cloud.google.com/pubsub/docs/push
type pubSubPushMessage struct {
	Message struct {
		Data       string            `json:"data"`
		MessageID  string            `json:"messageId"`
		Attributes map[string]string `json:"attributes"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// PubSubPush returns an HTTP handler for Pub/Sub Push subscription requests.
// Converts push body to eventbroker.Message and passes to the message processor.
func PubSubPush(processor pkgeventbroker.Handler, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

			return
		}

		var req pubSubPushMessage
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("Invalid Pub/Sub push body", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)

			return
		}

		data, err := base64.StdEncoding.DecodeString(req.Message.Data)
		if err != nil {
			log.Warn("Failed to decode Pub/Sub message data", zap.Error(err))
			http.Error(w, "Bad request", http.StatusBadRequest)

			return
		}

		topic := req.Message.Attributes["topic"]
		if topic == "" {
			topic = TopicNotificationEvent
		}

		msg := &pkgeventbroker.Message{
			Topic:   topic,
			Value:   data,
			Headers: req.Message.Attributes,
		}

		ctx := logger.WithLogger(r.Context(), log)
		if err := processor.Handle(ctx, msg); err != nil {
			log.Warn("Failed to process message",
				zap.String("topic", topic),
				zap.Error(err),
			)
			// Return 5xx so Pub/Sub will retry
			http.Error(w, "Processing failed", http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
