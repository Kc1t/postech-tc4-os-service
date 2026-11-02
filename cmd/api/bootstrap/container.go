package bootstrap

import (
	"context"
	"log"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/config"
	inboundmsg "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/messaging"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/scheduler"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/jwt"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/postgresql"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/outbound/services"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/Kc1t/postech-tc4-os-service/pkg/awsx"
	"github.com/Kc1t/postech-tc4-os-service/pkg/hasher"
	"github.com/Kc1t/postech-tc4-os-service/pkg/httpclient"
	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
	"github.com/Kc1t/postech-tc4-os-service/pkg/messaging"
	"github.com/Kc1t/postech-tc4-os-service/pkg/observability"
	"github.com/Kc1t/postech-tc4-os-service/pkg/outbox"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/newrelic/go-agent/v3/newrelic"
	"gorm.io/gorm"

	authhandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/auth"
	"github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/internalapi"
	parthandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/part"
	requesterhandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/requester"
	servicehandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/service"
	serviceorderhandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/service_order"
	vehiclehandler "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http/vehicle"

	authuc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/auth"
	partuc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/part"
	requesteruc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/requester"
	sagauc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/saga"
	serviceuc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/service"
	serviceorderuc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/service_order"
	vehicleuc "github.com/Kc1t/postech-tc4-os-service/internal/application/usecase/vehicle"
)

