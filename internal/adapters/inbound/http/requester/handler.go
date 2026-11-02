package requesterhandler

import (
	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/gin-gonic/gin"
)

type RequesterHandler struct {
	create        ports.CreateRequesterUseCase
	getByID       ports.GetRequesterUseCase
	getByDocument ports.GetRequesterByDocumentUseCase
	listAll       ports.ListRequestersUseCase
	update        ports.UpdateRequesterUseCase
	delete        ports.DeleteRequesterUseCase
	listVehicles  ports.ListVehiclesByRequesterUseCase
}

func NewRequesterHandler(
	create ports.CreateRequesterUseCase,
	getByID ports.GetRequesterUseCase,
	getByDocument ports.GetRequesterByDocumentUseCase,
	listAll ports.ListRequestersUseCase,
	update ports.UpdateRequesterUseCase,
	delete ports.DeleteRequesterUseCase,
	listVehicles ports.ListVehiclesByRequesterUseCase,
) *RequesterHandler {
	return &RequesterHandler{
		create:        create,
		getByID:       getByID,
		getByDocument: getByDocument,
		listAll:       listAll,
		update:        update,
		delete:        delete,
		listVehicles:  listVehicles,
	}
}

func (h *RequesterHandler) SetupRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/requesters", httputil.UUIDParams("id"))
	g.POST("", h.Create)
	g.GET("", h.FindAll)
	g.GET("/document/:document", h.FindByDocument)
	g.GET("/:id", h.FindByID)
	g.GET("/:id/vehicles", h.ListVehicles)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
}
