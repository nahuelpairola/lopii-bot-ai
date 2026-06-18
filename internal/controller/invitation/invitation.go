package invitation

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/middleware"
)

const botUsername = "tu_bot" // TODO: mover a config

type InvitationRepository interface {
	Create(createdBy uint64) (*invitation.Invitation, error)
}

type controller struct {
	repo InvitationRepository
}

func NewController(repo InvitationRepository) *controller {
	return &controller{repo: repo}
}

func (c *controller) RegisterRoutes(engine *gin.Engine) {
	// TODO: hardcoded adminID=1 hasta que exista login real
	engine.POST("/invitations", middleware.RequireAdmin(1), c.Create)
}

// Create genera una invitación nueva. Asume que el middleware de sesión
// ya validó que el usuario es admin y dejó su ID en el contexto.
func (c *controller) Create(ctx *gin.Context) {
	adminID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	inv, err := c.repo.Create(adminID.(uint64))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "could not create invitation"})
		return
	}

	link := "https://t.me/" + botUsername + "?start=" + inv.Code

	ctx.JSON(http.StatusOK, gin.H{
		"code":       inv.Code,
		"link":       link,
		"expires_at": inv.ExpiresAt,
	})
}
