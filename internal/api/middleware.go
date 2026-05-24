package api

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/slash-init/goflow/pkg/logger"
	"github.com/slash-init/goflow/pkg/metrics"
	"go.uber.org/zap"
)

func LoggerMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		logger.Info("http request",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Duration("duration", duration),
			zap.String("ip", c.IP()),
		)

		metrics.HTTPRequestDuration.
			WithLabelValues(
				c.Method(),
				c.Route().Path,
				strconv.Itoa(status),
			).
			Observe(duration.Seconds())

		return err
	}
}
