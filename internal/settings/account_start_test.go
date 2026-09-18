package settings

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

type zeroBalance struct{}

func (zeroBalance) SumAmountForAccount(uint64) (decimal.Decimal, error) { return decimal.Zero, nil }

func acc(id uint, name string, cur currency.Currency) account.Account {
	a := account.Account{Name: name, Currency: cur}
	a.ID = id
	return a
}

func prodLikeAccounts() []account.Account {
	return []account.Account{
		acc(43, "Mercado Pago", currency.ARS),
		acc(44, "Mercado Pago", currency.USD),
		acc(45, "FCI", currency.ARS),
		acc(46, "FCI", currency.USD),
		acc(47, "Inversión", currency.ARS),
		acc(48, "Efectivo", currency.USD),
		acc(50, "Jubilación", currency.ARS),
		acc(51, "Cedears", currency.ARS),
		acc(52, "Banco Galicia", currency.ARS),
		acc(53, "Banco Galicia", currency.USD),
	}
}

func startManage(t *testing.T, text string, res orchestrator.AccountManageResult) *fakeStateStore {
	t.Helper()
	store := &fakeStateStore{}
	engine := conversation.NewEngine(store, func(string) string { return "algo" })
	engine.Register(flow.NewAccountManageFlow(zeroBalance{}))
	s := &testServices{engine: engine, accounts: prodLikeAccounts(), accManage: res}
	if err := StartAccountManage(context.Background(), s, &messenger.FakeChat{}, 2, text); err != nil {
		t.Fatalf("StartAccountManage: %v", err)
	}
	return store
}

func TestStartAccountManage_RealMessages(t *testing.T) {
	const (
		confirm = flow.StepAccountManageConfirmAdjust
		askAmt  = flow.StepAccountManageAskTotal
		menu    = flow.StepAccountManageMenu
		pick    = flow.StepAccountManagePick
	)
	cases := []struct {
		msg       string
		wantStep  string
		wantAccID string
		wantTotal string
	}{
		{"Mercado Pago a $207555,66", confirm, "43", "207555,66"},
		{"Ajustar saldo cuenta fci", askAmt, "45", ""},
		{"Ajustar saldo mercado pago", askAmt, "43", ""},
		{"aJUSTAR SALDO MERCADO PAGO USD A 496,1", confirm, "44", "496,1"},
		{"Ajuste saldo inversión", askAmt, "47", ""},
		{"Ajuste de saldo fci usd", askAmt, "46", ""},
		{"Ajusta saldo de fci usd 3600,16", confirm, "46", "3600,16"},
		{"Actualizar monto de fci a 12920400,20", confirm, "45", "12920400,20"},
		{"Ajusta de cuenta de mercado pago a \n111.208,07", confirm, "43", "111.208,07"},
		{"Corregir monto cuenta banco Galicia en dólares a $4350 USD", confirm, "53", "4350"},
		{"Actualizar saldo de Mercado pago a $891867.82", confirm, "43", "891867.82"},
		{"Ajustar saldo Mercado Pago a 1.200", confirm, "43", "1.200"},
		{"poné fci en 0", confirm, "45", "0"},
		{"Ajustar saldo fci al 18/09 a 5000", confirm, "45", "5000"},
		{"Cambiar saldo cuenta Cedears", askAmt, "51", ""},
		{"Ajustar el valor de la cuenta de jubilación", askAmt, "50", ""},
		{"Corregir saldo cuenta banco Galicia en pesos", askAmt, "52", ""},
		{"Ajusta saldo de la cuenta mercado pago en dólares", askAmt, "44", ""},

		{"Ajusta saldo de fci usd a $ 3,605.24", askAmt, "46", ""},
		{"Ajustar fci ars a 10,473,170.66", askAmt, "45", ""},
		{"dejá fci en 50 mil", askAmt, "45", ""},
		{"Ajustar saldo fci a 5000 o 6000", askAmt, "45", ""},
		{"Ajustar saldo mercado pago a -500", askAmt, "43", ""},

		{"Ajustar montos de cuentas", pick, "", ""},
		{"Ajustar balance de cuenta", pick, "", ""},

		{"Cambiar nombre de cuenta Jubilación", menu, "50", ""},
		{"Cambiar nombre a mi cuenta FCI", menu, "45", ""},
		{"Setear cuenta Mercado Pago en USD por defecto", menu, "44", ""},
		{"Quiero que fci en pesos argentinos sea mi nueva cuenta por defecto", menu, "45", ""},
		{"Nueva cuenta FCI en USD 3614,66", menu, "46", ""},
		{"Modificar cuenta fci en ars", menu, "45", ""},
		{"renombrá fci a fci 2", menu, "45", ""},
		{"Cambio de cuenta", pick, "", ""},
		{"Eliminar cuenta mercapago", pick, "", ""},
		{"Otro nombre mercadopago", pick, "", ""},
	}
	for _, c := range cases {
		t.Run(c.msg, func(t *testing.T) {
			store := startManage(t, c.msg, orchestrator.AccountManageResult{})
			if store.stepName != c.wantStep {
				t.Errorf("step = %q, want %q", store.stepName, c.wantStep)
			}
			if got := conversation.StringOrEmpty(store.data["account_id"]); got != c.wantAccID {
				t.Errorf("account_id = %q, want %q", got, c.wantAccID)
			}
			if got := conversation.StringOrEmpty(store.data["new_total"]); got != c.wantTotal {
				t.Errorf("new_total = %q, want %q", got, c.wantTotal)
			}
		})
	}
}

func TestStartAccountManage_SameNameSameCurrencyOpensThePicker(t *testing.T) {
	accs := []account.Account{acc(1, "Banco", currency.ARS), acc(2, "Banco", currency.ARS)}
	if id, ok := accountNamedInText("banco a 5000", accs); ok {
		t.Errorf("resolved to %d, want the picker", id)
	}
}

func TestStartAccountManage_ModelMatchIsStillHonoredWhenTheTextNamesNothing(t *testing.T) {
	id := uint64(47)
	store := startManage(t, "la de inversiones", orchestrator.AccountManageResult{MatchedAccountID: &id})
	if store.data["account_id"] != "47" {
		t.Errorf("account_id = %v, want 47", store.data["account_id"])
	}
}

func TestStartAccountManage_TotalIsNeverTakenFromTheAccountName(t *testing.T) {
	accs := []account.Account{acc(9, "Plazo fijo 2025", currency.ARS)}
	total, _ := statedTotal("ajustar saldo plazo fijo 2025", accs)
	if total != "" {
		t.Errorf("total = %q, want none: 2025 is part of the account name", total)
	}
}

func TestStatedTotal_ParsesToTheAmountTheUserMeant(t *testing.T) {
	cases := map[string]string{
		"Mercado Pago a $207555,66":             "207555.66",
		"MERCADO PAGO USD A 496,1":              "496.1",
		"fci a 12920400,20":                     "12920400.2",
		"mercado pago a \n111.208,07":           "111208.07",
		"Mercado pago a $891867.82":             "891867.82",
		"Mercado Pago a 1.200":                  "1200",
		"banco Galicia a $4350 USD":             "4350",
		"fci en u$s1.500":                       "1500",
		"poné fci en 0":                         "0",
		"dejá el saldo de fci en 10.473.170,66": "10473170.66",
	}
	for msg, want := range cases {
		total, _ := statedTotal(msg, prodLikeAccounts())
		got, err := movement.ParseARAmount(total)
		if err != nil || got.String() != want {
			t.Errorf("%q → %q → %v (err %v), want %s", msg, total, got, err, want)
		}
	}
}
