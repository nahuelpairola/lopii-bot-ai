package agent

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func TestMatchesMessage_MultiWordDescription_SharedToken(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("gasto en trabas")}}}
	if !matchesMessage(group, "quiero eliminar mi registro de trabas") {
		t.Error("expected a match on the shared token 'trabas'")
	}
}

func TestMatchesMessage_ShortWordInLongMessage(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("Café")}}}
	if !matchesMessage(group, "le erre, el café salió 1500") {
		t.Error("expected a match on 'café' regardless of message length")
	}
}

func TestMatchesMessage_StopwordOnlyOverlap_NoMatch(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("de la")}}}
	if matchesMessage(group, "borra el de la lista") {
		t.Error("expected no match on stopword-only overlap")
	}
}

func TestMatchesMessage_Amount(t *testing.T) {
	amount := decimal.NewFromInt(1500)
	group := transactionGroup{Movements: []movement.Movement{{Amount: amount}}}
	if !matchesMessage(group, "fue 1500 pesos") {
		t.Error("expected a match on the amount '1500'")
	}
}

func TestScoreGroup_ExpenseAmountMatchesDespiteStoredSign(t *testing.T) {
	amount := decimal.NewFromFloat(-61306.49)
	now := time.Now()
	group := transactionGroup{Movements: []movement.Movement{{Type: movement.Expense, Amount: amount, Date: now}}}
	if got := scoreGroup(group, "eran 61306.49", now); got < 1 {
		t.Fatalf("scoreGroup = %v, want >= 1: el monto positivo del mensaje tiene que matchear el gasto guardado negativo", got)
	}
}

func TestMatchesMessage_DescriptionToken(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("compra en Carrefour")}}}
	if !matchesMessage(group, "el gasto en Carrefour fue mucho") {
		t.Error("expected a match on the description token 'Carrefour'")
	}
}

func TestMatchesMessage_NoMatch_EmptyDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("")}}}
	if matchesMessage(group, "some message") {
		t.Error("expected no match on empty description")
	}
}

func TestMatchesMessage_NoMatch_NilDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: nil}}}
	if matchesMessage(group, "some message") {
		t.Error("expected no match on nil description")
	}
}

func TestMatchesMessage_CaseInsensitive(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("TRABAS")}}}
	if !matchesMessage(group, "quiero eliminar mi registro de trabas") {
		t.Error("expected case-insensitive match")
	}
}

func TestMatchesMessage_UnaccentedMessageMatchesAccentedDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("compra en panadería")}}}
	if !matchesMessage(group, "La panaderia era 2k") {
		t.Error("unaccented message should match an accented description")
	}
}

func TestMatchesMessage_AccentedMessageMatchesUnaccentedDescription(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("compra en panaderia")}}}
	if !matchesMessage(group, "La panadería era 2k") {
		t.Error("accented message should match an unaccented description")
	}
}

func TestMatchesMessage_UnrelatedTokenStillDoesNotMatch(t *testing.T) {
	group := transactionGroup{Movements: []movement.Movement{{Description: strPtr("compra en panadería")}}}
	if matchesMessage(group, "nafta en la estación") {
		t.Error("unrelated message should not match")
	}
}

func TestMatchesMessage_TransferWithAccount(t *testing.T) {
	accountID := uint64(123)
	m := movement.Movement{
		AccountID:   &accountID,
		Description: strPtr("transferencia"),
		Amount:      decimal.NewFromInt(500),
		Currency:    currency.ARS,
	}
	group := transactionGroup{Movements: []movement.Movement{m}}
	if !matchesMessage(group, "la transferencia de 500") {
		t.Error("expected a match on the description token 'transferencia'")
	}
}

