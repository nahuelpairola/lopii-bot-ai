package flow

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

func ptrTo[T any](v T) *T { return &v }

func ndMov(id uint, userID uint64, acct uint64, cur currency.Currency, amount, desc string, at time.Time) movement.Movement {
	return movement.Movement{
		Model:       gorm.Model{ID: id, CreatedAt: at},
		UserID:      userID,
		AccountID:   ptrTo(acct),
		Currency:    cur,
		Amount:      decimal.RequireFromString(amount),
		Description: ptrTo(desc),
	}
}

func TestFindNearDuplicate(t *testing.T) {
	now := time.Date(2026, 8, 11, 22, 27, 28, 0, time.UTC)
	base := ndMov(253, 1, 43, currency.ARS, "-1070", "Café", now)

	cases := []struct {
		name  string
		prior movement.Movement
		want  bool
	}{
		{"253 vs 250: token compartido a 6m07s",
			ndMov(250, 1, 43, currency.ARS, "-12700", "Cafe", now.Add(-6*time.Minute-7*time.Second)), true},
		{"249 vs 248: monto idéntico a 1m31s (Pollo / Pago en Polleria NO comparten token)",
			ndMov(248, 1, 43, currency.ARS, "-1070", "Pago en Polleria", now.Add(-91*time.Second)), true},
		{"acentos: panadería vs panaderia",
			ndMov(200, 1, 43, currency.ARS, "-500", "panaderia", now.Add(-time.Minute)), false},

		{"justo en el borde de JustCreatedWindow",
			ndMov(201, 1, 43, currency.ARS, "-1070", "otra cosa", now.Add(-JustCreatedWindow)), true},
		{"un segundo pasado el borde",
			ndMov(202, 1, 43, currency.ARS, "-1070", "otra cosa", now.Add(-JustCreatedWindow-time.Second)), false},
		{"un previo del futuro no cuenta",
			ndMov(203, 1, 43, currency.ARS, "-1070", "otra cosa", now.Add(time.Minute)), false},

		{"token de exactamente minMatchTokenLen (Cafe)",
			ndMov(204, 1, 43, currency.ARS, "-99", "Cafe", now.Add(-time.Minute)), true},
		{"token más corto que el mínimo (pan)",
			ndMov(205, 1, 43, currency.ARS, "-99", "pan", now.Add(-time.Minute)), false},

		{"otro usuario", ndMov(206, 2, 43, currency.ARS, "-1070", "Café", now.Add(-time.Minute)), false},
		{"otra cuenta", ndMov(207, 1, 99, currency.ARS, "-1070", "Café", now.Add(-time.Minute)), false},
		{"otra moneda", ndMov(208, 1, 43, currency.USD, "-1070", "Café", now.Add(-time.Minute)), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FindNearDuplicate(base, nil, []movement.Movement{tc.prior})
			if (got != nil) != tc.want {
				t.Errorf("FindNearDuplicate = %v, want flagged=%v", got, tc.want)
			}
		})
	}
}

func TestFindNearDuplicate_FoldsAccents(t *testing.T) {
	now := time.Now()
	base := ndMov(300, 1, 43, currency.ARS, "-500", "compra en panadería", now)
	prior := ndMov(301, 1, 43, currency.ARS, "-999", "panaderia del barrio", now.Add(-time.Minute))

	if FindNearDuplicate(base, nil, []movement.Movement{prior}) == nil {
		t.Error("panadería/panaderia tiene que marcar: el fold de acentos ya está resuelto")
	}
}

func TestFindNearDuplicate_TransferLegsAreNotDuplicates(t *testing.T) {
	now := time.Now()
	tx := uuid.New()
	out := ndMov(400, 1, 43, currency.ARS, "-5000", "pase a caja", now)
	in := ndMov(401, 1, 43, currency.ARS, "5000", "pase a caja", now.Add(-time.Second))
	out.TransactionID, in.TransactionID = &tx, &tx

	if FindNearDuplicate(out, nil, []movement.Movement{in}) != nil {
		t.Error("las dos patas de un transfer no pueden marcarse entre sí")
	}
}

func TestFindNearDuplicate_SameTurnBatchDoesNotFlagItself(t *testing.T) {
	now := time.Now()
	first := ndMov(500, 1, 43, currency.ARS, "-1070", "café", now.Add(-time.Second))
	second := ndMov(501, 1, 43, currency.ARS, "-1070", "café", now)

	if got := FindNearDuplicate(second, []uint{500, 501}, []movement.Movement{first}); got != nil {
		t.Errorf("marcó %v: dos filas del MISMO mensaje no son un duplicado", got)
	}
	if FindNearDuplicate(second, nil, []movement.Movement{first}) == nil {
		t.Error("sin la exclusión del turno tendría que marcar; el test anterior es vacío")
	}
}