type Container struct {
	Config *config.Config
	Logger *slog.Logger
	NR     *newrelic.Application
	db     *gorm.DB

	// Workers de mensageria (nil quando a mensageria está desligada)
	OutboxRelay  *outbox.Relay
	SagaConsumer *messaging.Consumer
	SagaTimeouts *scheduler.SagaTimeouts
	sqsClient    *sqs.Client
	snsClient    *sns.Client

	// Repositories
	UserRepo         ports.UserRepository          `container:"repository"`
	RefreshTokenRepo ports.RefreshTokenRepository  `container:"repository"`
	RequesterRepo    ports.RequesterRepository     `container:"repository"`
	VehicleRepo      ports.VehicleRepository       `container:"repository"`
	ServiceOrderRepo ports.ServiceOrderRepository  `container:"repository"`
	ServiceRepo      ports.ServiceRepository       `container:"repository"`
	PartRepo         ports.PartRepository          `container:"repository"`
	SagaRepo         ports.SagaRepository          `container:"repository"`
	HistoryRepo      ports.StatusHistoryRepository `container:"repository"`
	TxManager        ports.TxManager
	Messenger        ports.SagaMessenger

	// Use Cases — auth
	RegisterUseCase     ports.RegisterUseCase     `container:"usecase"`
	LoginUseCase        ports.LoginUseCase        `container:"usecase"`
	RefreshTokenUseCase ports.RefreshTokenUseCase `container:"usecase"`
	LogoutUseCase       ports.LogoutUseCase       `container:"usecase"`

	// Use Cases — requester
	CreateRequester        ports.CreateRequesterUseCase        `container:"usecase"`
	GetRequester           ports.GetRequesterUseCase           `container:"usecase"`
	GetRequesterByDocument ports.GetRequesterByDocumentUseCase `container:"usecase"`
	ListRequesters         ports.ListRequestersUseCase         `container:"usecase"`
	UpdateRequester        ports.UpdateRequesterUseCase        `container:"usecase"`
	DeleteRequester        ports.DeleteRequesterUseCase        `container:"usecase"`

	// Use Cases — vehicle
	CreateVehicle           ports.CreateVehicleUseCase           `container:"usecase"`
	GetVehicle              ports.GetVehicleUseCase              `container:"usecase"`
	ListVehicles            ports.ListVehiclesUseCase            `container:"usecase"`
	ListVehiclesByRequester ports.ListVehiclesByRequesterUseCase `container:"usecase"`
	UpdateVehicle           ports.UpdateVehicleUseCase           `container:"usecase"`
	DeleteVehicle           ports.DeleteVehicleUseCase           `container:"usecase"`

	// Use Cases — service order (saga)
	CreateServiceOrder           ports.CreateServiceOrderUseCase           `container:"usecase"`
	GetServiceOrder              ports.GetServiceOrderUseCase              `container:"usecase"`
	GetServiceOrderByCode        ports.GetServiceOrderByCodeUseCase        `container:"usecase"`
	ListServiceOrders            ports.ListServiceOrdersUseCase            `container:"usecase"`
	ListServiceOrdersByRequester ports.ListServiceOrdersByRequesterUseCase `container:"usecase"`
	ListServiceOrdersByDocument  ports.ListServiceOrdersByDocumentUseCase  `container:"usecase"`
	CancelServiceOrder           ports.CancelServiceOrderUseCase           `container:"usecase"`
	DeliverServiceOrder          ports.DeliverServiceOrderUseCase          `container:"usecase"`
	GetServiceOrderHistory       ports.GetServiceOrderHistoryUseCase       `container:"usecase"`
	GetServiceOrderDetails       ports.GetServiceOrderDetailsUseCase       `container:"usecase"`
	GetAverageExecutionTime      ports.GetAverageExecutionTimeUseCase      `container:"usecase"`
	HandleSagaReply              ports.HandleSagaReplyUseCase              `container:"usecase"`
	ExpireSagas                  ports.ExpireSagasUseCase                  `container:"usecase"`

	// Use Cases — service
	CreateService ports.CreateServiceUseCase `container:"usecase"`
	GetService    ports.GetServiceUseCase    `container:"usecase"`
	ListServices  ports.ListServicesUseCase  `container:"usecase"`
	UpdateService ports.UpdateServiceUseCase `container:"usecase"`
	DeleteService ports.DeleteServiceUseCase `container:"usecase"`

	// Use Cases — part
	CreatePart      ports.CreatePartUseCase      `container:"usecase"`
	GetPart         ports.GetPartUseCase         `container:"usecase"`
	ListParts       ports.ListPartsUseCase       `container:"usecase"`
	UpdatePart      ports.UpdatePartUseCase      `container:"usecase"`
	DeletePart      ports.DeletePartUseCase      `container:"usecase"`
	AdjustPartStock ports.AdjustPartStockUseCase `container:"usecase"`

	// Handlers
	AuthHandler         *authhandler.AuthHandler                 `container:"handler"`
	RequesterHandler    *requesterhandler.RequesterHandler       `container:"handler"`
	VehicleHandler      *vehiclehandler.VehicleHandler           `container:"handler"`
	ServiceOrderHandler *serviceorderhandler.ServiceOrderHandler `container:"handler"`
	ServiceHandler      *servicehandler.ServiceHandler           `container:"handler"`
	PartHandler         *parthandler.PartHandler                 `container:"handler"`
	InternalHandler     *internalapi.Handler                     `container:"handler"`
}

var (
	instance *Container
	once     sync.Once
)

func GetContainer() *Container {
	once.Do(func() {
		instance = &Container{}
		instance.initialize()
	})
	return instance
}

func (c *Container) initialize() {
	c.Config = config.Load()
	c.Logger = logger.New(c.Config.LogLevel).With("service", "os-service")
	slog.SetDefault(c.Logger)
	c.setupNewRelic()
	c.setupDatabase()
	c.setupMessagingClients()
	c.setupRepositories()
	c.setupUseCases()
	c.setupHandlers()
	c.setupWorkers()
	c.seedAdmin()
}

func (c *Container) setupNewRelic() {
	app, err := observability.NewRelic(c.Config.NewRelicAppName, c.Config.NewRelicLicenseKey)
	switch {
	case err != nil:
		c.Logger.Error("new relic desabilitado", "error", err)
	case app != nil:
		c.NR = app
		c.Logger.Info("new relic habilitado", "app_name", c.Config.NewRelicAppName)
	default:
		c.Logger.Info("new relic desabilitado: NEW_RELIC_LICENSE_KEY ausente")
	}
}

// DB expõe a conexão para o readiness probe.
func (c *Container) DB() *gorm.DB { return c.db }