func TestScoreGroup_DebitoTarjetaLeGanaALasTransferencias(t *testing.T) {
	msg := "Del débito tarjeta Mercado Pago se me reintegraron $70.000"
	anchor := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	date := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)

	group := func(desc string) transactionGroup {
		return transactionGroup{Movements: []movement.Movement{{Description: strPtr(desc), Date: date}}}
	}
	correcto := scoreGroup(group("Débito tarjeta Mercado Pago"), msg, anchor)
	for _, ruido := range []string{
		"Pago tarjeta de crédito",
		"Transferencia a Mercado Pago",
		"Transferencia Mercado Pago a FCI",
		"Transferencia Mercado Pago a Cedears",
		"Transferencia Banco Galicia a Mercado Pago",
	} {
		if got := scoreGroup(group(ruido), msg, anchor); got >= correcto {
			t.Errorf("%q puntuó %v, y el correcto %v: el ruido no puede empatarle", ruido, got, correcto)
		}
	}
}

func TestScoreGroup_LaFechaDesempataPeroNoManda(t *testing.T) {
	msg := "la compra de locro"
	anchor := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	group := func(desc string, d time.Time) transactionGroup {
		return transactionGroup{Movements: []movement.Movement{{Description: strPtr(desc), Date: d}}}
	}

	viejoYExacto := scoreGroup(group("Compra de locro", anchor.AddDate(0, 0, -15)), msg, anchor)
	nuevoYFlojo := scoreGroup(group("Compra USD 100 a 1500", anchor), msg, anchor)
	if viejoYExacto <= nuevoYFlojo {
		t.Fatalf("exacto y viejo = %v, flojo y nuevo = %v: la fecha dio vuelta la cobertura", viejoYExacto, nuevoYFlojo)
	}

	cerca := scoreGroup(group("Compra de locro", anchor.AddDate(0, 0, -1)), msg, anchor)
	lejos := scoreGroup(group("Compra de locro", anchor.AddDate(0, 0, -20)), msg, anchor)
	if cerca <= lejos {
		t.Fatalf("cerca = %v, lejos = %v: a cobertura igual tiene que ganar el más cercano", cerca, lejos)
	}
}

func TestScoreGroup_SinCoberturaEsCero(t *testing.T) {
	g := transactionGroup{Movements: []movement.Movement{
		{Description: strPtr("Café"), Date: time.Now()},
	}}
	if got := scoreGroup(g, "era 700", time.Now()); got != 0 {
		t.Fatalf("scoreGroup = %v, want 0: sin token compartido no hay candidato", got)
	}
}

type fakeMovementRepoForResolve struct {
	result               []movement.Movement
	capturedSince        time.Time
	capturedUntil        *time.Time
	capturedRecencySince time.Time
	capturedLimit        int
	recencyCalled        bool
}

func (r *fakeMovementRepoForResolve) InsertBatch(ms []movement.Movement) error {
	return nil
}

func (r *fakeMovementRepoForResolve) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return decimal.Zero, nil
}

func (r *fakeMovementRepoForResolve) ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error {
	return nil
}

func (r *fakeMovementRepoForResolve) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	r.capturedSince = since
	r.capturedUntil = until
	return r.result, nil
}

func (r *fakeMovementRepoForResolve) FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error) {
	r.recencyCalled = true
	r.capturedRecencySince = since
	r.capturedLimit = limit
	return r.result, nil
}

func (r *fakeMovementRepoForResolve) SoftDeleteByIDs(ids []uint) error {
	return nil
}

func (r *fakeMovementRepoForResolve) InsertAccountsWithOpenings(items []movement.AccountOpening) error {
	return nil
}
func (r *fakeMovementRepoForResolve) SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return nil, nil
}
func (r *fakeMovementRepoForResolve) ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return nil, nil
}
func (r *fakeMovementRepoForResolve) ReassignAccount(fromID, toID uint64) error { return nil }
func (r *fakeMovementRepoForResolve) CountForUser(userID uint64) (int64, error) { return 0, nil }
func (r *fakeMovementRepoForResolve) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return 0, nil
}
func (r *fakeMovementRepoForResolve) ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error {
	return nil
}
func (r *fakeMovementRepoForResolve) TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error) {
	return nil, nil
}