func TestFindNearDuplicate_IgnoresSoftDeleted(t *testing.T) {
	now := time.Now()
	base := ndMov(600, 1, 43, currency.ARS, "-1070", "Café", now)
	prior := ndMov(601, 1, 43, currency.ARS, "-1070", "Café", now.Add(-time.Minute))
	prior.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}

	if FindNearDuplicate(base, nil, []movement.Movement{prior}) != nil {
		t.Error("un movimiento ya borrado no puede ser el duplicado")
	}
}

func TestFindNearDuplicate_PicksOneAndPrefersTheToken(t *testing.T) {
	now := time.Now()
	base := ndMov(700, 1, 43, currency.ARS, "-1070", "Café", now)
	byAmount := ndMov(701, 1, 43, currency.ARS, "-1070", "kiosco", now.Add(-2*time.Minute))
	byToken := ndMov(702, 1, 43, currency.ARS, "-9999", "Cafe", now.Add(-5*time.Minute))

	got := FindNearDuplicate(base, nil, []movement.Movement{byAmount, byToken})
	if got == nil || got.ID != 702 {
		t.Errorf("eligió %v, want el match por token (702)", got)
	}
}

func TestFindNearDuplicate_PrefersTheMostRecentOfTheSameKind(t *testing.T) {
	now := time.Now()
	base := ndMov(800, 1, 43, currency.ARS, "-1070", "Café", now)
	older := ndMov(801, 1, 43, currency.ARS, "-1070", "kiosco", now.Add(-8*time.Minute))
	newer := ndMov(802, 1, 43, currency.ARS, "-1070", "kiosco", now.Add(-1*time.Minute))

	got := FindNearDuplicate(base, nil, []movement.Movement{older, newer})
	if got == nil || got.ID != 802 {
		t.Errorf("eligió %v, want el más reciente (802)", got)
	}
}

func TestFindNearDuplicate_NeverTouchesATransferLeg(t *testing.T) {
	tx1, tx2 := uuid.New(), uuid.New()
	acc := uint64(46)
	desc := "Suscripción a FCI"

	pata1 := movement.Movement{
		Model:  gorm.Model{ID: 1, CreatedAt: time.Now().Add(-2 * time.Minute)},
		UserID: 2, AccountID: &acc, Currency: currency.ARS, TransactionID: &tx1,
		Amount: decimal.NewFromInt(-310000), Description: &desc, Type: movement.Transfer,
	}
	pata2 := movement.Movement{
		Model:  gorm.Model{ID: 2, CreatedAt: time.Now()},
		UserID: 2, AccountID: &acc, Currency: currency.ARS, TransactionID: &tx2,
		Amount: decimal.NewFromInt(-500000), Description: &desc, Type: movement.Transfer,
	}

	if got := FindNearDuplicate(pata2, nil, []movement.Movement{pata1}); got != nil {
		t.Errorf("marcó la pata #%d: fusionar una pata rompe los dos grupos", got.ID)
	}
	gasto := pata2
	gasto.TransactionID = nil
	gasto.Type = movement.Expense
	if got := FindNearDuplicate(gasto, nil, []movement.Movement{pata1}); got != nil {
		t.Errorf("un gasto marcó la pata #%d como duplicado", got.ID)
	}
}

func TestFindNearDuplicate_ARefundIsNotADuplicateOfItsExpense(t *testing.T) {
	now := time.Now()
	gasto := ndMov(500, 1, 43, currency.ARS, "-5000", "Super", now.Add(-2*time.Minute))
	gasto.Type = movement.Expense
	reintegro := ndMov(501, 1, 43, currency.ARS, "5000", "Super, me lo devolvieron", now)
	reintegro.Type = movement.Income

	if got := FindNearDuplicate(reintegro, nil, []movement.Movement{gasto}); got != nil {
		t.Errorf("el reintegro marcó al gasto #%d: fusionarlos da una fila de monto 0", got.ID)
	}
	reintegroPrimero := ndMov(502, 1, 43, currency.ARS, "5000", "Super, me lo devolvieron", now.Add(-2*time.Minute))
	reintegroPrimero.Type = movement.Income
	gastoSegundo := ndMov(503, 1, 43, currency.ARS, "-5000", "Super", now)
	gastoSegundo.Type = movement.Expense

	if got := FindNearDuplicate(gastoSegundo, nil, []movement.Movement{reintegroPrimero}); got != nil {
		t.Errorf("el gasto marcó al reintegro #%d", got.ID)
	}
}
