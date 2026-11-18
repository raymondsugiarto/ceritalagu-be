package i18n

import "github.com/gofiber/fiber/v3"

// Middleware resolves the request locale (Resolve) and stores it in the request context, so
// services read it with FromContext(ctx) from the ctx their handler passes down.
func Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		l := Resolve(c.Query(QueryParam), c.Cookies(CookieName), c.Get(fiber.HeaderAcceptLanguage))
		c.SetContext(WithLocale(c.Context(), l))
		return c.Next()
	}
}
