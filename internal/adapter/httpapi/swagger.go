package httpapi

import (
	"net/http"
)

const swaggerUIHTML = `<!doctype html>
<html lang="zh-Hant">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>REGS API 文件</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({
        url: "/openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        persistAuthorization: true
      });
    };
  </script>
</body>
</html>`

// swaggerUI 回傳使用同源 OpenAPI 規格的 Swagger UI 頁面。
func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

// serveOpenAPISpec 從設定路徑提供 OpenAPI YAML 規格。
func serveOpenAPISpec(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if path == "" {
			http.Error(w, "OpenAPI specification is not configured", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		http.ServeFile(w, r, path)
	}
}
