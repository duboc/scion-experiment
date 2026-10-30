package api

import "github.com/gin-gonic/gin"

// registerWave6Routes holds every route added in Wave 6 (#39-#44,
// ADR-012..016). It lives in its own file so that backend route
// registration never touches routes.go, which frontend owns because it
// contains defaultStorefrontHTML.
func registerWave6Routes(router *gin.Engine, controller *Controller) {
	_ = router
	_ = controller
}
