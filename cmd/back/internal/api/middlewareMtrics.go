package api

import (
	"net/http"
	"strings"
	"time"
	"twitter/internal/metrics"
)

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Создаем кастомный ResponseWriter для отслеживания статуса
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := sanitizePath(r.URL.Path)

		// Записываем метрики
		metrics.HttpRequestsTotal.WithLabelValues(r.Method, path, http.StatusText(rw.statusCode)).Inc()
		metrics.HttpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func sanitizePath(path string) string {
	// Убираем параметры из пути для группировки метрик
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	return path
}
