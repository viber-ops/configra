package web

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist
var files embed.FS

func NewHandler() http.Handler {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic("embedded Management UI is unavailable")
	}
	assets := http.StripPrefix("/ui/", http.FileServerFS(dist))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		setSecurityHeaders(response)
		if request.URL.Path == "/" || request.URL.Path == "/ui/" {
			nonce, nonceErr := newCSPNonce()
			if nonceErr != nil {
				http.Error(response, "Management UI unavailable", http.StatusInternalServerError)
				return
			}
			response.Header().Set("Content-Security-Policy", contentSecurityPolicy(nonce))
			response.Header().Set("Cache-Control", "no-store")
			response.Header().Set("Content-Type", "text/html; charset=utf-8")
			index, readErr := fs.ReadFile(dist, "index.html")
			if readErr != nil {
				http.Error(response, "Management UI unavailable", http.StatusInternalServerError)
				return
			}
			index = bytes.Replace(index, []byte("</head>"), []byte(`<meta name="csp-nonce" content="`+nonce+`"></head>`), 1)
			_, _ = response.Write(index)
			return
		}
		if !strings.HasPrefix(request.URL.Path, "/ui/assets/") || strings.HasSuffix(request.URL.Path, "/") {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		assets.ServeHTTP(response, request)
	})
}

func setSecurityHeaders(response http.ResponseWriter) {
	response.Header().Set("Content-Security-Policy", contentSecurityPolicy(""))
	response.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	response.Header().Set("Referrer-Policy", "no-referrer")
	response.Header().Set("X-Content-Type-Options", "nosniff")
}

func contentSecurityPolicy(nonce string) string {
	styleSource := "style-src 'self'"
	if nonce != "" {
		styleSource += " 'nonce-" + nonce + "'"
	}
	return "default-src 'self'; script-src 'self'; " + styleSource + "; img-src 'self' data:; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
}

func newCSPNonce() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