func (c *Container) setupMessagingClients() {
	if !c.Config.MessagingEnabled() {
		c.Logger.Warn("mensageria desligada: defina SQS_SAGA_REPLIES_URL, SQS_BILLING_COMMANDS_URL, SQS_EXECUTION_COMMANDS_URL e SNS_OS_EVENTS_ARN")
		return
	}
	awsCfg, err := awsx.LoadConfig(context.Background())
	if err != nil {
		log.Fatalf("bootstrap: aws config: %v", err)
	}
	c.sqsClient = sqs.NewFromConfig(awsCfg)
	c.snsClient = sns.NewFromConfig(awsCfg)
}

func (c *Container) setupWorkers() {
	if c.sqsClient == nil {
		return
	}
	c.OutboxRelay = outbox.NewRelay(c.db, messaging.NewAWSPublisher(c.snsClient, c.sqsClient), c.NR, c.Logger).
		WithInterval(c.Config.OutboxPollInterval)
	c.SagaConsumer = messaging.NewConsumer(c.sqsClient, c.Config.SagaRepliesQueueURL, "os-saga-replies", c.NR, c.Logger)
	inboundmsg.RegisterSagaReplies(c.SagaConsumer, c.HandleSagaReply)
	c.SagaTimeouts = scheduler.NewSagaTimeouts(c.ExpireSagas, c.Config.SagaTimeoutCheck, c.NR, c.Logger)
}

