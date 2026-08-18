package messaging

import (
	"reflect"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/pendingjob"
)

// Fakes que ningún otro test del paquete necesitaba todavía. Son inertes a
// propósito: este test no ejercita comportamiento, sólo cableado.

type fakeNudgeRepo struct{}

func (fakeNudgeRepo) SentKeys(uint64) ([]string, error)     { return nil, nil }
func (fakeNudgeRepo) MarkSent(uint64, string) error         { return nil }
func (fakeNudgeRepo) MarkSentAgain(uint64, string) error    { return nil }
func (fakeNudgeRepo) MarkTapped(uint64, string) error       { return nil }
func (fakeNudgeRepo) LastSentAt(uint64) (*time.Time, error) { return nil, nil }

type fakeJobsRepo struct{}

func (fakeJobsRepo) Insert(*pendingjob.PendingJob) error { return nil }
func (fakeJobsRepo) ListByUserOrdered(uint64) ([]pendingjob.PendingJob, error) {
	return nil, nil
}
func (fakeJobsRepo) ListPendingUserIDs() ([]uint64, error) { return nil, nil }
func (fakeJobsRepo) Delete(uint64) error                   { return nil }
func (fakeJobsRepo) CountByUser(uint64) (int64, error)     { return 0, nil }

// TestNewController_WiresEveryDependency recorre por reflexión TODOS los campos
// del controller y exige que ninguno haya quedado nil.
//
// El bug que ataja compila sin una queja: se agrega un campo al struct y su
// parámetro a NewController, pero se olvida la línea `campo: campo` en el
// literal de retorno. Go acepta un parámetro sin usar, así que el campo queda
// nil y nadie se entera hasta producción.
//
// Y no falla ruidosamente: siete puentes de *controller tienen guardas
// `if c.X == nil { return nil }` para que los tests puedan construir un
// controller parcial. En producción esa misma guarda convierte una dependencia
// sin cablear en un no-op silencioso — el sistema no crashea, deja de
// registrar. Es exactamente cómo el eval de la cola perdía el park sin decir
// nada.
//
// Va por reflexión y no campo por campo para que no envejezca: un campo nuevo
// entra solo en la verificación.
func TestNewController_WiresEveryDependency(t *testing.T) {
	c := NewController(
		&fakeUserRepository{},
		&fakeInvitationRepository{},
		&fakeAccountRepoFull{},
		&fakeMovementRepoFull{},
		&fakeSubcategoryRepoFull{},
		conversation.NewEngine(&fakeConvStore{}, FlowResumeLabel),
		&fakeFullOrchestrator{},
		&fakeMetricRepo{},
		&stubChatHistory{},
		&fakeReminderRepo{},
		&fakeTraceRepo{},
		fakeNudgeRepo{},
		fakeJobsRepo{},
		&fakeActionsRepo{},
	)

	// Elem() y no ValueOf(*c): copiar el controller copia el sync.Map de locks,
	// y go vet lo rechaza (copies lock value).
	v := reflect.ValueOf(c).Elem()
	typ := v.Type()
	for i := range typ.NumField() {
		f := typ.Field(i)
		switch v.Field(i).Kind() {
		case reflect.Interface, reflect.Ptr, reflect.Map, reflect.Slice, reflect.Func:
			if v.Field(i).IsNil() {
				t.Errorf("NewController dejó %s.%s en nil: falta la línea en el literal de retorno",
					typ.Name(), f.Name)
			}
		default:
			// locks es un valor (sync.Map adentro): su cero ya es usable.
		}
	}
}
