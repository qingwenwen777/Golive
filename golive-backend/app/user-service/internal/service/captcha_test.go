package service

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRandomCaptchaCodeUsesReadableAlphabet(t *testing.T) {
	const readableAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

	code, err := randomCaptchaCode(32)
	require.NoError(t, err)
	require.Len(t, code, 32)

	for _, ch := range code {
		require.Contains(t, readableAlphabet, string(ch))
		require.NotContains(t, "01IO", string(ch))
	}
}

func TestRenderCaptchaSVGKeepsCodeAsShapes(t *testing.T) {
	const code = "AB234Z"

	image, err := renderCaptchaSVG(code)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(image, "data:image/svg+xml;base64,"))

	payload := strings.TrimPrefix(image, "data:image/svg+xml;base64,")
	decoded, err := base64.StdEncoding.DecodeString(payload)
	require.NoError(t, err)

	svg := string(decoded)
	require.Contains(t, svg, `viewBox="0 0 132 44"`)
	require.Contains(t, svg, `<rect`)
	require.NotContains(t, svg, code)
	require.NotContains(t, svg, `<text`)
	require.NotContains(t, svg, `feDisplacementMap`)
}
