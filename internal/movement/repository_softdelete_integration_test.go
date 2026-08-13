//go:build integration

package movement

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
)

// Run with: go test -tags integration ./internal/movement/
// Requires local Postgres (docker compose up -d) with migrations applied.
//
// El contrato que fija este archivo: borrar algo que YA no está es el estado
// final pedido, no una falla. Antes no era así, y el 2026-08-10 dos
// correcciones que el usuario había confirmado murieron acá — el usuario tocó
// "confirmar", la fila ya estaba borrada, y el bot le dijo que había fallado.
// Los dos caminos que lo disparan siguen vivos: el doble tap en el botón y el
// replay que hace pendingjob después de un 429.

func TestSoftDeleteByIDs_EmptySliceIsAnError(t *testing.T) {
	r := InitRepository(testConnection(t))

	if err := r.SoftDeleteByIDs(nil); !errors.Is(err, ErrNoMovementIDs) {
		t.Fatalf("SoftDeleteByIDs(nil) = %v, want ErrNoMovementIDs", err)
	}
}

func TestSoftDeleteByIDs_AlreadyDeletedIsNotAnError(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	id := seedDeletableMovement(t, conn, r)

	if err := r.SoftDeleteByIDs([]uint{id}); err != nil {
		t.Fatalf("primer borrado: %v", err)
	}

	// El usuario confirma dos veces, o entra un replay del 429.
	if err := r.SoftDeleteByIDs([]uint{id}); err != nil {
		t.Fatalf("segundo borrado = %v, want nil (el estado final es el pedido)", err)
	}
}

func TestSoftDeleteByIDs_UnknownIDIsAnError(t *testing.T) {
	r := InitRepository(testConnection(t))

	if err := r.SoftDeleteByIDs([]uint{999999999}); !errors.Is(err, ErrMovementNotFound) {
		t.Fatalf("SoftDeleteByIDs(inexistente) = %v, want ErrMovementNotFound", err)
	}
}

// seedDeletableMovement inserta una cuenta propia y un movimiento suelto sobre
// ella, y devuelve el id del movimiento. Fecha fija y lejana para no pisar
// datos reales del usuario de prueba.
func seedDeletableMovement(t *testing.T, conn *database.Connection, r *repository) uint {
	t.Helper()

	accRepo := account.NewRepository(conn)
	acc := &account.Account{UserID: 1, Name: "SoftDelete_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	day := time.Date(2031, 5, 20, 0, 0, 0, 0, time.UTC)
	movs := []Movement{{
		UserID:        1,
		AccountID:     &accID,
		SubcategoryID: subcategoryID(t, conn, "Alimentación", ""),
		Date:          day,
		Type:          Expense,
		Amount:        decimal.NewFromInt(-1234),
		Currency:      currency.ARS,
	}}
	if err := r.InsertBatch(movs); err != nil {
		t.Fatalf("insert movement: %v", err)
	}
	return movs[0].ID
}
