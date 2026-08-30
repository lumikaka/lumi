package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"lumi/internal/agent"
	"lumi/internal/appstore"
	"lumi/internal/config"
	"lumi/internal/directoryopener"
	"lumi/internal/directorypicker"
	"lumi/internal/files"
	"lumi/internal/httpapi"
	"lumi/internal/imagegen"
	"lumi/internal/jobqueue"
	"lumi/internal/llm"
	"lumi/internal/modelsettings"
	"lumi/internal/project"
	"lumi/internal/projectcreation"
	"lumi/internal/provider"
	"lumi/internal/realtime"
	"lumi/internal/sitesettings"
	"lumi/internal/story"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Application struct {
	*echo.Echo
	realtimeHub     *realtime.Hub
	projects        *project.Manager
	providers       *provider.Service
	agentService    *agent.Service
	lifecycleCancel context.CancelFunc
	lifecycleDone   chan struct{}
	closeOnce       sync.Once
	closeErr        error
}

func New(cfg config.Config, appStore *appstore.Store, projects *project.Manager) (*Application, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpapi.ErrorHandler
	e.Use(middleware.RequestID())
	e.Use(requestLogger(slog.Default()))
	e.Use(middleware.Recover())
	e.Use(desktopAuthentication(cfg.DesktopAuth))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: realtime.AllowedOrigins(cfg.FrontendURL),
		AllowMethods: []string{echo.GET, echo.HEAD, echo.OPTIONS, echo.POST, echo.PUT, echo.PATCH, echo.DELETE},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))
	e.Use(trustedWriteOrigin(cfg.FrontendURL))

	realtimeHub := realtime.NewHub(realtime.ProjectTopics{AcquirePresence: projects.AcquirePresence})
	modelClient := llm.NewOpenAICompatibleClient(nil)
	imageClient := imagegen.NewOpenAICompatibleClient(nil)
	providerService := provider.NewService(appStore, sitesettings.NewOSMasterKeyStore())
	taskManager := jobqueue.NewManager(providerService, modelClient, realtimeHub).WithImageClient(imageClient)
	agentService := agent.NewService(projects, providerService, modelClient, taskManager, realtimeHub).WithImageClient(imageClient)
	projectCreationService := projectcreation.NewService(appStore, projects, agentService, realtimeHub)
	taskManager.WithAgentService(agentService)
	api := e.Group("/api/v1")
	api.Use(projectRequestLease(projects))
	api.GET("/health", httpapi.NewHealthHandler(appStore.DB()).Show)
	if cfg.DesktopAuth != nil {
		api.POST("/desktop-sessions", newDesktopSessionHandler(cfg.DesktopAuth).Create)
	}
	api.GET("/ws", realtime.NewWebSocketHandler(realtimeHub, cfg.FrontendURL).Serve)
	directorySelectionHandler := httpapi.NewDirectorySelectionHandler(directorypicker.NewNative())
	directoryOpeningHandler := httpapi.NewDirectoryOpeningHandler(directoryopener.NewNative())
	projectHandler := httpapi.NewProjectHandler(projects)
	projectCreationSessionHandler := httpapi.NewProjectCreationSessionHandler(projectCreationService)
	projectDefaultsHandler := httpapi.NewProjectDefaultsHandler()
	imagePreflightHandler := httpapi.NewImageGenerationPreflightHandler(providerService)
	openProjectHandler := httpapi.NewOpenProjectHandler(projects)
	recentProjectHandler := httpapi.NewRecentProjectHandler(projects)
	storyHandler := httpapi.NewStoryHandler(projects, realtimeHub)
	llmLogHandler := httpapi.NewLLMLogHandler(projects)
	projects.WithOpenHook(story.ReconcileOnOpen)
	projects.WithOpenHook(files.ReconcileOnOpen)
	projects.WithRuntime(taskManager).WithOpenHook(taskManager.StartProject)
	projects.WithOpenHook(agentService.ReconcileOnOpen)
	projects.WithLifecycleHook(func(event project.LifecycleEvent) {
		realtimeHub.Broadcast(realtime.SystemTopic, realtime.OpenProjectChanged, openProjectChangedPayload(event))
	})
	providerHandler := httpapi.NewProviderHandler(providerService, modelClient, realtimeHub)
	siteSettingsHandler := httpapi.NewSiteSettingsHandler(providerService, realtimeHub)
	taskHandler := httpapi.NewTaskHandler(taskManager)
	productionHandler := httpapi.NewProductionHandler(projects, taskManager, realtimeHub)
	modelResolver := modelsettings.NewResolver(providerService)
	modelSettingsHandler := httpapi.NewModelSettingsHandler(projects, modelResolver, realtimeHub)
	projectImagePreflightHandler := httpapi.NewProjectImageGenerationPreflightHandler(projects, modelResolver)
	filesHandler := httpapi.NewFilesHandler(projects, realtimeHub, taskManager)
	agentHandler := httpapi.NewAgentHandler(agentService)
	projectSetupHandler := httpapi.NewProjectSetupHandler(projects, realtimeHub)
	api.GET("/providers", providerHandler.Index)
	api.GET("/providers/active", providerHandler.Active)
	api.GET("/providers/:provider_uuid", providerHandler.Show)
	api.POST("/providers/:provider_uuid/connection-checks", providerHandler.Check)
	api.GET("/site-settings", siteSettingsHandler.Index)
	api.PATCH("/site-settings", siteSettingsHandler.Update)
	api.POST("/site-settings/resets", siteSettingsHandler.Reset)
	api.POST("/directory-selections", directorySelectionHandler.Create)
	api.POST("/directory-openings", directoryOpeningHandler.Create)
	api.GET("/project-defaults", projectDefaultsHandler.Show)
	api.POST("/projects", projectHandler.Create)
	api.POST("/project-creation-sessions", projectCreationSessionHandler.Create)
	api.GET("/project-creation-sessions/:session_uuid", projectCreationSessionHandler.Show)
	api.POST("/project-creation-sessions/:session_uuid/retries", projectCreationSessionHandler.Retry)
	api.POST("/project-creation-sessions/:session_uuid/references/:reference_uuid/uploads", projectCreationSessionHandler.UploadReference)
	api.POST("/image-generation-preflights", imagePreflightHandler.Create)
	api.GET("/open-projects", openProjectHandler.Index)
	api.POST("/open-projects", openProjectHandler.Create)
	api.PUT("/open-projects/:project_uuid", openProjectHandler.Update)
	api.DELETE("/open-projects/:project_uuid", openProjectHandler.Delete)
	api.GET("/recent-projects", recentProjectHandler.Index)
	api.PATCH("/recent-projects/:project_uuid", recentProjectHandler.Update)
	api.DELETE("/recent-projects/:project_uuid", recentProjectHandler.Delete)
	api.GET("/projects/:project_uuid", storyHandler.ShowProject)
	api.GET("/projects/:project_uuid/project-setup", projectSetupHandler.Show)
	api.PATCH("/projects/:project_uuid/project-setup", projectSetupHandler.Update)
	api.POST("/projects/:project_uuid/project-setup-finalizations", projectSetupHandler.Finalize)
	api.POST("/projects/:project_uuid/image-generation-preflights", projectImagePreflightHandler.Create)
	api.GET("/projects/:project_uuid/llm-logs", llmLogHandler.Index)
	api.GET("/projects/:project_uuid/llm-logs/:log_uuid", llmLogHandler.Show)
	api.PATCH("/projects/:project_uuid", storyHandler.UpdateProject)
	api.GET("/projects/:project_uuid/model-settings", modelSettingsHandler.Show)
	api.PATCH("/projects/:project_uuid/model-settings", modelSettingsHandler.Update)
	api.GET("/projects/:project_uuid/chapters", storyHandler.ListChapters)
	api.POST("/projects/:project_uuid/chapters", storyHandler.CreateChapter)
	api.PUT("/projects/:project_uuid/chapter-order", storyHandler.ReorderChapters)
	api.POST("/projects/:project_uuid/chapter-imports", storyHandler.ImportChapters)
	api.DELETE("/projects/:project_uuid/chapters/trash", storyHandler.EmptyChapterTrash)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid", storyHandler.ShowChapter)
	api.PATCH("/projects/:project_uuid/chapters/:chapter_uuid", storyHandler.UpdateChapter)
	api.DELETE("/projects/:project_uuid/chapters/:chapter_uuid", storyHandler.TrashChapter)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/restorations", storyHandler.RestoreChapter)
	api.DELETE("/projects/:project_uuid/chapters/:chapter_uuid/permanent", storyHandler.PermanentlyDeleteChapter)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/current-story", storyHandler.CurrentStory)
	api.PUT("/projects/:project_uuid/chapters/:chapter_uuid/current-story", storyHandler.UpdateCurrentStory)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/stories", storyHandler.ListChapterStories)
	api.GET("/projects/:project_uuid/story-profile", storyHandler.ShowStoryProfile)
	api.PUT("/projects/:project_uuid/story-profile", storyHandler.UpdateStoryProfile)
	api.GET("/projects/:project_uuid/story-profile/versions", storyHandler.ListStoryProfiles)
	api.POST("/projects/:project_uuid/story-profile/versions/:version_uuid/restorations", storyHandler.RestoreStoryProfile)
	api.POST("/projects/:project_uuid/story-profile/imports", storyHandler.ImportExternalStoryMD)
	api.POST("/projects/:project_uuid/story-profile/projection", storyHandler.RegenerateStoryMD)
	api.GET("/projects/:project_uuid/prompts", storyHandler.ListPromptCatalog)
	api.PATCH("/projects/:project_uuid/prompt-groups/:prompt_group", storyHandler.UpdatePromptGroup)
	api.GET("/projects/:project_uuid/prompt-versions", storyHandler.ListPromptVersions)
	api.POST("/projects/:project_uuid/prompt-versions", storyHandler.CreatePromptVersion)
	api.POST("/projects/:project_uuid/prompt-versions/:version_uuid/restorations", storyHandler.RestorePromptVersion)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/generations", taskHandler.CreateChapterGeneration)
	api.POST("/projects/:project_uuid/story-profile/generations", taskHandler.CreateStoryProfileGeneration)
	api.POST("/projects/:project_uuid/story-profile/reconstructions", taskHandler.CreateStoryProfileFromChapters)
	api.POST("/projects/:project_uuid/chapter-batches", taskHandler.CreateChapterBatchPlan)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-storyboard-generations", taskHandler.CreateComicStoryboardGeneration)
	api.GET("/projects/:project_uuid/tasks", taskHandler.Index)
	api.GET("/projects/:project_uuid/tasks/:task_uuid", taskHandler.Show)
	api.GET("/projects/:project_uuid/tasks/:task_uuid/events", taskHandler.Events)
	api.POST("/projects/:project_uuid/tasks/:task_uuid/cancellations", taskHandler.Cancel)
	api.POST("/projects/:project_uuid/tasks/:task_uuid/retries", taskHandler.Retry)
	api.GET("/projects/:project_uuid/chat_threads", agentHandler.ListThreads)
	api.POST("/projects/:project_uuid/chat_threads", agentHandler.CreateThread)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid", agentHandler.ShowThread)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/turns", agentHandler.ListTurns)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/turns", agentHandler.CreateTurn)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/items", agentHandler.ListItems)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/events", agentHandler.ListEvents)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/trajectory", agentHandler.ShowTrajectory)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups", agentHandler.ListFollowUps)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups", agentHandler.CreateFollowUp)
	api.PATCH("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid", agentHandler.UpdateFollowUp)
	api.PATCH("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid/position", agentHandler.MoveFollowUp)
	api.DELETE("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid", agentHandler.DeleteFollowUp)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/follow_ups/:follow_up_uuid/steerings", agentHandler.SteerFollowUp)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/steerings", agentHandler.Steer)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/cancellations", agentHandler.Abort)
	api.GET("/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests", agentHandler.ListUserInputRequests)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests/:request_uuid/responses", agentHandler.RespondUserInput)
	api.POST("/projects/:project_uuid/chat_threads/:thread_uuid/user_input_requests/:request_uuid/cancellations", agentHandler.CancelUserInput)
	api.GET("/projects/:project_uuid/workflows", agentHandler.ListWorkflows)
	api.POST("/projects/:project_uuid/workflows", agentHandler.CreateYoloWorkflow)
	api.GET("/projects/:project_uuid/workflows/:workflow_uuid", agentHandler.ShowWorkflow)
	api.GET("/projects/:project_uuid/workflows/:workflow_uuid/runs", agentHandler.ListWorkflowRuns)
	api.GET("/projects/:project_uuid/workflows/:workflow_uuid/events", agentHandler.ListWorkflowEvents)
	api.GET("/projects/:project_uuid/workflows/:workflow_uuid/llm-logs", agentHandler.ListWorkflowLLMLogs)
	api.POST("/projects/:project_uuid/workflows/:workflow_uuid/cancellations", agentHandler.CancelWorkflow)
	api.POST("/projects/:project_uuid/workflows/:workflow_uuid/retries", agentHandler.RetryWorkflow)
	api.POST("/projects/:project_uuid/workflows/:workflow_uuid/conflict-resolutions", taskHandler.ResolveComicStoryboardConflict)
	api.POST("/projects/:project_uuid/asset-uploads", filesHandler.CreateUpload)
	api.GET("/projects/:project_uuid/asset-uploads/:upload_uuid", filesHandler.ShowUpload)
	api.POST("/projects/:project_uuid/asset-uploads/:upload_uuid/completions", filesHandler.FinalizeUpload)
	api.DELETE("/projects/:project_uuid/asset-uploads/:upload_uuid", filesHandler.CancelUpload)
	api.GET("/projects/:project_uuid/assets", filesHandler.ListAssets)
	api.GET("/projects/:project_uuid/assets/:asset_uuid", filesHandler.ShowAsset)
	api.PATCH("/projects/:project_uuid/assets/:asset_uuid", filesHandler.UpdateAsset)
	api.DELETE("/projects/:project_uuid/assets/:asset_uuid", filesHandler.TrashAsset)
	api.POST("/projects/:project_uuid/assets/:asset_uuid/restorations", filesHandler.RestoreAsset)
	api.POST("/projects/:project_uuid/assets/:asset_uuid/thumbnails", filesHandler.CreateThumbnail)
	api.GET("/projects/:project_uuid/integrity-scans", filesHandler.ListScans)
	api.POST("/projects/:project_uuid/integrity-scans", filesHandler.CreateScan)
	api.GET("/projects/:project_uuid/integrity-scans/:scan_uuid", filesHandler.ShowScan)
	api.POST("/projects/:project_uuid/asset-reconciliations", filesHandler.Reconcile)
	api.POST("/projects/:project_uuid/asset-gc-plans", filesHandler.GCDryRun)
	api.POST("/projects/:project_uuid/asset-gc-plans/:plan_uuid/applications", filesHandler.GCApply)
	api.GET("/projects/:project_uuid/asset-maintenance-tasks", filesHandler.ListMaintenanceTasks)
	api.POST("/projects/:project_uuid/asset-maintenance-tasks", filesHandler.CreateMaintenanceTask)
	api.GET("/projects/:project_uuid/asset-maintenance-tasks/:task_uuid", filesHandler.ShowMaintenanceTask)
	api.GET("/projects/:project_uuid/asset-maintenance-tasks/:task_uuid/events", filesHandler.MaintenanceTaskEvents)
	api.POST("/projects/:project_uuid/asset-maintenance-tasks/:task_uuid/cancellations", filesHandler.CancelMaintenanceTask)
	api.GET("/projects/:project_uuid/premise", productionHandler.ShowPremise)
	api.PATCH("/projects/:project_uuid/premise", productionHandler.UpdatePremise)
	api.GET("/projects/:project_uuid/premise-sources", productionHandler.ListPremiseSources)
	api.POST("/projects/:project_uuid/premise-sources", productionHandler.CreatePremiseSource)
	api.PATCH("/projects/:project_uuid/premise-sources/:source_uuid", productionHandler.UpdatePremiseSource)
	api.POST("/projects/:project_uuid/premise-sources/:source_uuid/setting-generations", productionHandler.GenerateSettingImage)
	api.GET("/projects/:project_uuid/premise-setting-images", productionHandler.ListSettingImages)
	api.POST("/projects/:project_uuid/premise-setting-images", productionHandler.ImportSettingImage)
	api.POST("/projects/:project_uuid/premise-setting-images/:setting_image_uuid/selections", productionHandler.SelectSettingImage)
	api.POST("/projects/:project_uuid/premise-setting-images/:setting_image_uuid/breakdowns", productionHandler.BreakdownSettingImage)
	api.GET("/projects/:project_uuid/premise-assets", productionHandler.ListPremiseAssets)
	api.POST("/projects/:project_uuid/premise-assets", productionHandler.CreatePremiseAsset)
	api.DELETE("/projects/:project_uuid/premise-assets/trash", productionHandler.EmptyPremiseAssetTrash)
	api.GET("/projects/:project_uuid/premise-assets/:premise_asset_uuid", productionHandler.ShowPremiseAsset)
	api.PATCH("/projects/:project_uuid/premise-assets/:premise_asset_uuid", productionHandler.UpdatePremiseAsset)
	api.DELETE("/projects/:project_uuid/premise-assets/:premise_asset_uuid", productionHandler.TrashPremiseAsset)
	api.DELETE("/projects/:project_uuid/premise-assets/:premise_asset_uuid/permanent", productionHandler.PermanentlyDeletePremiseAsset)
	api.POST("/projects/:project_uuid/premise-assets/:premise_asset_uuid/restorations", productionHandler.RestorePremiseAsset)
	api.GET("/projects/:project_uuid/premise-assets/:premise_asset_uuid/variants", productionHandler.ListAssetVariants)
	api.POST("/projects/:project_uuid/premise-assets/:premise_asset_uuid/variants", productionHandler.CreateAssetVariant)
	api.POST("/projects/:project_uuid/premise-assets/:premise_asset_uuid/variants/:variant_uuid/selections", productionHandler.SelectAssetVariant)
	api.POST("/projects/:project_uuid/premise-assets/:premise_asset_uuid/generations", productionHandler.GeneratePremiseAssetVariant)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic", productionHandler.ShowComicState)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections", productionHandler.ListSections)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections", productionHandler.CreateSection)
	api.PUT("/projects/:project_uuid/chapters/:chapter_uuid/comic-section-order", productionHandler.ReorderSections)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid", productionHandler.ShowSection)
	api.PATCH("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid", productionHandler.UpdateSection)
	api.DELETE("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid", productionHandler.DeleteSection)
	api.PUT("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/premise-assets", productionHandler.SetSectionPremiseAssets)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/storyboard-variants", productionHandler.ListStoryboards)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/storyboard-variants", productionHandler.CreateStoryboard)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/storyboard-variants/:variant_uuid/selections", productionHandler.SelectStoryboard)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/images", productionHandler.ImportSectionImage)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/image-variants", productionHandler.ListImageVariants)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/image-variants/:variant_uuid/selections", productionHandler.SelectImageVariant)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-sections/:section_uuid/image-generations", productionHandler.GenerateSectionImage)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-image-generation-batches", productionHandler.GenerateChapterImagesBatch)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots", productionHandler.ListSnapshots)
	api.GET("/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots/:snapshot_uuid", productionHandler.ShowSnapshot)
	api.POST("/projects/:project_uuid/chapters/:chapter_uuid/comic-snapshots/:snapshot_uuid/restorations", productionHandler.RestoreSnapshot)
	api.GET("/projects/:project_uuid/comic-exports", productionHandler.ListExports)
	api.GET("/projects/:project_uuid/comic-exports/readiness", productionHandler.ExportReadiness)
	api.POST("/projects/:project_uuid/comic-exports", productionHandler.CreateExport)
	api.GET("/projects/:project_uuid/production-tasks", productionHandler.ListProductionTasks)
	api.GET("/projects/:project_uuid/production-tasks/:task_uuid", productionHandler.ShowProductionTask)
	api.GET("/projects/:project_uuid/production-tasks/:task_uuid/events", productionHandler.ProductionTaskEvents)
	api.POST("/projects/:project_uuid/production-tasks/:task_uuid/cancellations", productionHandler.CancelProductionTask)
	api.POST("/projects/:project_uuid/production-tasks/:task_uuid/retries", productionHandler.RetryProductionTask)
	configureAgentProjectAPIGateway(e, agentService)
	if err := projectCreationService.Reconcile(context.Background()); err != nil {
		return nil, err
	}
	e.GET("/media/projects/:project_uuid/assets/:asset_uuid/content", filesHandler.Content, projectRequestLease(projects))
	e.GET("/media/projects/:project_uuid/comic-exports/:export_uuid/content", productionHandler.ExportContent, projectRequestLease(projects))
	e.GET("/media/recent-projects/:project_uuid/cover", recentProjectHandler.Cover)
	lifecycleContext, lifecycleCancel := context.WithCancel(context.Background())
	lifecycleDone := make(chan struct{})
	lifecycle := newProjectLifecycleController(projects, projectIdleGrace)
	go func() {
		defer close(lifecycleDone)
		lifecycle.run(lifecycleContext, projectIdleCheckInterval)
	}()
	return &Application{
		Echo: e, realtimeHub: realtimeHub, projects: projects, providers: providerService, agentService: agentService,
		lifecycleCancel: lifecycleCancel, lifecycleDone: lifecycleDone,
	}, nil
}

func requestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		HandleError:      true,
		LogLatency:       true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogMethod:        true,
		LogURI:           true,
		LogRequestID:     true,
		LogUserAgent:     true,
		LogStatus:        true,
		LogError:         true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogValuesFunc: func(c echo.Context, values middleware.RequestLoggerValues) error {
			level := requestLogLevel(values.Status, values.Error)
			attributes := []slog.Attr{
				slog.String("method", values.Method),
				slog.String("uri", values.URI),
				slog.Int("status", values.Status),
				slog.Duration("latency", values.Latency),
				slog.String("host", values.Host),
				slog.String("bytes_in", values.ContentLength),
				slog.Int64("bytes_out", values.ResponseSize),
				slog.String("user_agent", values.UserAgent),
				slog.String("remote_ip", values.RemoteIP),
				slog.String("request_id", values.RequestID),
			}
			if values.Error != nil {
				attributes = append(attributes, slog.String("error", values.Error.Error()))
			}
			logger.LogAttrs(c.Request().Context(), level, "http request", attributes...)
			return nil
		},
	})
}

func requestLogLevel(status int, err error) slog.Level {
	if status < http.StatusBadRequest && err != nil {
		return slog.LevelError
	}
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}
	if status >= http.StatusBadRequest {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func openProjectChangedPayload(event project.LifecycleEvent) map[string]any {
	return map[string]any{"project_uuid": event.ProjectUUID, "open": event.Open}
}

// projectRequestLease keeps a project Store and Runtime alive for the full
// request, including handlers (such as task APIs) that route through the
// runtime registry rather than opening the Store themselves.
func projectRequestLease(projects *project.Manager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			if !strings.HasPrefix(path, "/api/v1/projects/:project_uuid") && !strings.HasPrefix(path, "/media/projects/:project_uuid") {
				return next(c)
			}
			var handlerErr error
			if err := projects.WithStore(c.Request().Context(), c.Param("project_uuid"), func(store *project.Store) error {
				if store.SetupStatus() == project.SetupStatusDraft && !draftProjectRequestAllowed(c.Request().Method, path) {
					handlerErr = httpapi.NewError(http.StatusConflict, project.CodeProjectSetupIncomplete, "项目设置尚未定稿", "请先在 ChatArea 中确认绘本规格。", nil)
					return nil
				}
				handlerErr = next(c)
				return nil
			}); err != nil {
				return httpapi.ProjectAPIError(err)
			}
			return handlerErr
		}
	}
}

