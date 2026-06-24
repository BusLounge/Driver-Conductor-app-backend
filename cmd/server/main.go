package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/smarttransit/sms-auth-backend/internal/config"
	"github.com/smarttransit/sms-auth-backend/internal/database"
	"github.com/smarttransit/sms-auth-backend/internal/handlers"
	"github.com/smarttransit/sms-auth-backend/internal/middleware"
	"github.com/smarttransit/sms-auth-backend/internal/services"
	"github.com/smarttransit/sms-auth-backend/pkg/jwt"
	"github.com/smarttransit/sms-auth-backend/pkg/onesignal"
	"github.com/smarttransit/sms-auth-backend/pkg/sms"
	"github.com/smarttransit/sms-auth-backend/pkg/validator"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Initialize logger
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)

	logger.Info("Starting SmartTransit SMS Authentication Backend")
	logger.Infof("Version: %s, Build Time: %s", version, buildTime)
	logger.Info("🔍 DEBUG: Lounge Owner registration system ENABLED")
	logger.Info("🔍 DEBUG: This build includes lounge owner routes")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("Failed to load configuration: %v", err)
	}

	// Set log level
	logLevel, err := logrus.ParseLevel(cfg.Server.LogLevel)
	if err != nil {
		logger.Warn("Invalid log level, using INFO")
		logLevel = logrus.InfoLevel
	}
	logger.SetLevel(logLevel)

	// Set Gin mode
	if cfg.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Initialize database connection
	logger.Info("Connecting to database...")
	db, err := database.NewConnection(cfg.Database)
	if err != nil {
		logger.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	logger.Info("Database connection established")

	// Test database connection
	if err := db.Ping(); err != nil {
		logger.Fatalf("Failed to ping database: %v", err)
	}

	// Initialize services
	logger.Info("Initializing services...")
	jwtService := jwt.NewService(
		cfg.JWT.Secret,
		cfg.JWT.RefreshSecret,
		cfg.JWT.AccessTokenExpiry,
		cfg.JWT.RefreshTokenExpiry,
	)
	otpService := services.NewOTPService(db)
	phoneValidator := validator.NewPhoneValidator()
	rateLimitService := services.NewRateLimitService(db)
	auditService := services.NewAuditService(db)
	userRepository := database.NewUserRepository(db)
	refreshTokenRepository := database.NewRefreshTokenRepository(db)
	userSessionRepository := database.NewUserSessionRepository(db)

	// Initialize passenger repository
	passengerRepository := database.NewPassengerRepository(db)

	// Initialize staff-related repositories
	staffRepository := database.NewBusStaffRepository(db)
	ownerRepository := database.NewBusOwnerRepository(db)
	permitRepository := database.NewRoutePermitRepository(db)
	busRepository := database.NewBusRepository(db)

	// Initialize booking repository
	bookingRepository := database.NewBookingRepository(db)

	// Initialize lounge owner repositories
	// Type assertion needed: db is interface DB, but repositories need *sqlx.DB
	sqlxDB, ok := db.(*database.PostgresDB)
	if !ok {
		logger.Fatal("Failed to cast database connection to PostgresDB")
	}
	loungeOwnerRepository := database.NewLoungeOwnerRepository(sqlxDB.DB)
	loungeRepository := database.NewLoungeRepository(sqlxDB.DB)
	loungeStaffRepository := database.NewLoungeStaffRepository(sqlxDB.DB)
	loungeDriverRepository := database.NewLoungeDriverRepository(sqlxDB.DB)
	loungeTransportLocationRepository := database.NewLoungeTransportLocationRepository(sqlxDB.DB)
	loungeTransportLocationPriceRepository := database.NewLoungeTransportLocationPriceRepository(sqlxDB.DB)
	seatLayoutRepository := database.NewBusSeatLayoutRepository(sqlxDB.DB)

	// Initialize staff service
	staffService := services.NewStaffService(staffRepository, ownerRepository, userRepository)

	// NOTE: Active trip service is initialized after repositories are ready (see below)

	// Initialize trip scheduling repositories
	tripScheduleRepo := database.NewTripScheduleRepository(sqlxDB.DB)
	scheduledTripRepo := database.NewScheduledTripRepository(sqlxDB.DB)
	masterRouteRepo := database.NewMasterRouteRepository(sqlxDB.DB)
	systemSettingRepo := database.NewSystemSettingRepository(sqlxDB.DB)

	// Initialize active trip repository (for real-time trip tracking)
	activeTripRepo := database.NewActiveTripRepository(db)

	// Initialize trip generator service
	tripGeneratorSvc := services.NewTripGeneratorService(
		tripScheduleRepo,
		scheduledTripRepo,
		busRepository,
		seatLayoutRepository,
		systemSettingRepo,
	)

	// Initialize SMS Gateway (Dialog)
	var smsGateway sms.SMSGateway

	// Get both app hashes for SMS auto-read
	driverAppHash := cfg.SMS.DriverAppHash
	passengerAppHash := cfg.SMS.PassengerAppHash

	if driverAppHash != "" || passengerAppHash != "" {
		logger.Info("SMS auto-read enabled:")
		if driverAppHash != "" {
			logger.Info("  Driver app hash: " + driverAppHash)
		}
		if passengerAppHash != "" {
			logger.Info("  Passenger app hash: " + passengerAppHash)
		}
	}

	if cfg.SMS.Mode == "production" {
		logger.Info("Initializing Dialog SMS Gateway in production mode...")

		// Choose gateway based on method
		if cfg.SMS.Method == "url" {
			logger.Info("Using Dialog URL method (GET request with esmsqk)")
			urlGateway := sms.NewDialogURLGateway(cfg.SMS.ESMSQK, cfg.SMS.Mask, driverAppHash, passengerAppHash)
			smsGateway = urlGateway
		} else {
			logger.Info("Using Dialog API v2 method (POST with authentication)")
			apiGateway := sms.NewDialogGateway(sms.DialogConfig{
				APIURL:           cfg.SMS.APIURL,
				Username:         cfg.SMS.Username,
				Password:         cfg.SMS.Password,
				Mask:             cfg.SMS.Mask,
				DriverAppHash:    driverAppHash,
				PassengerAppHash: passengerAppHash,
			})
			smsGateway = apiGateway
		}

		logger.Info("Dialog SMS Gateway initialized")
	} else {
		logger.Info("SMS Gateway in development mode (no actual SMS will be sent)")
		// Still initialize but won't be used in dev mode
		smsGateway = sms.NewDialogGateway(sms.DialogConfig{
			APIURL:           cfg.SMS.APIURL,
			Username:         cfg.SMS.Username,
			Password:         cfg.SMS.Password,
			Mask:             cfg.SMS.Mask,
			DriverAppHash:    driverAppHash,
			PassengerAppHash: passengerAppHash,
		})
	}

	logger.Info("Services initialized")

	// Initialize OneSignal service (for push notifications)
	oneSignalService := onesignal.NewOneSignalService(
		cfg.OneSignalAppID,
		cfg.OneSignalRestAPIKey,
	)
	if cfg.OneSignalAppID == "" || cfg.OneSignalRestAPIKey == "" {
		logger.Warn("⚠️ OneSignal credentials not configured - push notifications will not work")
	} else {
		logger.Info("OneSignal credentials configured ")
	}

	// Initialize notification service (with OneSignal service and required repositories)
	notificationService := services.NewNotificationService(
		oneSignalService,
		userSessionRepository,
		loungeRepository,
		loungeOwnerRepository,
	)

	logger.Info("OneSignal push notification Services initialized")

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(
		jwtService,
		otpService,
		phoneValidator,
		rateLimitService,
		auditService,
		notificationService, // ✅ Add notificationService
		userRepository,
		passengerRepository,
		refreshTokenRepository,
		userSessionRepository,
		smsGateway,
		cfg,
		loungeRepository,
	)

	// Initialize staff handler
	staffHandler := handlers.NewStaffHandler(staffService, userRepository, staffRepository, scheduledTripRepo)

	// Initialize active trip service and handler (for Start Trip / End Trip / Location tracking)
	logger.Info("🚌 Initializing Active Trip tracking system...")
	activeTripService := services.NewActiveTripService(
		activeTripRepo,
		scheduledTripRepo,
		staffRepository,
		busRepository,
		permitRepository,
	)
	activeTripHandler := handlers.NewActiveTripHandler(activeTripService, staffRepository)
	logger.Info("✓ Active Trip tracking system initialized")

	// Initialize bus owner and permit handlers
	busOwnerHandler := handlers.NewBusOwnerHandler(ownerRepository, permitRepository, userRepository, staffRepository)
	permitHandler := handlers.NewPermitHandler(permitRepository, ownerRepository, masterRouteRepo)
	busHandler := handlers.NewBusHandler(busRepository, permitRepository, ownerRepository)
	masterRouteHandler := handlers.NewMasterRouteHandler(masterRouteRepo)
	districtRepo := database.NewDistrictRepository(db)
	districtHandler := handlers.NewDistrictHandler(districtRepo)
	loungeOwnerDistrictRepo := database.NewLoungeOwnerDistrictRepository(db)
	loungeOwnerDistrictHandler := handlers.NewLoungeOwnerDistrictHandler(loungeOwnerDistrictRepo)

	// Initialize bus owner route repository and handler
	busOwnerRouteRepo := database.NewBusOwnerRouteRepository(db)
	busOwnerRouteHandler := handlers.NewBusOwnerRouteHandler(busOwnerRouteRepo, ownerRepository)

	// Initialize lounge owner, lounge, staff, lounge driver, lounge transport location, lounge tranport location price and admin handlers
	logger.Info("🔍 DEBUG: Initializing lounge handlers...")
	loungeOwnerHandler := handlers.NewLoungeOwnerHandler(loungeOwnerRepository, userRepository, loungeRepository) //added lounge repository new
	loungeRouteRepository := database.NewLoungeRouteRepository(sqlxDB.DB)
	loungeHandler := handlers.NewLoungeHandler(loungeRepository, loungeOwnerRepository, loungeRouteRepository)
	// Initialize lounge owner bank repositories and handler
	loungeOwnerBankRepo := database.NewLoungeOwnerBankDetailsRepository(sqlxDB.DB)
	loungeOwnerBankLinkRepo := database.NewLoungeOwnerBankLinkRepository(sqlxDB.DB)
	loungeOwnerBankHandler := handlers.NewLoungeOwnerBankHandler(
		loungeOwnerBankRepo,
		loungeOwnerBankLinkRepo,
		loungeOwnerRepository,
	)

	// Initialize lounge booking system first before staff handler
	logger.Info("🏨 Initializing lounge booking system...")
	loungeBookingRepo := database.NewLoungeBookingRepository(sqlxDB.DB)
	logger.Info("✓ Lounge booking system initialized")

	loungeStaffHandler := handlers.NewLoungeStaffHandler(loungeStaffRepository, loungeRepository, loungeOwnerRepository, userRepository, phoneValidator, loungeStaffRepository, bookingRepository, loungeBookingRepo)
	loungeDriverHandler := handlers.NewLoungeDriverHandler(loungeOwnerRepository, loungeRepository, loungeDriverRepository)
	loungeTransportLocationHandler := handlers.NewLoungeTransportLocationHandler(loungeOwnerRepository, loungeRepository, loungeTransportLocationRepository)
	loungeTransportLocationPriceHandler := handlers.NewLoungeTransportLocationPriceHandler(loungeOwnerRepository, loungeRepository, loungeTransportLocationRepository, loungeTransportLocationPriceRepository)

	// Initialize lounge booking handler
	logger.Info("🏨 Initializing lounge booking handler...")
	loungeBookingHandler := handlers.NewLoungeBookingHandler(loungeBookingRepo, loungeRepository, loungeOwnerRepository, loungeStaffRepository)
	logger.Info("✓ Lounge booking handler initialized")

	cloudinaryStorageService, cloudinaryErr := services.NewCloudinaryStorageService(cfg.CloudinaryURL, cfg.Server.Environment)
	if cloudinaryErr != nil {
		logger.Warnf("Cloudinary storage is not configured: %v", cloudinaryErr)
	}
	storageHandler := handlers.NewStorageHandler(cloudinaryStorageService, loungeOwnerRepository, loungeRepository)

	// Initialize lounge special packages system
	logger.Info("📦 Initializing lounge special packages system...")
	loungeSpecialPackageRepo := database.NewLoungeSpecialPackageRepository(db)
	loungeSpecialPackageHandler := handlers.NewLoungeSpecialPackageHandler(loungeSpecialPackageRepo, loungeRepository, loungeOwnerRepository)
	logger.Info("✓ Lounge special packages system initialized")

	// Initialize Lounge Booking Driver Assignment system
	logger.Info("Initializing lounge booking driver assignment system...")
	loungeBookingDriverAssignmentRepo := database.NewLoungeBookingDriverAssignmentRepository(sqlxDB.DB)
	loungeBookingDriverAssignmentHandler := handlers.NewLoungeBookingDriverAssignmentHandler(
		loungeBookingDriverAssignmentRepo,
		loungeOwnerRepository,
		loungeRepository,
		loungeBookingRepo,
		loungeDriverRepository,
		smsGateway,
	)
	logger.Info("✓ Lounge booking driver assignment system initialized")

	logger.Info("🔍 DEBUG: Lounge handlers initialized successfully")
	adminHandler := handlers.NewAdminHandler(loungeOwnerRepository, loungeRepository, userRepository)

	// Initialize admin authentication repository, service, and handler
	logger.Info("Initializing admin authentication system...")
	adminUserRepository := database.NewAdminUserRepository(db)
	adminRefreshTokenRepository := database.NewAdminRefreshTokenRepository(db)
	adminAuthService := services.NewAdminAuthService(
		adminUserRepository,
		adminRefreshTokenRepository,
		jwtService,
		cfg.JWT.AccessTokenExpiry,
		cfg.JWT.RefreshTokenExpiry,
	)
	adminAuthHandler := handlers.NewAdminAuthHandler(adminAuthService, logger)
	logger.Info("✓ Admin authentication system initialized")

	// Initialize bus seat layout system
	logger.Info("Initializing bus seat layout system...")
	busSeatLayoutRepository := database.NewBusSeatLayoutRepository(db)
	busSeatLayoutService := services.NewBusSeatLayoutService(busSeatLayoutRepository)
	busSeatLayoutHandler := handlers.NewBusSeatLayoutHandler(busSeatLayoutService, logger)
	logger.Info("✓ Bus seat layout system initialized")

	// Initialize trip scheduling handlers
	tripScheduleHandler := handlers.NewTripScheduleHandler(
		tripScheduleRepo,
		permitRepository,
		ownerRepository,
		busRepository,
		busOwnerRouteRepo,
		tripGeneratorSvc,
	)

	// Initialize Trip Seat and Manual Booking system
	logger.Info("Initializing trip seat and manual booking system...")
	tripSeatRepo := database.NewTripSeatRepository(sqlxDB.DB)
	manualBookingRepo := database.NewManualBookingRepository(sqlxDB.DB)
	logger.Info("✓ Trip seat and manual booking repositories initialized")

	scheduledTripHandler := handlers.NewScheduledTripHandler(
		scheduledTripRepo,
		tripScheduleRepo,
		permitRepository,
		ownerRepository,
		busOwnerRouteRepo,
		busRepository,
		staffRepository,
		systemSettingRepo,
		tripSeatRepo,
	)
	systemSettingHandler := handlers.NewSystemSettingHandler(systemSettingRepo)
	logger.Info("Trip scheduling handlers initialized")

	// Initialize search system
	logger.Info("Initializing search system...")
	searchRepo := database.NewSearchRepository(db)
	searchService := services.NewSearchService(searchRepo, logger)
	searchHandler := handlers.NewSearchHandler(searchService, logger)
	logger.Info("✓ Search system initialized")

	// Initialize Trip Seat Handler (tripSeatRepo already initialized above)
	tripSeatHandler := handlers.NewTripSeatHandler(
		tripSeatRepo,
		manualBookingRepo,
		scheduledTripRepo,
		ownerRepository,
		busOwnerRouteRepo,
	)
	logger.Info("✓ Trip seat handler initialized")

	// Initialize App Booking system (passenger app bookings)
	logger.Info("Initializing app booking system...")
	appBookingRepo := database.NewAppBookingRepository(sqlxDB.DB)
	appBookingHandler := handlers.NewAppBookingHandler(
		appBookingRepo,
		scheduledTripRepo,
		tripSeatRepo,
		busOwnerRouteRepo,
		logger,
	)
	staffBookingHandler := handlers.NewStaffBookingHandler(appBookingRepo)
	logger.Info("✓ App booking system initialized")

	// ============================================================================
	// BOOKING ORCHESTRATION SYSTEM (Intent → Payment → Confirm)
	// ============================================================================
	logger.Info("🎯 Initializing Booking Orchestration system...")
	bookingIntentRepo := database.NewBookingIntentRepository(sqlxDB.DB)
	bookingOrchestratorConfig := services.DefaultOrchestratorConfig()

	// Initialize PAYable payment service
	payableService := services.NewPAYableService(&cfg.Payment, logger)
	if payableService.IsConfigured() {
		logger.WithField("environment", payableService.GetEnvironment()).Info("✓ PAYable payment gateway configured")
	} else {
		logger.Warn("⚠️ PAYable payment gateway not configured - using placeholder mode")
	}

	// Initialize payment audit repository for logging all payment events
	paymentAuditRepo := database.NewPaymentAuditRepository(sqlxDB.DB, logger)
	logger.Info("✓ Payment audit repository initialized")

	bookingOrchestratorService := services.NewBookingOrchestratorService(
		bookingIntentRepo,
		tripSeatRepo,
		scheduledTripRepo,
		appBookingRepo,
		loungeBookingRepo,
		loungeRepository,
		busOwnerRouteRepo,
		payableService,
		bookingOrchestratorConfig,
		logger,
	)
	bookingOrchestratorHandler := handlers.NewBookingOrchestratorHandler(
		bookingOrchestratorService,
		payableService,
		paymentAuditRepo,
		logger,
	)
	logger.Info("✓ Booking Orchestration system initialized")

	// Start background job for intent expiration
	intentExpirationService := services.NewIntentExpirationService(bookingIntentRepo, logger)
	intentExpirationService.Start()
	defer intentExpirationService.Stop()

	// Initialize Gin router
	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(requestLogger(logger))

	// CORS configuration
	corsConfig := cors.Config{
		AllowOrigins:     cfg.CORS.AllowedOrigins,
		AllowMethods:     cfg.CORS.AllowedMethods,
		AllowHeaders:     cfg.CORS.AllowedHeaders,
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsConfig))

	// Health check endpoint
	router.GET("/health", healthCheckHandler(db))

	// Set environment in context for development mode
	router.Use(func(c *gin.Context) {
		c.Set("environment", cfg.Server.Environment)
		c.Next()
	})

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Debug endpoint - shows all request headers and IP detection (public)
		v1.GET("/debug/headers", debugHeadersHandler())

		// Debug endpoint - list all registered routes
		v1.GET("/debug/routes", func(c *gin.Context) {
			routes := router.Routes()
			routeList := make([]map[string]string, 0)
			for _, route := range routes {
				routeList = append(routeList, map[string]string{
					"method": route.Method,
					"path":   route.Path,
				})
			}
			c.JSON(200, gin.H{
				"message":      "Registered routes",
				"total_routes": len(routeList),
				"routes":       routeList,
			})
		})

		// Authentication routes (public)
		auth := v1.Group("/auth")
		{
			auth.POST("/send-otp", authHandler.SendOTP)
			auth.POST("/verify-otp", authHandler.VerifyOTP)
			auth.POST("/verify-otp-generic", authHandler.VerifyOTPGeneric)
			auth.POST("/verify-otp-staff", authHandler.VerifyOTPStaff)
			// lounge_owner_specific routes
			auth.POST("/verify-otp-lounge-owner", func(c *gin.Context) {
				authHandler.VerifyOTPLoungeOwner(c, loungeOwnerRepository)
			})
			// lounge_staff_specific routes
			auth.POST("/verify-otp-lounge-staff", func(c *gin.Context) {
				authHandler.VerifyOTPLoungeStaff(c, loungeStaffRepository)
			})
			// lounge_staff_registered routes
			auth.POST("/verify-otp-lounge-staff-registered", func(c *gin.Context) {
				authHandler.VerifyOTPLoungeStaffRegistered(c, loungeStaffRepository)
			})
			// Lounge owner-specific endpoint
			auth.GET("/otp-status/:phone", authHandler.GetOTPStatus)
			auth.POST("/refresh-token", authHandler.RefreshToken)
			auth.POST("/refresh", authHandler.RefreshToken) // Alias for mobile compatibility
			// Protected routes (require JWT authentication)
			protected := auth.Group("")
			protected.Use(middleware.AuthMiddleware(jwtService))
			{
				protected.POST("/logout", authHandler.Logout)
			}
		}

		// Admin Authentication routes (separate from regular user auth)
		logger.Info("🔐 Registering Admin Authentication routes...")
		adminAuth := v1.Group("/admin/auth")
		{
			// Public routes
			logger.Info("  ✅ POST /api/v1/admin/auth/login")
			adminAuth.POST("/login", adminAuthHandler.Login)
			logger.Info("  ✅ POST /api/v1/admin/auth/refresh")
			adminAuth.POST("/refresh", adminAuthHandler.RefreshToken)
			logger.Info("  ✅ POST /api/v1/admin/auth/logout")
			adminAuth.POST("/logout", adminAuthHandler.Logout)

			// Protected routes (require admin JWT authentication)
			adminProtected := adminAuth.Group("")
			adminProtected.Use(middleware.AuthMiddleware(jwtService))
			{
				logger.Info("  ✅ GET /api/v1/admin/auth/profile")
				adminProtected.GET("/profile", adminAuthHandler.GetProfile)
				logger.Info("  ✅ POST /api/v1/admin/auth/change-password")
				adminProtected.POST("/change-password", adminAuthHandler.ChangePassword)
				logger.Info("  ✅ POST /api/v1/admin/auth/create")
				adminProtected.POST("/create", adminAuthHandler.CreateAdmin)
				logger.Info("  ✅ GET /api/v1/admin/auth/list")
				adminProtected.GET("/list", adminAuthHandler.ListAdmins)
			}
		}
		logger.Info("🔐 Admin Authentication routes registered successfully")

		// Lounge OTP tracking routes (admin only)
		logger.Info("🔐 Registering Lounge OTP tracking routes...")
		otpMaster := v1.Group("/admin/otp-master")
		otpMaster.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/admin/otp-master")
			otpMaster.POST("", authHandler.RecordOTPMaster)
			logger.Info("  ✅ GET /api/v1/admin/otp-master")
			otpMaster.GET("", authHandler.GetOTPMasterRecords)
		}
		logger.Info("🔐 Lounge OTP tracking routes registered successfully")

		// Bus Seat Layout routes (admin only)
		logger.Info("🚌 Registering Bus Seat Layout routes...")
		busSeatLayout := v1.Group("/admin/seat-layouts")
		busSeatLayout.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/admin/seat-layouts")
			busSeatLayout.POST("", busSeatLayoutHandler.CreateTemplate)
			logger.Info("  ✅ GET /api/v1/admin/seat-layouts")
			busSeatLayout.GET("", busSeatLayoutHandler.ListTemplates)
			logger.Info("  ✅ GET /api/v1/admin/seat-layouts/:id")
			busSeatLayout.GET("/:id", busSeatLayoutHandler.GetTemplate)
			logger.Info("  ✅ PUT /api/v1/admin/seat-layouts/:id")
			busSeatLayout.PUT("/:id", busSeatLayoutHandler.UpdateTemplate)
			logger.Info("  ✅ DELETE /api/v1/admin/seat-layouts/:id")
			busSeatLayout.DELETE("/:id", busSeatLayoutHandler.DeleteTemplate)
		}
		logger.Info("🚌 Bus Seat Layout routes registered successfully")

		// User routes (protected)
		user := v1.Group("/user")
		user.Use(middleware.AuthMiddleware(jwtService))
		{
			user.GET("/profile", authHandler.GetProfile)
			user.PUT("/profile", authHandler.UpdateProfile)
			user.POST("/complete-basic-profile", authHandler.CompleteBasicProfile) // Simple first_name + last_name for passengers
		}

		// Staff routes
		staff := v1.Group("/staff")
		{
			// Public routes (no authentication required)
			staff.POST("/check-registration", staffHandler.CheckRegistration)
			staff.POST("/register", staffHandler.RegisterStaff)
			staff.GET("/bus-owners/search", staffHandler.SearchBusOwners)

			// Protected routes (require JWT authentication)
			staffProtected := staff.Group("")
			staffProtected.Use(middleware.AuthMiddleware(jwtService))
			{
				staffProtected.GET("/profile", staffHandler.GetProfile)
				staffProtected.PUT("/profile", staffHandler.UpdateProfile)
				staffProtected.GET("/my-trips", staffHandler.GetMyTrips)

				// Active Trip routes (Start Trip / End Trip / Location tracking)
				logger.Info("🚌 Registering Active Trip routes...")
				staffProtected.GET("/trips/my-active", activeTripHandler.GetMyActiveTrip)
				staffProtected.POST("/trips/start", activeTripHandler.StartTrip)
				staffProtected.PUT("/trips/:id/location", activeTripHandler.UpdateLocation)
				staffProtected.POST("/trips/:id/end", activeTripHandler.EndTrip)
				staffProtected.GET("/trips/:id/active", activeTripHandler.GetActiveTrip)
				staffProtected.PUT("/trips/:id/passengers", activeTripHandler.UpdatePassengerCount)
				staffProtected.GET("/trips/:id/bookings", staffBookingHandler.GetTripBookings)
				logger.Info("✓ Active Trip routes registered")
			}
		}

		// Bus Owner routes (all protected)
		busOwner := v1.Group("/bus-owner")
		busOwner.Use(middleware.AuthMiddleware(jwtService))
		{
			// Profile endpoints (no verification needed - for registration flow)
			busOwner.GET("/profile", busOwnerHandler.GetProfile)
			busOwner.GET("/profile-status", busOwnerHandler.CheckProfileStatus)
			busOwner.POST("/complete-onboarding", busOwnerHandler.CompleteOnboarding)
			busOwner.GET("/staff", busOwnerHandler.GetStaff) // Get all staff (no verification needed)

			// Staff management (requires verification)
			busOwner.POST("/staff", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerHandler.AddStaff)           // Add driver or conductor
			busOwner.POST("/staff/verify", busOwnerHandler.VerifyStaff)                                                      // Verify if staff can be added (no verification needed)
			busOwner.POST("/staff/link", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerHandler.LinkStaff)     // Link verified staff to bus owner
			busOwner.POST("/staff/unlink", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerHandler.UnlinkStaff) // Remove staff from bus owner
		}

		// Bus Owner Routes (custom route configurations)
		busOwnerRoutes := v1.Group("/bus-owner-routes")
		busOwnerRoutes.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			busOwnerRoutes.GET("", busOwnerRouteHandler.GetRoutes)
			busOwnerRoutes.GET("/:id", busOwnerRouteHandler.GetRouteByID)
			busOwnerRoutes.GET("/by-master-route/:master_route_id", busOwnerRouteHandler.GetRoutesByMasterRoute)

			// Write endpoints (requires verification)
			busOwnerRoutes.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerRouteHandler.CreateRoute)
			busOwnerRoutes.PUT("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerRouteHandler.UpdateRoute)
			busOwnerRoutes.DELETE("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), busOwnerRouteHandler.DeleteRoute)
		}

		// Lounge Owner routes (all protected)
		logger.Info("🏢 Registering Lounge Owner routes...")
		loungeOwner := v1.Group("/lounge-owner")
		{
			// public lounge owner endpoints (for staff registering and lounge selection parts )
			loungeOwner.GET("/approved", loungeOwnerHandler.GetApprovedLoungeOwners)
			loungeOwner.GET("/approved/by-district/:district_id", loungeOwnerHandler.GetApprovedLoungeOwnersByDistrict)
			loungeOwner.GET("/approved/grouped-by-district", loungeOwnerHandler.GetApprovedLoungeOwnersByDsitrict)
			loungeOwner.GET("/:owner_id/lounges", loungeOwnerHandler.GetLoungesByOwnerID)
			loungeOwner.GET("/:owner_id/lounges/by-district/:district_id", loungeOwnerHandler.GetLoungesByOwnerAndDistrict)

			// Protected routes (require JWT authentication)
			loungeOwner.Use(middleware.AuthMiddleware(jwtService))
			{
				// Registration endpoints (no verification needed - for registration flow)
				logger.Info("  ✅ POST /api/v1/lounge-owner/register/business-info")
				loungeOwner.POST("/register/business-info", loungeOwnerHandler.SaveBusinessAndManagerInfo)
				logger.Info("  ✅ POST /api/v1/lounge-owner/register/upload-manager-nic")
				loungeOwner.POST("/register/upload-manager-nic", loungeOwnerHandler.UploadManagerNIC)
				logger.Info("  ✅ POST /api/v1/lounge-owner/register/add-lounge (requires approval)")
				loungeOwner.POST("/register/add-lounge", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeHandler.AddLounge)
				logger.Info("  ✅ GET /api/v1/lounge-owner/registration/progress")
				loungeOwner.GET("/registration/progress", loungeOwnerHandler.GetRegistrationProgress)

				// Profile endpoints
				logger.Info("  ✅ GET /api/v1/lounge-owner/profile")
				loungeOwner.GET("/profile", loungeOwnerHandler.GetProfile)
				logger.Info("  ✅ PUT /api/v1/lounge-owner/profile/update")
				loungeOwner.PUT("/profile/update", loungeOwnerHandler.UpdateProfile)

				// Lounge owner bank details & links
				logger.Info("  ✅ POST /api/v1/lounge-owner/bank-details")
				loungeOwner.POST("/bank-details", loungeOwnerBankHandler.CreateBankDetails)
				logger.Info("  ✅ GET /api/v1/lounge-owner/bank-details/:id")
				loungeOwner.GET("/bank-details/:id", loungeOwnerBankHandler.GetBankDetails)
				logger.Info("  ✅ PUT /api/v1/lounge-owner/bank-details/:id")
				loungeOwner.PUT("/bank-details/:id", loungeOwnerBankHandler.UpdateBankDetails)
				logger.Info("  ✅ DELETE /api/v1/lounge-owner/bank-details/:id")
				loungeOwner.DELETE("/bank-details/:id", loungeOwnerBankHandler.DeleteBankDetails)

				logger.Info("  ✅ POST /api/v1/lounge-owner/bank-links")
				loungeOwner.POST("/bank-links", loungeOwnerBankHandler.CreateBankLink)
				logger.Info("  ✅ GET /api/v1/lounge-owner/bank-links")
				loungeOwner.GET("/bank-links", loungeOwnerBankHandler.ListBankLinks)
				logger.Info("  ✅ DELETE /api/v1/lounge-owner/bank-links/:id")
				loungeOwner.DELETE("/bank-links/:id", loungeOwnerBankHandler.DeleteBankLink)
			}

		}
		logger.Info("🏢 Lounge Owner routes registered successfully")

		// Image upload routes (Cloudinary-backed)
		logger.Info("📸 Registering image upload routes...")
		uploads := v1.Group("/uploads")
		uploads.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/uploads/lounge-photos/:lounge_id")
			uploads.POST("/lounge-photos/:lounge_id", storageHandler.UploadLoungePhoto)
			logger.Info("  ✅ POST /api/v1/uploads/lounge-products/:lounge_id/image")
			uploads.POST("/lounge-products/:lounge_id/image", storageHandler.UploadProductImage)
			logger.Info("  ✅ POST /api/v1/uploads/lounge-owner/:user_id/nic/:side")
			uploads.POST("/lounge-owner/:user_id/nic/:side", storageHandler.UploadManagerNICImage)
			logger.Info("  ✅ POST /api/v1/uploads/lounge-special-packages/:lounge_id/image")
			uploads.POST("/lounge-special-packages/:lounge_id/image", storageHandler.UploadSpecialPackageImage)
			logger.Info("  ✅ POST /api/v1/uploads/image/delete")
			uploads.POST("/image/delete", storageHandler.DeleteImage)
		}
		logger.Info("📸 Image upload routes registered successfully")

		// Lounge Staff routes (protected)
		logger.Info("🏨 Registering Lounge Staff routes...")
		loungeStaff := v1.Group("/lounge-staff")
		loungeStaff.Use(middleware.AuthMiddleware(jwtService))
		{
			// Profile endpoints
			logger.Info("  ✅ GET /api/v1/lounge-staff/profile")
			loungeStaff.GET("/profile", loungeStaffHandler.GetProfile)
			logger.Info("  ✅ PUT /api/v1/lounge-staff/profile/update")
			loungeStaff.PUT("/profile/update", loungeStaffHandler.UpdateProfile)

			// Booking endpoints
			logger.Info("  ✅ GET /api/v1/lounge-staff/bookings")
			loungeStaff.GET("/bookings", loungeStaffHandler.GetLoungeBookingsForStaff)
			logger.Info("  ✅ GET /api/v1/lounge-staff/bookings/reference/:reference")
			loungeStaff.GET("/bookings/reference/:reference", loungeStaffHandler.GetBookingByReference)
		}
		logger.Info("🏨 Lounge Staff routes registered successfully")

		// Lounge routes (protected)
		logger.Info("🏨 Registering Lounge routes...")
		lounges := v1.Group("/lounges")
		{
			// Public routes (no authentication)
			logger.Info("  ✅ GET /api/v1/lounges/active (public)")
			lounges.GET("/active", loungeHandler.GetAllActiveLounges)
			logger.Info("  ✅ GET /api/v1/lounges/states (public)")
			lounges.GET("/states", loungeHandler.GetDistinctStates)
			logger.Info("  ✅ GET /api/v1/lounges/by-stop/:stopId (public)")
			lounges.GET("/by-stop/:stopId", loungeHandler.GetLoungesByStop)
			logger.Info("  ✅ GET /api/v1/lounges/by-route/:routeId (public)")
			lounges.GET("/by-route/:routeId", loungeHandler.GetLoungesByRoute)
			logger.Info("  ✅ GET /api/v1/lounges/near-stop/:routeId/:stopId (public)")
			lounges.GET("/near-stop/:routeId/:stopId", loungeHandler.GetLoungesNearStop)

			// Protected routes (require JWT authentication)
			loungesProtected := lounges.Group("")
			loungesProtected.Use(middleware.AuthMiddleware(jwtService))
			{
				// lounge management for a specific lounge
				logger.Info("  ✅ GET /api/v1/lounges/my-lounges (read-only, no approval needed)")
				loungesProtected.GET("/my-lounges", loungeHandler.GetMyLounges)
				logger.Info("  ✅ GET /api/v1/lounges/:id (read-only, no approval needed)")
				loungesProtected.GET("/:id", loungeHandler.GetLoungeByID)
				// Write operations require approval
				logger.Info("  ✅ POST /api/v1/lounges (create new lounge, requires approval)")
				loungesProtected.POST("", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeHandler.AddLounge)
				logger.Info("  ✅ PUT /api/v1/lounges/:id (requires approval)")
				loungesProtected.PUT("/:id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeHandler.UpdateLounge)
				logger.Info("  ✅ DELETE /api/v1/lounges/:id (requires approval)")
				loungesProtected.DELETE("/:id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeHandler.DeleteLounge)

				// Staff management for specific lounge (requires approval)
				//there is a route missmatch in staff related functions will fix it soon
				// logger.Info("  ✅ GET /api/v1/lounges/:id/staff (read-only, no approval needed)")
				// loungesProtected.GET("/:id/staff", loungeStaffHandler.GetStaffByLounge)//This endpont will most probabbly removed (i kept it for now to backward compatability)
				logger.Info("  ✅ POST /api/v1/lounges/:id/staff/direct-add (owner can directly add staff) (requires approval)")
				loungesProtected.POST("/:id/staff/direct-add", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeStaffHandler.AddStaffToLoungeDirectByOwner)
				// Permission management moved to users.roles array - removed permission_type field
				logger.Info("  ✅ PUT /api/v1/lounges/:id/staff/:staff_id/status (requires approval)")
				loungesProtected.PUT("/:id/staff/:staff_id/status", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeStaffHandler.UpdateStaffStatus)
				logger.Info("  ✅ DELETE /api/v1/lounges/:id/staff/:staff_id (requires approval)")
				loungesProtected.DELETE("/:id/staff/:staff_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeStaffHandler.RemoveStaff)

				// driver manamegemnt for specific lounge (using the :id to match with the other lounge routes)
				logger.Info(" ✅ GET /api/v1/lounges/:id/drivers - get drivers in lounge (read-only, no approval needed)")
				loungesProtected.GET("/:id/drivers", loungeDriverHandler.GetDriversByLounge)
				logger.Info(" ✅ POST /api/v1/lounges/drivers - add drivers to lounge (requires approval)")
				loungesProtected.POST("/drivers", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeDriverHandler.AddDriver)
				logger.Info(" ✅ PUT /api/v1/lounges/:id/drivers/:driver_id - update drivers in lounge (requires approval)")
				loungesProtected.PUT("/:id/drivers/:driver_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeDriverHandler.UpdateDriver)
				logger.Info(" ✅ DELETE /api/v1/lounges/:id/drivers/:driver_id - delete drivers in lounge (requires approval)")
				loungesProtected.DELETE("/:id/drivers/:driver_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeDriverHandler.DeleteDriver)

				// transport location management for specific lounge(:id = lounge_id)
				logger.Info(" ✅ GET /api/v1/lounges/:id/transport-locations - get transport locations in lounge (read-only, no approval needed)")
				loungesProtected.GET("/:id/transport-locations", loungeTransportLocationHandler.GetLoungeTransportLocationsByLoungeID)
				logger.Info(" ✅ POST /api/v1/lounges/transport-locations - add transport location to lounge (requires approval)")
				loungesProtected.POST("/transport-locations", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeTransportLocationHandler.AddLoungeTransportLocation)
				logger.Info(" ✅ PUT /api/v1/lounges/:id/transport-locations/:location_id - update transport locations in lounge (requires approval)")
				loungesProtected.PUT("/:id/transport-locations/:location_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeTransportLocationHandler.UpdateLoungeTransportLocationByID)
				logger.Info(" ✅ DELETE /api/v1/lounges/:id/transport-locations/:location_id - delete transport locations in lounge (requires approval)")
				loungesProtected.DELETE("/:id/transport-locations/:location_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeTransportLocationHandler.DeleteLoungeTransportLocationByID)

				// transport location price management for specific lounge
				logger.Info(" ✅ GET /api/v1/lounges/:id/transport-locations/:location_id/prices - get transport location prices in lounge (read-only, no approval needed)")
				loungesProtected.GET("/:id/transport-locations/:location_id/prices", loungeTransportLocationPriceHandler.GetLoungeTransportLocationPrices)
				logger.Info(" ✅ POST /api/v1/lounges/:id/transport-locations/:location_id/prices - set transport location prices in lounge (requires approval)")
				loungesProtected.POST("/:id/transport-locations/:location_id/prices", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeTransportLocationPriceHandler.SetLoungeTransportLocationPrices)

				// lounge staff approval management
				logger.Info(" ✅ PUT /api/v1/lounges/:id/staff/:staff_id/approval - set approval status for staff (requires approval)")
				loungesProtected.PUT("/:id/staff/:staff_id/approval", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeStaffHandler.ApproveStaff)
				logger.Info("✅ GET /api/v1/lounges/:id/staff - get staff with filtering (read-only, no approval needed)")
				loungesProtected.GET("/:id/staff", loungeStaffHandler.GetStaffByLoungeWithApprovalFilter)

			}
		}
		logger.Info("� Lounge routes registered successfully")

		// ============================================================================
		// LOUNGE BOOKING & MARKETPLACE ROUTES
		// ============================================================================
		logger.Info("🏨 Registering Lounge Booking routes...")

		// Lounge Marketplace - Categories (public)
		loungeMarketplace := v1.Group("/lounge-marketplace")
		{
			logger.Info("  ✅ GET /api/v1/lounge-marketplace/categories (public)")
			loungeMarketplace.GET("/categories", loungeBookingHandler.GetCategories)
		}

		// Lounge Products - Add to existing lounges group (protected)
		loungesProtectedProducts := v1.Group("/lounges")
		loungesProtectedProducts.Use(middleware.AuthMiddleware(jwtService))
		{
			// Products for a lounge (anyone can view, owner can manage)
			logger.Info("  ✅ GET /api/v1/lounges/:id/products (read-only, no approval needed)")
			loungesProtectedProducts.GET("/:id/products", loungeBookingHandler.GetLoungeProducts)
			logger.Info("  ✅ POST /api/v1/lounges/:id/products (requires approval)")
			loungesProtectedProducts.POST("/:id/products", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeBookingHandler.CreateProduct)
			logger.Info("  ✅ PUT /api/v1/lounges/:id/products/:product_id (requires approval)")
			loungesProtectedProducts.PUT("/:id/products/:product_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeBookingHandler.UpdateProduct)
			logger.Info("  ✅ DELETE /api/v1/lounges/:id/products/:product_id (requires approval)")
			loungesProtectedProducts.DELETE("/:id/products/:product_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeBookingHandler.DeleteProduct)

			// Special Packages for a lounge
			logger.Info("  ✅ GET /api/v1/lounges/:id/special-packages (read-only, no approval needed)")
			loungesProtectedProducts.GET("/:id/special-packages", loungeSpecialPackageHandler.GetSpecialPackages)
			logger.Info("  ✅ POST /api/v1/lounges/:id/special-packages (requires approval)")
			loungesProtectedProducts.POST("/:id/special-packages", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeSpecialPackageHandler.CreateSpecialPackage)
			logger.Info("  ✅ PUT /api/v1/lounges/:id/special-packages/:package_id (requires approval)")
			loungesProtectedProducts.PUT("/:id/special-packages/:package_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeSpecialPackageHandler.UpdateSpecialPackage)
			logger.Info("  ✅ DELETE /api/v1/lounges/:id/special-packages/:package_id (requires approval)")
			loungesProtectedProducts.DELETE("/:id/special-packages/:package_id", middleware.RequireApprovedLoungeOwner(loungeOwnerRepository), loungeSpecialPackageHandler.DeleteSpecialPackage)

			// Bookings for a lounge (owner/staff view - read-only, no approval needed)
			logger.Info("  ✅ GET /api/v1/lounges/:id/bookings (owner/staff, read-only)")
			loungesProtectedProducts.GET("/:id/bookings", loungeBookingHandler.GetLoungeBookingsForOwner)
			logger.Info("  ✅ GET /api/v1/lounges/:id/bookings-with-orders (owner/staff, read-only)")
			loungesProtectedProducts.GET("/:id/bookings-with-orders", loungeBookingHandler.GetLoungeBookingsWithOrdersForOwner)
			logger.Info("  ✅ GET /api/v1/lounges/:id/bookings/today (owner/staff, read-only)")
			loungesProtectedProducts.GET("/:id/bookings/today", loungeBookingHandler.GetTodaysBookings)

		}

		// Lounge Bookings - Passenger endpoints
		loungeBookings := v1.Group("/lounge-bookings")
		loungeBookings.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/lounge-bookings - Create lounge booking")
			loungeBookings.POST("", loungeBookingHandler.CreateLoungeBooking)
			logger.Info("  ✅ GET /api/v1/lounge-bookings - Get my lounge bookings")
			loungeBookings.GET("", loungeBookingHandler.GetMyLoungeBookings)
			logger.Info("  ✅ GET /api/v1/lounge-bookings/upcoming - Get upcoming bookings")
			loungeBookings.GET("/upcoming", loungeBookingHandler.GetUpcomingLoungeBookings)
			logger.Info("  ✅ GET /api/v1/lounge-bookings/:id - Get booking by ID")
			loungeBookings.GET("/:id", loungeBookingHandler.GetLoungeBookingByID)
			logger.Info("  ✅ GET /api/v1/lounge-bookings/reference/:reference - Get by reference")
			loungeBookings.GET("/reference/:reference", loungeBookingHandler.GetLoungeBookingByReference)
			logger.Info("  ✅ GET /api/v1/lounge-bookings/qr/:qr_code_data - Get by QR code data")
			loungeBookings.GET("/qr/:qr_code_data", loungeBookingHandler.GetLoungeBookingByQRCode)
			logger.Info("  ✅ POST /api/v1/lounge-bookings/:id/check-in-out - Toggle check-in/check-out")
			loungeBookings.POST("/:id/check-in-out", loungeBookingHandler.ToggleBookingCheckInOut)
			logger.Info("  ✅ POST /api/v1/lounge-bookings/:id/cancel - Cancel booking")
			loungeBookings.POST("/:id/cancel", loungeBookingHandler.CancelLoungeBooking)

			// Staff operations
			logger.Info("  ✅ POST /api/v1/lounge-bookings/:id/check-in - Check in guest")
			loungeBookings.POST("/:id/check-in", loungeBookingHandler.CheckInGuest)
			logger.Info("  ✅ POST /api/v1/lounge-bookings/:id/complete - Complete booking")
			loungeBookings.POST("/:id/complete", loungeBookingHandler.CompleteLoungeBooking)

			// Orders for a booking
			logger.Info("  ✅ GET /api/v1/lounge-bookings/:id/orders - Get booking orders")
			loungeBookings.GET("/:id/orders", loungeBookingHandler.GetBookingOrders)
			logger.Info("  ✅ GET /api/v1/lounge-bookings/:id/with-orders - Get booking with orders")
			loungeBookings.GET("/:id/with-orders", loungeBookingHandler.GetLoungeBookingWithOrders)
		}

		// Lounge Orders - In-lounge ordering
		loungeOrders := v1.Group("/lounge-orders")
		loungeOrders.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/lounge-orders - Create in-lounge order")
			loungeOrders.POST("", loungeBookingHandler.CreateLoungeOrder)
			logger.Info("  ✅ PUT /api/v1/lounge-orders/:id/status - Update order status")
			loungeOrders.PUT("/:id/status", loungeBookingHandler.UpdateOrderStatus)
		}
		logger.Info("🏨 Lounge Booking routes registered successfully")

		// Lounge Booking Driver Assignments
		logger.Info("Registering Lounge Booking Driver Assignment routes...")
		driverAssignments := v1.Group("/lounge-booking-driver-assignments")
		driverAssignments.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/lounge-booking-driver-assignments - Create assignment")
			driverAssignments.POST("", loungeBookingDriverAssignmentHandler.CreateAssignment)
			logger.Info("  ✅ GET /api/v1/lounge-booking-driver-assignments/:id - Get assignment by ID")
			driverAssignments.GET("/:id", loungeBookingDriverAssignmentHandler.GetAssignmentByID)
			logger.Info("  ✅ PUT /api/v1/lounge-booking-driver-assignments/:id - Update assignment")
			driverAssignments.PUT("/:id", loungeBookingDriverAssignmentHandler.UpdateAssignment)
			logger.Info("  ✅ DELETE /api/v1/lounge-booking-driver-assignments/:id - Delete assignment")
			driverAssignments.DELETE("/:id", loungeBookingDriverAssignmentHandler.DeleteAssignment)
			logger.Info("  ✅ POST /api/v1/lounge-booking-driver-assignments/:id/cancel - Cancel assignment")
			driverAssignments.POST("/:id/cancel", loungeBookingDriverAssignmentHandler.CancelAssignment)
			logger.Info("  ✅ POST /api/v1/lounge-booking-driver-assignments/:id/complete - Complete assignment")
			driverAssignments.POST("/:id/complete", loungeBookingDriverAssignmentHandler.CompleteAssignment)
		}

		// Assignments by booking
		logger.Info("  ✅ GET /api/v1/lounge-bookings/:id/driver-assignments - Get assignments for booking")
		v1.GET("/lounge-bookings/:id/driver-assignments", loungeBookingDriverAssignmentHandler.GetAssignmentsByBooking)
		logger.Info("  ✅ GET /api/v1/lounge-bookings/:id/assigned-driver - Get active assigned driver for booking")
		v1.GET("/lounge-bookings/:id/assigned-driver", loungeBookingDriverAssignmentHandler.GetAssignedDriverByBooking)
		logger.Info("  ✅ GET /api/v1/lounge-booking-driver-assignments/check/:booking_id - Check driver assignment")
		v1.GET("/lounge-booking-driver-assignments/check/:booking_id", loungeBookingDriverAssignmentHandler.CheckDriverAssignment)

		// Assignments by driver
		logger.Info("  ✅ GET /api/v1/drivers/:driver_id/assignments - Get assignments for driver")
		v1.GET("/drivers/:driver_id/assignments", loungeBookingDriverAssignmentHandler.GetAssignmentsByDriver)

		// Assignments by lounge (protected)
		loungeAssignments := v1.Group("/lounges")
		loungeAssignments.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ GET /api/v1/lounges/:id/driver-assignments - Get assignments for lounge")
			loungeAssignments.GET("/:id/driver-assignments", loungeBookingDriverAssignmentHandler.GetAssignmentsByLounge)
		}
		logger.Info("🚗 Lounge Booking Driver Assignment routes registered successfully")

		// Staff profile routes (for lounge staff members)
		logger.Info("👤 Registering Staff profile routes...")
		staffProfile := v1.Group("/staff")
		staffProfile.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ GET /api/v1/staff/my-profile")
			staffProfile.GET("/my-profile", loungeStaffHandler.GetMyStaffProfile)
		}
		logger.Info("👤 Staff profile routes registered successfully")

		// Permit routes (all protected)
		permits := v1.Group("/permits")
		permits.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			permits.GET("", permitHandler.GetAllPermits)
			permits.GET("/valid", permitHandler.GetValidPermits)
			permits.GET("/:id", permitHandler.GetPermitByID)
			permits.GET("/:id/route-details", permitHandler.GetRouteDetails)

			// Write endpoints (requires verification)
			permits.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), permitHandler.CreatePermit)
			permits.PUT("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), permitHandler.UpdatePermit)
			permits.DELETE("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), permitHandler.DeletePermit)
		}

		// Master Routes (all protected - for dropdown selection)
		masterRoutes := v1.Group("/master-routes")
		masterRoutes.Use(middleware.AuthMiddleware(jwtService))
		{
			masterRoutes.GET("", masterRouteHandler.ListMasterRoutes)
			masterRoutes.GET("/:id", masterRouteHandler.GetMasterRouteByID)
		}

		// Districts (public lookup endpoints)
		districts := v1.Group("/districts")
		{
			districts.GET("", districtHandler.ListDistricts)
			districts.GET("/:id", districtHandler.GetDistrictByID)
		}

		// Lounge owner districts (create + fetch by district)
		loungeOwnerDistricts := v1.Group("/lounge-owner-districts")
		{
			loungeOwnerDistricts.POST("", loungeOwnerDistrictHandler.Create)
			loungeOwnerDistricts.POST("/check-exists", loungeOwnerDistrictHandler.CheckExists)
			loungeOwnerDistricts.GET("/by-district/:district_id", loungeOwnerDistrictHandler.GetByDistrict)
		}

		// Bus routes (all protected)
		buses := v1.Group("/buses")
		buses.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			buses.GET("", busHandler.GetAllBuses)
			buses.GET("/:id", busHandler.GetBusByID)
			buses.GET("/status/:status", busHandler.GetBusesByStatus)

			// Write endpoints (requires verification)
			buses.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), busHandler.CreateBus)
			buses.PUT("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), busHandler.UpdateBus)
			buses.DELETE("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), busHandler.DeleteBus)
		}

		// Trip Schedule routes (all protected - bus owners only)
		tripSchedules := v1.Group("/trip-schedules")
		tripSchedules.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			tripSchedules.GET("", tripScheduleHandler.GetAllSchedules)
			tripSchedules.GET("/:id", tripScheduleHandler.GetScheduleByID)

			// Write endpoints (requires verification)
			tripSchedules.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), tripScheduleHandler.CreateSchedule)
			tripSchedules.PUT("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), tripScheduleHandler.UpdateSchedule)
			tripSchedules.DELETE("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), tripScheduleHandler.DeleteSchedule)
			tripSchedules.POST("/:id/deactivate", middleware.RequireVerifiedBusOwner(ownerRepository), tripScheduleHandler.DeactivateSchedule)
		}

		// Timetable routes (new timetable system - all protected)
		timetables := v1.Group("/timetables")
		timetables.Use(middleware.AuthMiddleware(jwtService))
		{
			// Write endpoints (requires verification)
			timetables.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), tripScheduleHandler.CreateTimetable)
		}

		// Special Trip routes (one-time trips, not from timetable - all protected)
		specialTrips := v1.Group("/special-trips")
		specialTrips.Use(middleware.AuthMiddleware(jwtService))
		{
			// Write endpoints (requires verification)
			specialTrips.POST("", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.CreateSpecialTrip)
		}

		// Scheduled Trip routes (all protected - bus owners only)
		scheduledTrips := v1.Group("/scheduled-trips")
		scheduledTrips.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			scheduledTrips.GET("", scheduledTripHandler.GetTripsByDateRange)
			scheduledTrips.GET("/:id", scheduledTripHandler.GetTripByID)

			// Write endpoints (requires verification)
			scheduledTrips.PATCH("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.UpdateTrip)
			scheduledTrips.POST("/:id/cancel", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.CancelTrip)

			// NEW: Publish/Unpublish endpoints (requires verification)
			scheduledTrips.PUT("/:id/publish", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.PublishTrip)
			scheduledTrips.PUT("/:id/unpublish", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.UnpublishTrip)
			scheduledTrips.POST("/bulk-publish", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.BulkPublishTrips)
			scheduledTrips.POST("/bulk-unpublish", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.BulkUnpublishTrips)

			// NEW: Assign staff and permit (requires verification)
			scheduledTrips.PATCH("/:id/assign", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.AssignStaffAndPermit)
			// NEW: Assign seat layout (requires verification)
			scheduledTrips.PATCH("/:id/assign-seat-layout", middleware.RequireVerifiedBusOwner(ownerRepository), scheduledTripHandler.AssignSeatLayout)

			// ============================================================================
			// TRIP SEATS ROUTES (Seat management for scheduled trips)
			// ============================================================================
			// Read endpoints (no verification needed)
			scheduledTrips.GET("/:id/seats", tripSeatHandler.GetTripSeats)
			scheduledTrips.GET("/:id/seats/summary", tripSeatHandler.GetTripSeatSummary)
			scheduledTrips.GET("/:id/route-stops", tripSeatHandler.GetTripRouteStops)

			// Write endpoints (requires verification)
			scheduledTrips.POST("/:id/seats/create", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.CreateTripSeats)
			scheduledTrips.POST("/:id/seats/block", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.BlockSeats)
			scheduledTrips.POST("/:id/seats/unblock", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.UnblockSeats)
			scheduledTrips.PUT("/:id/seats/price", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.UpdateSeatPrices)

			// ============================================================================
			// MANUAL BOOKINGS ROUTES (Phone/Agent/Walk-in bookings)
			// ============================================================================
			// Read endpoints (no verification needed)
			scheduledTrips.GET("/:id/manual-bookings", tripSeatHandler.GetManualBookings)

			// Write endpoints (requires verification)
			scheduledTrips.POST("/:id/manual-bookings", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.CreateManualBooking)
		}

		// Manual Bookings standalone routes (for operations on existing bookings)
		logger.Info("📋 Registering Manual Booking routes...")
		manualBookings := v1.Group("/manual-bookings")
		manualBookings.Use(middleware.AuthMiddleware(jwtService))
		{
			// Read endpoints (no verification needed)
			logger.Info("  ✅ GET /api/v1/manual-bookings/:id")
			manualBookings.GET("/:id", tripSeatHandler.GetManualBooking)
			logger.Info("  ✅ GET /api/v1/manual-bookings/reference/:ref")
			manualBookings.GET("/reference/:ref", tripSeatHandler.GetManualBookingByReference)
			logger.Info("  ✅ GET /api/v1/manual-bookings/search")
			manualBookings.GET("/search", tripSeatHandler.SearchManualBookingsByPhone)

			// Write endpoints (requires verification)
			logger.Info("  ✅ PUT /api/v1/manual-bookings/:id/payment (requires verification)")
			manualBookings.PUT("/:id/payment", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.UpdateManualBookingPayment)
			logger.Info("  ✅ PUT /api/v1/manual-bookings/:id/status (requires verification)")
			manualBookings.PUT("/:id/status", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.UpdateManualBookingStatus)
			logger.Info("  ✅ DELETE /api/v1/manual-bookings/:id (requires verification)")
			manualBookings.DELETE("/:id", middleware.RequireVerifiedBusOwner(ownerRepository), tripSeatHandler.CancelManualBooking)
		}
		logger.Info("📋 Manual Booking routes registered successfully")

		// ============================================================================
		// APP BOOKINGS ROUTES (Passenger app bookings)
		// ============================================================================
		logger.Info("📱 Registering App Booking routes...")
		appBookings := v1.Group("/bookings")
		appBookings.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/bookings - Create new booking")
			appBookings.POST("", appBookingHandler.CreateBooking)
			logger.Info("  ✅ GET /api/v1/bookings - Get my bookings")
			appBookings.GET("", appBookingHandler.GetMyBookings)
			logger.Info("  ✅ GET /api/v1/bookings/upcoming - Get upcoming bookings")
			appBookings.GET("/upcoming", appBookingHandler.GetUpcomingBookings)
			logger.Info("  ✅ GET /api/v1/bookings/:id - Get booking by ID")
			appBookings.GET("/:id", appBookingHandler.GetBookingByID)
			logger.Info("  ✅ GET /api/v1/bookings/reference/:reference - Get booking by reference")
			appBookings.GET("/reference/:reference", appBookingHandler.GetBookingByReference)
			logger.Info("  ✅ POST /api/v1/bookings/:id/confirm-payment - Confirm payment")
			appBookings.POST("/:id/confirm-payment", appBookingHandler.ConfirmPayment)
			logger.Info("  ✅ POST /api/v1/bookings/:id/cancel - Cancel booking")
			appBookings.POST("/:id/cancel", appBookingHandler.CancelBooking)
			logger.Info("  ✅ GET /api/v1/bookings/:id/qr - Get booking QR code")
			appBookings.GET("/:id/qr", appBookingHandler.GetBookingQR)
		}
		logger.Info("📱 App Booking routes registered successfully")

		// ============================================================================
		// BOOKING ORCHESTRATION ROUTES (Intent → Payment → Confirm)
		// ============================================================================
		logger.Info("🎯 Registering Booking Orchestration routes...")

		// Booking Intent routes (protected - requires auth)
		bookingOrchestration := v1.Group("/booking")
		bookingOrchestration.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/booking/intent - Create booking intent")
			bookingOrchestration.POST("/intent", bookingOrchestratorHandler.CreateIntent)

			logger.Info("  ✅ GET /api/v1/booking/intents - Get my intents")
			bookingOrchestration.GET("/intents", bookingOrchestratorHandler.GetMyIntents)

			logger.Info("  ✅ GET /api/v1/booking/intent/:intent_id - Get intent status")
			bookingOrchestration.GET("/intent/:intent_id", bookingOrchestratorHandler.GetIntentStatus)

			logger.Info("  ✅ POST /api/v1/booking/intent/:intent_id/initiate-payment - Initiate payment")
			bookingOrchestration.POST("/intent/:intent_id/initiate-payment", bookingOrchestratorHandler.InitiatePayment)

			logger.Info("  ✅ POST /api/v1/booking/intent/:intent_id/cancel - Cancel intent")
			bookingOrchestration.POST("/intent/:intent_id/cancel", bookingOrchestratorHandler.CancelIntent)

			logger.Info("  ✅ PATCH /api/v1/booking/intent/:intent_id/add-lounge - Add lounge to intent")
			bookingOrchestration.PATCH("/intent/:intent_id/add-lounge", bookingOrchestratorHandler.AddLoungeToIntent)

			logger.Info("  ✅ POST /api/v1/booking/confirm - Confirm booking after payment")
			bookingOrchestration.POST("/confirm", bookingOrchestratorHandler.ConfirmBooking)
		}

		// Payment webhook (no auth - called by payment gateway)
		logger.Info("  ✅ POST /api/v1/payments/webhook - Payment gateway webhook")
		v1.POST("/payments/webhook", bookingOrchestratorHandler.PaymentWebhook)

		// Payment return URL (no auth - browser redirect from payment gateway)
		logger.Info("  ✅ GET /api/v1/payments/return - Payment return page")
		v1.GET("/payments/return", bookingOrchestratorHandler.PaymentReturn)

		logger.Info("🎯 Booking Orchestration routes registered successfully")

		// ============================================================================
		// STAFF BOOKING ROUTES (Conductor/Driver operations)
		// ============================================================================
		logger.Info("👨‍✈️ Registering Staff Booking routes...")
		staffBookings := v1.Group("/staff/bookings")
		staffBookings.Use(middleware.AuthMiddleware(jwtService))
		{
			logger.Info("  ✅ POST /api/v1/staff/bookings/verify - Verify booking by QR")
			staffBookings.POST("/verify", staffBookingHandler.VerifyBookingByQR)
			logger.Info("  ✅ POST /api/v1/staff/bookings/check-in - Check-in passenger")
			staffBookings.POST("/check-in", staffBookingHandler.CheckInPassenger)
			logger.Info("  ✅ POST /api/v1/staff/bookings/board - Board passenger")
			staffBookings.POST("/board", staffBookingHandler.BoardPassenger)
			logger.Info("  ✅ POST /api/v1/staff/bookings/no-show - Mark no-show")
			staffBookings.POST("/no-show", staffBookingHandler.MarkNoShow)
		}
		logger.Info("👨‍✈️ Staff Booking routes registered successfully")

		// Permit-specific trip routes
		permits.GET("/:id/trip-schedules", tripScheduleHandler.GetSchedulesByPermit)
		permits.GET("/:id/scheduled-trips", scheduledTripHandler.GetTripsByPermit)

		// Public bookable trips (no auth required)
		v1.GET("/bookable-trips", scheduledTripHandler.GetBookableTrips)

		// ============================================================================
		// SEARCH ROUTES (Phase 1 MVP - Trip Discovery)
		// ============================================================================
		logger.Info("🔍 Registering Search routes...")

		// Public search routes (no authentication required)
		search := v1.Group("/search")
		{
			logger.Info("  ✅ POST /api/v1/search - Main search endpoint")
			search.POST("", searchHandler.SearchTrips)

			logger.Info("  ✅ GET /api/v1/search/popular - Popular routes")
			search.GET("/popular", searchHandler.GetPopularRoutes)

			logger.Info("  ✅ GET /api/v1/search/autocomplete - Stop suggestions")
			search.GET("/autocomplete", searchHandler.GetStopAutocomplete)

			logger.Info("  ✅ GET /api/v1/search/health - Health check")
			search.GET("/health", searchHandler.HealthCheck)
		}
		logger.Info("🔍 Search routes registered successfully")

		// System Settings routes (protected)
		systemSettings := v1.Group("/system-settings")
		systemSettings.Use(middleware.AuthMiddleware(jwtService))
		{
			systemSettings.GET("", systemSettingHandler.GetAllSettings)
			systemSettings.GET("/:key", systemSettingHandler.GetSettingByKey)
			systemSettings.PUT("/:key", systemSettingHandler.UpdateSetting)
		}

		// Admin routes
		admin := v1.Group("/admin")
		// TODO: Add admin auth middleware
		{
			// Lounge Owner approval (TODO: Implement)
			admin.GET("/lounge-owners/pending", adminHandler.GetPendingLoungeOwners)
			admin.GET("/lounge-owners/:id", adminHandler.GetLoungeOwnerDetails)
			admin.POST("/lounge-owners/:id/approve", adminHandler.ApproveLoungeOwner)
			admin.POST("/lounge-owners/:id/reject", adminHandler.RejectLoungeOwner)

			// Lounge approval (TODO: Implement)
			admin.GET("/lounges/pending", adminHandler.GetPendingLounges)
			admin.POST("/lounges/:id/approve", adminHandler.ApproveLounge)
			admin.POST("/lounges/:id/reject", adminHandler.RejectLounge)

			// Bus Owner approval (TODO: Implement later)
			admin.GET("/bus-owners/pending", adminHandler.GetPendingBusOwners)
			admin.POST("/bus-owners/:id/approve", adminHandler.ApproveBusOwner)

			// Staff approval (TODO: Implement later)
			admin.GET("/staff/pending", adminHandler.GetPendingStaff)
			admin.POST("/staff/:id/approve", adminHandler.ApproveStaff)

			// Dashboard stats (TODO: Implement)
			admin.GET("/dashboard/stats", adminHandler.GetDashboardStats)

			// Search analytics
			admin.GET("/search/analytics", searchHandler.GetSearchAnalytics)
		}
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Infof("Server starting on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited successfully")
}

