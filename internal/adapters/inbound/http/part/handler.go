package parthandler

import (
	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/Kc1t/postech-tc4-os-service/internal/ports"
	"github.com/gin-gonic/gin"
)

type PartHandler struct {
	create      ports.CreatePartUseCase
	getByID     ports.GetPartUseCase
	listAll     ports.ListPartsUseCase
	update      ports.UpdatePartUseCase
	delete      ports.DeletePartUseCase
	adjustStock ports.AdjustPartStockUseCase
}

func NewPartHandler(
	create ports.CreatePartUseCase,
	getByID ports.GetPartUseCase,
	listAll ports.ListPartsUseCase,
	update ports.UpdatePartUseCase,
	delete ports.DeletePartUseCase,
	adjustStock ports.AdjustPartStockUseCase,
) *PartHandler {
	return &PartHandler{
		create:      create,
		getByID:     getByID,
		listAll:     listAll,
		update:      update,
		delete:      delete,
		adjustStock: adjustStock,
	}
}

func (h *PartHandler) SetupRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/parts", httputil.UUIDParams("id"))
	g.POST("", h.Create)
	g.GET("", h.FindAll)
	g.GET("/:id", h.FindByID)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	g.PATCH("/:id/stock", h.AdjustStock)
}
