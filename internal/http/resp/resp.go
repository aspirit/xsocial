package resp

import (
	"net/http"

	"github.com/aspirit/xsocial/internal/apierrors"

	"github.com/gin-gonic/gin"
)

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Fail writes the {code, message} error envelope. Every error goes through
// apierrors.Convert, so unmapped errors become a generic 500 and are logged
// instead of leaking internal text to the client.
func Fail(c *gin.Context, err error) {
	apiErr := apierrors.Convert(err)
	if apiErr == nil { // only for a nil error, which is a caller bug
		apiErr = apierrors.ErrInternalError
	}

	c.Set(ContextKeyMessage, apiErr.Message)
	c.Set(ContextKeyStatus, apiErr.Code)
	c.JSON(apiErr.HTTPStatus, gin.H{
		FieldCode:    apiErr.Code,
		FieldMessage: apiErr.Message,
	})
}

func Finish(c *gin.Context, err error, data any) {
	if err != nil {
		Fail(c, err)

		return
	}
	Success(c, data)
}
