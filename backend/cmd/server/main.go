package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/config"
	appdb "livechat/backend/internal/db"
	"livechat/backend/internal/handlers"
	"livechat/backend/internal/middleware"
	"livechat/backend/internal/redisclient"
)

func corsMiddleware(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// requireDB guards routes that need the app database to already be
// configured (i.e. the Setup Wizard has run) — checked live on every
// request since state.DB() may flip from nil to set mid-process.
func requireDB(state *appstate.State) gin.HandlerFunc {
	return func(c *gin.Context) {
		if state.DB() == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "not_configured", "detail": "run the Setup Wizard first"})
			return
		}
		c.Next()
	}
}

func main() {
	cfg := config.Load()

	state := appstate.New(nil)
	if conn, err := appdb.Connect(cfg); err == nil {
		state.SetDB(conn)
		log.Println("connected to MySQL:", cfg.DBName)
	} else {
		log.Println("MySQL not configured/reachable yet — Setup Wizard required:", err)
	}

	if client, err := redisclient.Connect(cfg); err == nil {
		client.Close()
		log.Println("Redis reachable")
	} else {
		log.Println("Redis not reachable (optional for Phase 0):", err)
	}

	router := gin.Default()
	router.Use(corsMiddleware(cfg.FrontendOrigin))

	api := router.Group("/api")
	{
		setup := api.Group("/setup")
		setup.GET("/status", handlers.StatusHandler(state))
		setup.GET("/checklist", handlers.ChecklistHandler(cfg))
		setup.POST("/db/test", handlers.DBTestHandler())
		setup.POST("/finish", handlers.FinishHandler(cfg, ".env", state))

		auth := api.Group("/auth")
		auth.Use(requireDB(state))
		auth.POST("/login", handlers.LoginHandler(state))
		auth.POST("/logout", middleware.RequireAuth(state), handlers.LogoutHandler(state))
		auth.GET("/me", middleware.RequireAuth(state), handlers.MeHandler(state))

		// Everything below requires a configured DB + an authenticated
		// session; RBAC (RequireRole) narrows further per-route.
		authed := api.Group("")
		authed.Use(requireDB(state), middleware.RequireAuth(state))

		profile := authed.Group("/profile")
		profile.PATCH("", handlers.UpdateProfileHandler(state))
		profile.POST("/password", handlers.ChangePasswordHandler(state))

		staffOnly := middleware.RequireRole("admin", "super_admin")

		merchants := authed.Group("/merchants")
		merchants.Use(staffOnly)
		merchants.GET("", handlers.ListMerchantsHandler(state))
		merchants.POST("", middleware.RequireRole("super_admin"), handlers.CreateMerchantHandler(state))
		merchants.PATCH("/:uuid/status", middleware.RequireRole("super_admin"), handlers.SetMerchantStatusHandler(state))
		merchants.POST("/:uuid/admins", middleware.RequireRole("super_admin"), handlers.AssignMerchantAdminHandler(state))

		users := authed.Group("/users")
		users.Use(staffOnly)
		users.GET("", handlers.ListUsersHandler(state))
		users.POST("", handlers.CreateUserHandler(state))
		users.PATCH("/:uuid/status", handlers.SetUserStatusHandler(state))
		users.POST("/:uuid/force-password", handlers.ForcePasswordHandler(state))
		users.POST("/:uuid/merchants", handlers.GrantUserMerchantHandler(state))
		users.DELETE("/:uuid/merchants/:merchantUuid", handlers.RevokeUserMerchantHandler(state))
	}

	log.Println("listening on :" + cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatal(err)
	}
}