func draftProjectRequestAllowed(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return true
	}
	if path == "/api/v1/projects/:project_uuid/project-setup" && method == http.MethodPatch {
		return true
	}
	if path == "/api/v1/projects/:project_uuid/project-setup-finalizations" && method == http.MethodPost {
		return true
	}
	return strings.HasPrefix(path, "/api/v1/projects/:project_uuid/chat_threads")
}

func trustedWriteOrigin(frontendURL string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			switch c.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				return next(c)
			}
			if !realtime.OriginAllowed(c.Request().Header.Get(echo.HeaderOrigin), frontendURL) {
				return httpapi.NewError(http.StatusForbidden, "untrusted_origin", "请求来源不受信任", "写请求只能由 Lumi 本地界面发起。", nil)
			}
			return next(c)
		}
	}
}

func (application *Application) RealtimeHub() *realtime.Hub {
	return application.realtimeHub
}

func (application *Application) Close() error {
	if application == nil {
		return nil
	}
	application.closeOnce.Do(func() {
		if application.lifecycleCancel != nil {
			application.lifecycleCancel()
		}
		if application.lifecycleDone != nil {
			<-application.lifecycleDone
		}
		var projectErr, realtimeErr error
		if application.projects != nil {
			projectErr = application.projects.Close()
		}
		if application.realtimeHub != nil {
			realtimeErr = application.realtimeHub.Close()
		}
		if application.providers != nil {
			application.providers.Close()
		}
		application.closeErr = errors.Join(projectErr, realtimeErr)
	})
	return application.closeErr
}

func (application *Application) Shutdown(ctx context.Context) error {
	httpErr := application.Echo.Shutdown(ctx)
	return errors.Join(httpErr, application.Close())
}
