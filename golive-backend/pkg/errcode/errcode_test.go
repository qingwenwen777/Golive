package errcode_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/qingwenwen777/golive/pkg/errcode"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRespond_WritesAppErrorStatusMessageAndReason(t *testing.T) {
	router := gin.New()
	router.GET("/coins", func(c *gin.Context) {
		errcode.Respond(c, errcode.ErrInsufficientCoin)
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/coins", nil))

	require.Equal(t, http.StatusPaymentRequired, rec.Code)
	require.JSONEq(t, `{"message":"insufficient coin balance","reason":"insufficient_coin"}`, rec.Body.String())
}

func TestRespond_HidesPlainErrorDetails(t *testing.T) {
	router := gin.New()
	router.GET("/boom", func(c *gin.Context) {
		errcode.Respond(c, errors.New("Error 1406: Data too long for column 'reason' at row 1"))
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"message":"internal error"}`, rec.Body.String())
}

func TestRespondWith_MergesReasonAndPayload(t *testing.T) {
	router := gin.New()
	router.POST("/gift", func(c *gin.Context) {
		errcode.RespondWith(c, errcode.ErrInsufficientCoin, gin.H{
			"order": gin.H{
				"giftId": "rocket",
				"status": "failed",
			},
		})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/gift", nil))

	require.Equal(t, http.StatusPaymentRequired, rec.Code)
	require.JSONEq(t, `{
		"message":"insufficient coin balance",
		"reason":"insufficient_coin",
		"order":{"giftId":"rocket","status":"failed"}
	}`, rec.Body.String())
}
