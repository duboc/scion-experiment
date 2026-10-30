package api

import "github.com/gin-gonic/gin"

// registerWave6Routes holds every route added in Wave 6 (#39-#44,
// ADR-012..016). It lives in its own file so that backend route
// registration never touches routes.go, which frontend owns because it
// contains defaultStorefrontHTML.
func registerWave6Routes(router *gin.Engine, controller *Controller) {
	// Issue #42 (ADR-014): dynamic product attributes upsert (admin-guarded).
	router.PUT("/products/:id/attributes", controller.UpsertProductAttributes)
	router.PUT("/api/products/:id/attributes", controller.UpsertProductAttributes)

	// Issue #44 (ADR-016): item-to-item similar products, in-stock substitutes & co-visitation.
	router.GET("/products/:id/similar", controller.SimilarProducts)
	router.GET("/api/products/:id/similar", controller.SimilarProducts)

	// Issue #43 (ADR-015): closed-loop search telemetry, click attribution, A/B KPIs & LTR rollup.
	router.POST("/analytics/events", controller.RecordAnalyticsEvent)
	router.POST("/api/analytics/events", controller.RecordAnalyticsEvent)
	router.GET("/analytics/summary", controller.AnalyticsSummary)
	router.GET("/api/analytics/summary", controller.AnalyticsSummary)
	router.POST("/analytics/rollup", controller.RollupAnalytics)
	router.POST("/api/analytics/rollup", controller.RollupAnalytics)
}