// requestLogger middleware for logging HTTP requests
func requestLogger(logger *logrus.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// Log incoming request
		logger.WithFields(logrus.Fields{
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
			"ip":         c.ClientIP(),
			"user_agent": c.Request.UserAgent(),
		}).Info("Incoming request")

		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		// Build log entry with basic fields
		fields := logrus.Fields{
			"status":     c.Writer.Status(),
			"method":     c.Request.Method,
			"path":       path,
			"query":      query,
			"ip":         c.ClientIP(),
			"latency_ms": latency.Milliseconds(),
			"user_agent": c.Request.UserAgent(),
		}

		// Add authorization header presence (not the actual token for security)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			fields["has_auth"] = true
			if len(authHeader) > 20 {
				fields["auth_type"] = authHeader[:20] + "..." // Show Bearer prefix only
			}
		} else {
			fields["has_auth"] = false
		}

		// Add user context if available
		if userID, exists := c.Get("user_id"); exists {
			fields["user_id"] = userID
		}
		if phone, exists := c.Get("phone"); exists {
			fields["phone"] = phone
		}
		if roles, exists := c.Get("roles"); exists {
			fields["roles"] = roles
		}

		entry := logger.WithFields(fields)

		// Log errors with more details
		if len(c.Errors) > 0 {
			// Add error details
			for i, err := range c.Errors {
				entry = entry.WithField(fmt.Sprintf("error_%d", i), err.Error())
				if err.Meta != nil {
					entry = entry.WithField(fmt.Sprintf("error_%d_meta", i), err.Meta)
				}
			}
			entry.Error("Request failed with errors")
		} else {
			// Log based on status code
			status := c.Writer.Status()
			if status >= 500 {
				entry.Error("Request completed with server error")
			} else if status >= 400 {
				entry.Warn("Request completed with client error")
			} else {
				entry.Info("Request completed successfully")
			}
		}
	}
}

// healthCheckHandler returns a health check endpoint
func healthCheckHandler(db database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check database connection
		dbStatus := "healthy"
		if err := db.Ping(); err != nil {
			dbStatus = "unhealthy"
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unhealthy",
				"database": dbStatus,
				"error":    err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"database":  dbStatus,
			"version":   version,
			"timestamp": time.Now().Unix(),
		})
	}
}

// debugHeadersHandler shows all request headers for debugging IP issues
func debugHeadersHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Collect all headers
		headers := make(map[string]string)
		for name, values := range c.Request.Header {
			headers[name] = values[0] // Take first value
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "Debug information for IP detection",
			"headers": headers,
			"ip_detection": gin.H{
				"gin_clientip":      c.ClientIP(),
				"remote_addr":       c.Request.RemoteAddr,
				"x_real_ip":         c.Request.Header.Get("X-Real-IP"),
				"x_forwarded_for":   c.Request.Header.Get("X-Forwarded-For"),
				"x_forwarded_host":  c.Request.Header.Get("X-Forwarded-Host"),
				"x_forwarded_proto": c.Request.Header.Get("X-Forwarded-Proto"),
			},
			"user_agent": c.Request.UserAgent(),
			"timestamp":  time.Now().Unix(),
		})
	}
}
