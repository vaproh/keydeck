package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *App) AdminRouter() http.Handler {
	r := chi.NewRouter()
	r.Mount("/auth", a.AuthRouter())
	r.Group(func(r chi.Router) {
		r.Use(a.auth.Middleware)
		r.Route("/admin", func(r chi.Router) {
			r.Get("/tokens", a.listAdminTokens)
			r.Post("/tokens", a.createAdminToken)
			r.Post("/tokens/{id}/revoke", a.revokeAdminToken)

			r.Get("/stats", a.stats)
			r.Get("/products", a.listProducts)
			r.Post("/products", a.createProduct)
			r.Get("/products/{id}", a.getProduct)
			r.Patch("/products/{id}", a.updateProduct)
			r.Post("/products/{id}/limits", a.addLimit)
			r.Delete("/products/{id}/limits/{limitID}", a.deleteLimit)
			r.Post("/products/{id}/features", a.addFeature)
			r.Delete("/products/{id}/features/{featureID}", a.deleteFeature)
			r.Post("/products/{id}/presets", a.createPreset)
			r.Patch("/products/{id}/presets/{presetID}", a.updatePreset)
			r.Delete("/products/{id}/presets/{presetID}", a.deletePreset)
			r.Get("/products/{id}/overview", a.productOverview)
			r.Post("/products/{id}/licenses", a.createLicense)

			r.Get("/licenses", a.listLicenses)
			r.Get("/licenses/{id}", a.getLicense)
			r.Patch("/licenses/{id}", a.updateLicense)
			r.Post("/licenses/{id}/{action}", a.licenseAction)
			r.Post("/licenses/{id}/activations/{activationID}/revoke", a.revokeActivation)

			r.Get("/releases", a.listReleases)
			r.Post("/releases", a.uploadRelease)
			r.Get("/audit", a.auditList)
			r.Get("/heartbeats", a.listHeartbeats)
		})
	})
	return r
}
