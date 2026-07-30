package main

import (
	"context"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"livechat/backend/internal/appstate"
	"livechat/backend/internal/config"
	appdb "livechat/backend/internal/db"
	"livechat/backend/internal/handlers"
	"livechat/backend/internal/middleware"
	"livechat/backend/internal/redisclient"
	"livechat/backend/internal/storage"
	"livechat/backend/internal/ws"
	"livechat/backend/internal/wsserver"
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
		// Every migration file is idempotent (CREATE TABLE IF NOT EXISTS /
		// INSERT ... ON DUPLICATE KEY), so re-running the full set on every
		// boot is safe — this is what actually applies new migrations
		// added after the Setup Wizard already ran once.
		if err := appdb.RunMigrations(conn, cfg.MigrationsPath); err != nil {
			log.Fatal("startup migration failed: ", err)
		}
		state.SetDB(conn)
		log.Println("connected to MySQL:", cfg.DBName)
	} else {
		log.Println("MySQL not configured/reachable yet — Setup Wizard required:", err)
	}

	// Kept open for the lifetime of the process — the ws.Hub's pub/sub
	// loop and presence tracking both need a live client, not just a
	// one-off reachability check. nil is a valid value throughout: the
	// hub and presence package both fall back to local-only behavior.
	redisClient, err := redisclient.Connect(cfg)
	if err != nil {
		log.Println("Redis not reachable (optional for Phase 0, required for multi-instance later):", err)
		redisClient = nil
	} else {
		log.Println("Redis reachable")
	}

	hub := ws.NewHub(context.Background(), redisClient)
	wsserver.Start(cfg, state, hub, redisClient)
	fileDriver := storage.NewLocalDriver(cfg.UploadsPath)

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

		// Any staff role (agent/admin/super_admin) — chatAccess/scopedMerchantIDs
		// inside each handler narrow further by merchant.
		staffChats := authed.Group("/chats")
		staffChats.GET("", handlers.ListChatsHandler(state))
		staffChats.GET("/:uuid", handlers.GetChatHandler(state))
		staffChats.POST("/:uuid/claim", handlers.ClaimChatHandler(state, hub))
		staffChats.POST("/:uuid/assign", handlers.AssignChatHandler(state, hub))
		staffChats.POST("/:uuid/close", handlers.CloseChatHandler(state, hub))
		staffChats.POST("/:uuid/messages", handlers.SendMessageHandler(state, hub))
		staffChats.POST("/:uuid/files", handlers.UploadFileHandler(state, hub, fileDriver))

		authed.GET("/dashboard/summary", handlers.DashboardSummaryHandler(state, redisClient))
		authed.GET("/files/:uuid", handlers.DownloadFileHandler(state, fileDriver))

		// Visitor-facing — unauthenticated by design (a real website
		// visitor isn't logged in), validated via the (visitor, chat) uuid
		// pair instead. Doubles as the Phase 2 internal test harness.
		visitorChats := api.Group("/visitor")
		visitorChats.Use(requireDB(state))
		visitorChats.POST("/start", handlers.StartChatHandler(state, hub, redisClient))
		visitorChats.GET("/chats/:uuid", handlers.GetVisitorChatHandler(state))
		visitorChats.POST("/chats/:uuid/messages", handlers.SendVisitorMessageHandler(state, hub))
		visitorChats.POST("/chats/:uuid/files", handlers.UploadVisitorFileHandler(state, hub, fileDriver))
		visitorChats.GET("/files/:uuid", handlers.DownloadVisitorFileHandler(state, fileDriver))
	}

	log.Println("listening on :" + cfg.AppPort)
	if err := router.Run(":" + cfg.AppPort); err != nil {
		log.Fatal(err)
	}
}
