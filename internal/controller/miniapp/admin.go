package miniapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/invitation"
)

// Los tres estados son exactamente los que handleStart chequea al canjear un
// código (usada → vencida → válida). La vista no puede decir "pendiente" de
// algo que el bot va a rechazar.
const (
	stateUsed    = "usada"
	stateExpired = "expirada"
	statePending = "pendiente"

	// dateFormat es el orden que lee un argentino: día/mes/año.
	dateFormat = "02/01/2006"
)

func (c *controller) handleAdmin(ctx *gin.Context) {
	c.renderAdmin(ctx)
}

func (c *controller) handleCreateInvitation(ctx *gin.Context) {
	adminID := ctx.GetUint64(contextUserIDKey)
	if _, err := c.invitations.Create(adminID); err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	c.renderAdmin(ctx)
}

// renderAdmin es el cuerpo de los dos handlers: el POST contesta con la misma
// vista que el GET, así la nueva invitación es simplemente la primera fila
// (List ordena por created_at desc) y no hay ningún estado de "recién creada"
// que tenga que viajar.
func (c *controller) renderAdmin(ctx *gin.Context) {
	invs, err := c.invitations.List()
	if err != nil {
		ctx.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// El período no se usa para nada de lo que Admin muestra: viaja para que
	// @AppState emita los inputs que el tabbar levanta con hx-include. Sin él,
	// salir de Admin por cualquier pestaña manda la navegación sin p/pt/m/c y
	// periodFromQuery cae a los defaults en silencio.
	p := periodFromQuery(ctx, templates.SinglePeriodScope)

	data := templates.AdminData{Period: p, Rows: make([]templates.InvitationRow, 0, len(invs))}
	for _, inv := range invs {
		data.Rows = append(data.Rows, c.invitationRow(inv))
	}

	ctx.Status(http.StatusOK)
	templates.Admin(data).Render(ctx.Request.Context(), ctx.Writer)
}

// invitationRow deriva el estado en vez de guardarlo: el estado real vive en
// used_at y expires_at, y una columna aparte se desincroniza sola.
func (c *controller) invitationRow(inv invitation.Invitation) templates.InvitationRow {
	row := templates.InvitationRow{Code: inv.Code, Date: inv.CreatedAt.Format(dateFormat)}
	switch {
	case inv.UsedAt != nil:
		row.State = stateUsed
		row.Date = inv.UsedAt.Format(dateFormat)
	case time.Now().After(inv.ExpiresAt):
		row.State = stateExpired
	default:
		row.State = statePending
		row.Link = "https://t.me/" + c.botUsername + "?start=" + inv.Code
	}
	return row
}