func (c *Container) setupDatabase() {
	db, err := postgresql.Open(c.Config.PostgresDSN)
	if err != nil {
		log.Fatalf("bootstrap: failed to connect to postgresql: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := postgresql.Migrate(db); err != nil {
		log.Fatalf("bootstrap: failed to run migrations: %v", err)
	}

	c.db = db
	log.Printf("bootstrap: connected to postgresql and migrations applied")
}

func (c *Container) setupRepositories() {
	c.UserRepo = postgresql.NewUserRepository(c.db)
	c.RefreshTokenRepo = postgresql.NewRefreshTokenRepository(c.db)
	c.RequesterRepo = postgresql.NewRequesterRepository(c.db)
	c.VehicleRepo = postgresql.NewVehicleRepository(c.db)
	c.ServiceOrderRepo = postgresql.NewServiceOrderRepository(c.db)
	c.ServiceRepo = postgresql.NewServiceRepository(c.db)
	c.PartRepo = postgresql.NewPartRepository(c.db)
	c.SagaRepo = postgresql.NewSagaRepository(c.db)
	c.HistoryRepo = postgresql.NewStatusHistoryRepository(c.db)
	c.TxManager = postgresql.NewTxManager(c.db)
	c.Messenger = postgresql.NewOutboxMessenger(postgresql.Routes{
		BillingCommandsQueueURL:   c.Config.BillingCommandsQueueURL,
		ExecutionCommandsQueueURL: c.Config.ExecutionCommandsQueueURL,
		OSEventsTopicARN:          c.Config.OSEventsTopicARN,
	})
}

func (c *Container) setupUseCases() {
	// Servicos de infraestrutura (desacoplados via interface nos ports)
	tokenSvc := jwt.New(c.Config.JWTSecret, c.Config.AccessTokenExpMin, c.Config.RefreshTokenExpDays)
	pwdHasher := hasher.NewBcrypt(c.Config.BcryptCost)

	// Auth
	c.RegisterUseCase = authuc.NewRegister(c.UserRepo, pwdHasher)
	loginUC, err := authuc.NewLogin(
		c.UserRepo, c.RefreshTokenRepo, tokenSvc, pwdHasher,
		c.Config.MaxFailedLogins, c.Config.LoginLockMin,
	)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	c.LoginUseCase = loginUC
	c.RefreshTokenUseCase = authuc.NewRefresh(c.UserRepo, c.RefreshTokenRepo, tokenSvc)
	c.LogoutUseCase = authuc.NewLogout(c.RefreshTokenRepo)

	// Requester
	c.CreateRequester = requesteruc.NewCreateRequester(c.RequesterRepo)
	c.GetRequester = requesteruc.NewGetRequester(c.RequesterRepo)
	c.GetRequesterByDocument = requesteruc.NewGetRequesterByDocument(c.RequesterRepo)
	c.ListRequesters = requesteruc.NewListRequesters(c.RequesterRepo)
	c.UpdateRequester = requesteruc.NewUpdateRequester(c.RequesterRepo)
	c.DeleteRequester = requesteruc.NewDeleteRequester(c.RequesterRepo)

	// Vehicle
	c.CreateVehicle = vehicleuc.NewCreateVehicle(c.VehicleRepo, c.RequesterRepo)
	c.GetVehicle = vehicleuc.NewGetVehicle(c.VehicleRepo)
	c.ListVehicles = vehicleuc.NewListVehicles(c.VehicleRepo)
	c.ListVehiclesByRequester = vehicleuc.NewListVehiclesByRequester(c.VehicleRepo)
	c.UpdateVehicle = vehicleuc.NewUpdateVehicle(c.VehicleRepo)
	c.DeleteVehicle = vehicleuc.NewDeleteVehicle(c.VehicleRepo)

	// ServiceOrder + saga
	orchestrator := sagauc.NewOrchestrator(sagauc.Deps{
		Tx:         c.TxManager,
		Inbox:      postgresql.NewInbox(c.db, "os-saga-replies"),
		Sagas:      c.SagaRepo,
		Orders:     c.ServiceOrderRepo,
		History:    c.HistoryRepo,
		Requesters: c.RequesterRepo,
		Vehicles:   c.VehicleRepo,
		Services:   c.ServiceRepo,
		Parts:      c.PartRepo,
		Messenger:  c.Messenger,
	}, sagauc.Config{
		ApprovalTimeout: c.Config.SagaApprovalTimeout,
		PaymentTimeout:  c.Config.SagaPaymentTimeout,
		FaultInjection:  c.Config.FaultInjectionEnabled,
	})
	c.CreateServiceOrder = sagauc.NewCreateServiceOrder(orchestrator)
	c.CancelServiceOrder = sagauc.NewCancelServiceOrder(orchestrator)
	c.DeliverServiceOrder = sagauc.NewDeliverServiceOrder(orchestrator)
	c.HandleSagaReply = sagauc.NewHandleReply(orchestrator)
	c.ExpireSagas = sagauc.NewExpireSagas(orchestrator)

	upstream := httpclient.Config{Timeout: c.Config.UpstreamTimeout}
	billing := services.NewBillingClient(services.Config{
		Name: "billing-service", BaseURL: c.Config.BillingServiceURL, JWTSecret: c.Config.JWTSecret, HTTP: upstream,
	}, c.Logger)
	execution := services.NewExecutionClient(services.Config{
		Name: "execution-service", BaseURL: c.Config.ExecutionServiceURL, JWTSecret: c.Config.JWTSecret, HTTP: upstream,
	}, c.Logger)

	c.GetServiceOrder = serviceorderuc.NewGetServiceOrder(c.ServiceOrderRepo)
	c.GetServiceOrderByCode = serviceorderuc.NewGetServiceOrderByCode(c.ServiceOrderRepo, c.RequesterRepo)
	c.ListServiceOrders = serviceorderuc.NewListServiceOrders(c.ServiceOrderRepo)
	c.ListServiceOrdersByRequester = serviceorderuc.NewListServiceOrdersByRequester(c.ServiceOrderRepo)
	c.ListServiceOrdersByDocument = serviceorderuc.NewListServiceOrdersByDocument(c.ServiceOrderRepo, c.RequesterRepo)
	c.GetServiceOrderHistory = serviceorderuc.NewGetServiceOrderHistory(c.ServiceOrderRepo, c.HistoryRepo)
	c.GetServiceOrderDetails = serviceorderuc.NewGetServiceOrderDetails(
		c.ServiceOrderRepo, c.SagaRepo, c.HistoryRepo, billing, execution)
	c.GetAverageExecutionTime = serviceorderuc.NewGetAverageExecutionTime(c.ServiceOrderRepo)

	// Service
	c.CreateService = serviceuc.NewCreateService(c.ServiceRepo)
	c.GetService = serviceuc.NewGetService(c.ServiceRepo)
	c.ListServices = serviceuc.NewListServices(c.ServiceRepo)
	c.UpdateService = serviceuc.NewUpdateService(c.ServiceRepo)
	c.DeleteService = serviceuc.NewDeleteService(c.ServiceRepo, c.ServiceOrderRepo)

	// Part
	c.CreatePart = partuc.NewCreatePart(c.PartRepo)
	c.GetPart = partuc.NewGetPart(c.PartRepo)
	c.ListParts = partuc.NewListParts(c.PartRepo)
	c.UpdatePart = partuc.NewUpdatePart(c.PartRepo)
	c.DeletePart = partuc.NewDeletePart(c.PartRepo, c.ServiceOrderRepo)
	c.AdjustPartStock = partuc.NewAdjustPartStock(c.PartRepo)
}

func (c *Container) setupHandlers() {
	c.AuthHandler = authhandler.NewAuthHandler(
		c.RegisterUseCase, c.LoginUseCase, c.RefreshTokenUseCase,
		c.LogoutUseCase, c.Config.AccessTokenExpMin,
	)
	c.RequesterHandler = requesterhandler.NewRequesterHandler(
		c.CreateRequester, c.GetRequester, c.GetRequesterByDocument,
		c.ListRequesters, c.UpdateRequester, c.DeleteRequester,
		c.ListVehiclesByRequester,
	)
	c.VehicleHandler = vehiclehandler.NewVehicleHandler(
		c.CreateVehicle, c.GetVehicle, c.ListVehicles,
		c.UpdateVehicle, c.DeleteVehicle,
	)
	c.ServiceOrderHandler = serviceorderhandler.NewServiceOrderHandler(serviceorderhandler.Deps{
		Create:               c.CreateServiceOrder,
		GetByID:              c.GetServiceOrder,
		GetByCode:            c.GetServiceOrderByCode,
		ListAll:              c.ListServiceOrders,
		ListByDocument:       c.ListServiceOrdersByDocument,
		Cancel:               c.CancelServiceOrder,
		Deliver:              c.DeliverServiceOrder,
		History:              c.GetServiceOrderHistory,
		Details:              c.GetServiceOrderDetails,
		AverageExecutionTime: c.GetAverageExecutionTime,
	})
	c.InternalHandler = internalapi.NewHandler(c.Config.InternalAPIToken, c.GetRequesterByDocument)
	c.ServiceHandler = servicehandler.NewServiceHandler(
		c.CreateService, c.GetService, c.ListServices,
		c.UpdateService, c.DeleteService,
	)
	c.PartHandler = parthandler.NewPartHandler(
		c.CreatePart, c.GetPart, c.ListParts,
		c.UpdatePart, c.DeletePart, c.AdjustPartStock,
	)
}

func (c *Container) seedAdmin() {
	email := os.Getenv("ADMIN_EMAIL")
	password := os.Getenv("ADMIN_PASSWORD")

	// Em prod exige credenciais explicitas, em dev usa fallback
	if c.Config.AppEnv == config.EnvProduction && (email == "" || password == "") {
		log.Printf("bootstrap: skipping admin seed in prod (ADMIN_EMAIL and ADMIN_PASSWORD must be set)")
		return
	}
	if email == "" {
		email = "admin@workshop.com"
	}
	if password == "" {
		password = "admin123"
	}

	ctx := context.Background()
	_, err := c.UserRepo.FindByEmail(ctx, email)
	if err == nil {
		return
	}

	if err := c.RegisterUseCase.Execute(ctx, "Admin", email, password, entities.RoleAdmin); err != nil {
		log.Printf("bootstrap: failed to seed admin user: %v", err)
		return
	}
	c.Logger.Info("bootstrap: admin user seeded", "email", email)
}
