//go:build integration

package movement

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/subcategory"
)

func TestMovementQuery_Search_MatchesAllThreeLegs(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	acc := &account.Account{UserID: userID, Name: "Search_" + uuid.NewString()[:8], Currency: currency.ARS}
	if err := accRepo.Insert(acc); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accID := uint64(acc.ID)

	byCategory := ownSubcategory(t, conn, userID, "Panadería", "Facturas")
	bySubcategory := ownSubcategory(t, conn, userID, "Comida", "Panadería")
	byDescription := ownSubcategory(t, conn, userID, "Comida", "Super")
	noMatch := ownSubcategory(t, conn, userID, "Transporte", "Nafta")

	day := time.Date(2032, 5, 10, 0, 0, 0, 0, time.UTC)
	algo, delaEsquina, nafta := "algo", "pan de la PANADERÍA de la esquina", "nafta"
	if err := r.InsertBatch([]Movement{
		{UserID: userID, AccountID: &accID, SubcategoryID: byCategory, Date: day, Type: Expense, Amount: decimal.NewFromInt(-100), Currency: currency.ARS, Description: &algo},
		{UserID: userID, AccountID: &accID, SubcategoryID: bySubcategory, Date: day, Type: Expense, Amount: decimal.NewFromInt(-200), Currency: currency.ARS, Description: &algo},
		{UserID: userID, AccountID: &accID, SubcategoryID: byDescription, Date: day, Type: Expense, Amount: decimal.NewFromInt(-300), Currency: currency.ARS, Description: &delaEsquina},
		{UserID: userID, AccountID: &accID, SubcategoryID: noMatch, Date: day, Type: Expense, Amount: decimal.NewFromInt(-400), Currency: currency.ARS, Description: &nafta},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	term := "panaderia"
	q := MovementQuery{
		UserID:    userID,
		From:      day.AddDate(0, 0, -1),
		To:        day.AddDate(0, 0, 1),
		Currency:  currency.ARS,
		AccountID: &accID,
		Search:    &term,
	}

	rows, err := r.ListForUser(q, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("search %q tiene que traer las 3 patas del OR y NO el control negativo; trajo %d", term, len(rows))
	}
}

func TestMovementQuery_Search_RealProductionPhrases(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	accRepo := account.NewRepository(conn)
	userID := uint64(1)

	src := &account.Account{UserID: userID, Name: "SearchSrc_" + uuid.NewString()[:8], Currency: currency.ARS}
	dst := &account.Account{UserID: userID, Name: "SearchDst_" + uuid.NewString()[:8], Currency: currency.ARS}
	for _, a := range []*account.Account{src, dst} {
		if err := accRepo.Insert(a); err != nil {
			t.Fatalf("insert account: %v", err)
		}
	}
	srcID, dstID := uint64(src.ID), uint64(dst.ID)

	vehiculo := ownSubcategory(t, conn, userID, "Transporte", "Seguro vehículo")
	hogar := ownSubcategory(t, conn, userID, "Vivienda", "Seguro hogar")
	super := ownSubcategory(t, conn, userID, "Alimentación", "Supermercado")
	almacen := ownSubcategory(t, conn, userID, "Alimentación", "Almacén / barrio")
	streaming := ownSubcategory(t, conn, userID, "Suscripciones", "Streaming")
	delivery := ownSubcategory(t, conn, userID, "Ocio y salidas", "Delivery")
	fci := ownSubcategory(t, conn, userID, "Inversiones", "FCI")

	const (
		segMoto  = "Seguro moto"
		segAuto  = "Seguro auto"
		segHogar = "seguro del hogar"
		miga     = "18 mil sandwiches de miga"
		carne    = "Carnicería"
		hbo      = "HBO Max"
		locro    = "Compra de locro"
		fciMP    = "Transferencia FCI a Mercado Pago"
	)
	day := time.Date(2033, 8, 19, 0, 0, 0, 0, time.UTC)
	expense := func(sub uint64, desc string, amount int64) Movement {
		d := desc
		return Movement{UserID: userID, AccountID: &srcID, SubcategoryID: sub, Date: day, Type: Expense, Amount: decimal.NewFromInt(-amount), Currency: currency.ARS, Description: &d}
	}
	txID := uuid.New()
	transferDesc := fciMP
	if err := r.InsertBatch([]Movement{
		expense(vehiculo, segMoto, 10980),
		expense(vehiculo, segAuto, 34540),
		expense(hogar, segHogar, 20305),
		expense(super, miga, 18000),
		expense(almacen, carne, 9000),
		expense(streaming, hbo, 8122),
		expense(delivery, locro, 12000),
		{TransactionID: &txID, UserID: userID, AccountID: &srcID, SubcategoryID: fci, Date: day, Type: Transfer, Amount: decimal.NewFromInt(-100000), Currency: currency.ARS, Description: &transferDesc},
		{TransactionID: &txID, UserID: userID, AccountID: &dstID, SubcategoryID: fci, Date: day, Type: Transfer, Amount: decimal.NewFromInt(100000), Currency: currency.ARS, Description: &transferDesc},
	}); err != nil {
		t.Fatalf("insert movements: %v", err)
	}

	transfer := string(Transfer)
	cases := []struct {
		name   string
		search string
		typ    *string
		want   []string
	}{
		{"phrase_with_connectors_matches_only_that_vehicle", "seguro de la moto", nil, []string{segMoto}},
		{"phrase_with_del_matches_only_the_car", "seguro del auto", nil, []string{segAuto}},
		{"two_words_matched_across_description_only", "seguro moto", nil, []string{segMoto}},
		{"every_word_may_sit_in_a_different_field", "FCI Mercado Pago", &transfer, []string{fciMP}},
		{"single_generic_word_still_matches_every_insurance", "seguro", nil, []string{segMoto, segAuto, segHogar}},
		{"word_matches_inside_a_longer_word", "super", nil, []string{miga}},
		{"three_letter_word_is_kept", "HBO", nil, []string{hbo}},
		{"three_letter_word_is_kept_on_transfers", "FCI", &transfer, []string{fciMP}},
		{"category_name_with_connector_matches", "ocio y salidas", nil, []string{locro}},
		{"accented_term_matches", "Almacén", nil, []string{carne}},
		{"only_connectors_falls_back_to_the_whole_term", "del", nil, []string{segHogar, locro}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			term := tc.search
			q := MovementQuery{
				UserID:    userID,
				From:      day,
				To:        day,
				Currency:  currency.ARS,
				AccountID: &srcID,
				Type:      tc.typ,
				Search:    &term,
			}
			rows, err := r.ListForUser(q, 50, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]bool{}
			for _, m := range rows {
				got[*m.Description] = true
			}
			if len(got) != len(tc.want) || len(rows) != len(tc.want) {
				t.Fatalf("search %q: quería %v, trajo %v", tc.search, tc.want, got)
			}
			for _, w := range tc.want {
				if !got[w] {
					t.Fatalf("search %q: quería %v, trajo %v", tc.search, tc.want, got)
				}
			}
		})
	}
}

func ownSubcategory(t *testing.T, conn *database.Connection, userID uint64, category, sub string) uint64 {
	t.Helper()
	s := subcategory.Subcategory{UserID: &userID, Category: category, Subcategory: sub + "_" + uuid.NewString()[:6]}
	if err := conn.DB.Create(&s).Error; err != nil {
		t.Fatalf("insert subcategory %q|%q: %v", category, sub, err)
	}
	return uint64(s.ID)
}