func TestResolveCandidates_NoDate_UsesCreatedAtRecencyWindow(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{{Model: gorm.Model{ID: 1}, Description: strPtr("Nafta YPF")}}}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 42, "che, lo de la nafta ypf era otro monto", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	if !fake.recencyCalled {
		t.Fatal("expected FindRecentlyCreatedForUser to be used when no date is mentioned")
	}
	want := time.Now().Add(-recencyWindow)
	if diff := fake.capturedRecencySince.Sub(want); diff > 2*time.Second || diff < -2*time.Second {
		t.Errorf("recency since = %v, want ~%v", fake.capturedRecencySince, want)
	}
}

func TestResolveCandidates_PastDatedButRecentlyCreated_Resolves(t *testing.T) {
	yesterday := time.Now().AddDate(0, 0, -1)
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 71}, Date: yesterday, Description: strPtr("Pago a Pablo por asado")},
	}}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 2, "Perdon, el asado eran 15 mil", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1 (token 'asado' matched)", len(candidates))
	}
}

func TestResolveCandidates_NoTextMatch_FallsBackToRecentWindow(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 1}, Description: strPtr("Café")},
		{Model: gorm.Model{ID: 2}, Description: strPtr("Panadería")},
	}}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 42, "era 700", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("got %d candidates, want 2 (fallback to recent window)", len(candidates))
	}
}

func TestResolveCandidates_EmptyWindow_ReturnsNoCandidates(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: nil}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 42, "era 700", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("got %d candidates, want 0 (nothing in window)", len(candidates))
	}
}

func TestResolveCandidates_NoTextMatch_JustCreated_ResolvesToThatOne(t *testing.T) {
	now := time.Now()
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 9, CreatedAt: now.Add(-30 * time.Second)}, Description: strPtr("Pan")},
		{Model: gorm.Model{ID: 8, CreatedAt: now.Add(-5 * time.Hour)}, Description: strPtr("Helado")},
		{Model: gorm.Model{ID: 7, CreatedAt: now.Add(-6 * time.Hour)}, Description: strPtr("Nafta")},
	}}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 2, "Eran 1500", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1: lo acaba de cargar, no hay que preguntarle cuál", len(candidates))
	}
	if candidates[0].Movements[0].ID != 9 {
		t.Errorf("candidate ID = %d, want 9 (el más recién creado)", candidates[0].Movements[0].ID)
	}
}

func TestResolveCandidates_Fallback_SkipsSystemMovements(t *testing.T) {
	old := time.Now().Add(-5 * time.Hour)
	sys := func(sub string) *subcategory.Subcategory {
		return &subcategory.Subcategory{Category: "Sistema", Subcategory: sub}
	}
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 3, CreatedAt: old}, Description: strPtr("Jubilación"), Subcategory: sys("Saldo inicial")},
		{Model: gorm.Model{ID: 2, CreatedAt: old}, Description: strPtr("FCI"), Subcategory: sys("Transferencia")},
		{Model: gorm.Model{ID: 1, CreatedAt: old}, Description: strPtr("Helado"),
			Subcategory: &subcategory.Subcategory{Category: "Ocio y salidas", Subcategory: "Salir a comer"}},
	}}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 2, "era 700", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1: los movimientos de Sistema no van al picker", len(candidates))
	}
	if candidates[0].Movements[0].ID != 1 {
		t.Errorf("candidate ID = %d, want 1 (el único que no es de Sistema)", candidates[0].Movements[0].ID)
	}
}

func TestCandidateLabel(t *testing.T) {
	nowART := time.Now().In(constants.ArgentinaZone)
	today, _ := time.Parse("2006-01-02", nowART.Format("2006-01-02"))
	sub := func(cat, s string) *subcategory.Subcategory {
		return &subcategory.Subcategory{Category: cat, Subcategory: s}
	}
	mk := func(amount string, cur currency.Currency, desc *string, s *subcategory.Subcategory, d time.Time) transactionGroup {
		return transactionGroup{Movements: []movement.Movement{{
			Type: movement.Expense, Amount: mustDecimal(t, amount), Currency: cur,
			Description: desc, Subcategory: s, Date: d,
		}}}
	}

	cases := []struct {
		name  string
		group transactionGroup
		want  string
	}{
		{
			name:  "monto en formato argentino y fecha relativa",
			group: mk("2000", currency.ARS, strPtr("Pan"), sub("Alimentación", "Almacén / barrio"), today),
			want:  "🔴 Pan · $2.000 · hoy",
		},
		{
			name:  "sin descripción cae a la subcategoría, nunca deja '· ·'",
			group: mk("21528105", currency.ARS, nil, sub("Sistema", "Saldo inicial"), today.AddDate(0, 0, -1)),
			want:  "🔴 Saldo inicial · $21.528.105 · ayer",
		},
		{
			name:  "USD conserva los centavos y lleva su símbolo",
			group: mk("3614.66", currency.USD, nil, sub("Inversiones", "FCI"), today.AddDate(0, 0, -3)),
			want:  "🔴 FCI · US$3.614,66 · " + today.AddDate(0, 0, -3).Format("02/01"),
		},
		{
			name:  "el signo almacenado no llega al usuario",
			group: mk("-2000", currency.ARS, strPtr("Pan"), sub("Alimentación", "Almacén / barrio"), today),
			want:  "🔴 Pan · $2.000 · hoy",
		},
	}

	for _, tc := range cases {
		if got := candidateLabel(tc.group); got != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveCandidates_ManyTextMatches_IsCapped(t *testing.T) {
	now := time.Now()
	var many []movement.Movement
	for i := 0; i < 20; i++ {
		many = append(many, movement.Movement{
			Model:       gorm.Model{ID: uint(100 + i), CreatedAt: now.Add(-time.Duration(i+2) * time.Hour)},
			Description: strPtr("compra en el super"),
		})
	}
	fake := &fakeMovementRepoForResolve{result: many}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 2, "el super era 8 mil", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) > pickerMaxOptions {
		t.Fatalf("got %d candidates, want <= %d: un picker no puede tener 20 botones", len(candidates), pickerMaxOptions)
	}
	if candidates[0].Movements[0].ID != 100 {
		t.Errorf("primer candidato = %d, want 100 (el corte tiene que dejar los más recientes)", candidates[0].Movements[0].ID)
	}
}

func TestResolveCandidates_ElCorteEsPorParecidoNoPorRecencia(t *testing.T) {
	now := time.Now()
	var window []movement.Movement
	for i := 0; i < 5; i++ {
		window = append(window, movement.Movement{
			Model:       gorm.Model{ID: uint(200 + i), CreatedAt: now.Add(-time.Duration(i+2) * time.Hour)},
			Description: strPtr("Transferencia Banco Galicia a Mercado Pago"),
		})
	}
	window = append(window, movement.Movement{
		Model:       gorm.Model{ID: 205, CreatedAt: now.Add(-200 * time.Hour)},
		Description: strPtr("Débito tarjeta Mercado Pago"),
	})

	fake := &fakeMovementRepoForResolve{result: window}
	svc := &fakeServices{movements: fake}

	candidates, err := resolveCandidates(svc, 3, "Del débito tarjeta Mercado Pago se me reintegraron $70.000", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("sin candidatos")
	}
	if candidates[0].Movements[0].ID != 205 {
		t.Errorf("primer candidato = %d, want 205: el que nombra la fila entera va primero, aunque sea el más viejo",
			candidates[0].Movements[0].ID)
	}
}

func TestResolveCandidates_NoDate_WindowIsDynamic(t *testing.T) {
	fake := &fakeMovementRepoForResolve{result: []movement.Movement{
		{Model: gorm.Model{ID: 1}, Description: strPtr("Café")},
	}}
	svc := &fakeServices{movements: fake}

	if _, err := resolveCandidates(svc, 42, "era 700", "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.capturedLimit != recencyLimit {
		t.Errorf("limit = %d, want %d: la ventana se acota por cantidad, no solo por tiempo", fake.capturedLimit, recencyLimit)
	}
	if recencyWindow < 30*24*time.Hour {
		t.Errorf("recencyWindow = %v, want >= 30 días: es un techo contra fósiles, no la ventana real", recencyWindow)
	}
	if recencyLimit < 60 {
		t.Errorf("recencyLimit = %d, want >= 60: con 30, el débito de tarjeta del 2026-08-16 quedaba en la fila 32", recencyLimit)
	}
}

func TestRelativeDate_MovementDateIsCivilNotInstant(t *testing.T) {
	parseDay := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return d
	}
	now := time.Now().In(constants.ArgentinaZone)

	cases := []struct{ date, want string }{
		{now.Format("2006-01-02"), "hoy"},
		{now.AddDate(0, 0, -1).Format("2006-01-02"), "ayer"},
		{now.AddDate(0, 0, -4).Format("2006-01-02"), now.AddDate(0, 0, -4).Format("02/01")},
	}
	for _, tc := range cases {
		if got := movement.RelativeDate(parseDay(tc.date)); got != tc.want {
			t.Errorf("movement.RelativeDate(%s) = %q, want %q", tc.date, got, tc.want)
		}
	}
}

func (r *fakeMovementRepoForResolve) CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return nil, nil
}

func TestResolveCandidates_LoneDateFromAnchorsASingleDay(t *testing.T) {
	fake := &fakeMovementRepoForResolve{}
	svc := &fakeServices{movements: fake}

	if _, err := resolveCandidates(svc, 3, "el débito de tarjeta del 04 de agosto", "2026-08-04", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.recencyCalled {
		t.Fatal("con fecha no se usa la ventana de created_at")
	}
	if want := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC); !fake.capturedSince.Equal(want) {
		t.Errorf("since = %v, want %v", fake.capturedSince, want)
	}
	if fake.capturedUntil == nil {
		t.Fatal("una fecha sola dejó la ventana abierta hasta hoy: no acota nada")
	}
	if want := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC); !fake.capturedUntil.Equal(want) {
		t.Errorf("until = %v, want %v", *fake.capturedUntil, want)
	}
}

func TestResolveCandidates_DateRangeCoversTheWholeSpan(t *testing.T) {
	fake := &fakeMovementRepoForResolve{}
	svc := &fakeServices{movements: fake}

	if _, err := resolveCandidates(svc, 3, "la compra de la semana pasada ponela en otra categoría", "2026-08-10", "2026-08-16"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.recencyCalled {
		t.Fatal("con fecha no se usa la ventana de created_at")
	}
	if want := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC); !fake.capturedSince.Equal(want) {
		t.Errorf("since = %v, want %v", fake.capturedSince, want)
	}
	if fake.capturedUntil == nil {
		t.Fatal("until = nil: la semana quedó abierta")
	}
	if want := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC); !fake.capturedUntil.Equal(want) {
		t.Errorf("until = %v, want %v (el tramo se colapsó)", *fake.capturedUntil, want)
	}
}

func TestResolveCandidates_LoneDateToAnchorsSymmetrically(t *testing.T) {
	fake := &fakeMovementRepoForResolve{}
	svc := &fakeServices{movements: fake}

	if _, err := resolveCandidates(svc, 3, "lo de hasta el 4 de agosto", "", "2026-08-04"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC); !fake.capturedSince.Equal(want) {
		t.Errorf("since = %v, want %v (la ventana salía invertida)", fake.capturedSince, want)
	}
	if fake.capturedUntil == nil {
		t.Fatal("until = nil")
	}
	if want := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC); !fake.capturedUntil.Equal(want) {
		t.Errorf("until = %v, want %v", *fake.capturedUntil, want)
	}
}
