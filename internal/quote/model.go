package quote

import (
	"time"

	"github.com/shopspring/decimal"
)

// Quote es la cotización del dólar de una casa en una fecha. Sin created_at:
// es serie pública inmutable, no dato de usuario. La PK (date, casa) es lo que
// hace idempotente a toda escritura.
//
// OJO: la serie NO es estrictamente de días hábiles. La fuente arrastra el
// último valor a algunos fines de semana y a otros no (verificado 2026-08-06:
// el sábado 2026-08-01 no existe, el domingo 2026-08-02 sí). Los huecos son
// irregulares: quien lee resuelve con "<= fecha", nunca "= fecha", y nunca
// se rellenan del lado de la escritura.
type Quote struct {
	Date   time.Time       `gorm:"primaryKey;column:date"`
	Casa   string          `gorm:"primaryKey;column:casa"`
	Compra decimal.Decimal `gorm:"column:compra"`
	Venta  decimal.Decimal `gorm:"column:venta"`
}

func (Quote) TableName() string { return "usd_quotes" }

// CPI es la variación mensual del IPC en porcentaje (1.9 = 1.9%), no un número
// índice, y puede ser negativa. Month es el primer día del mes.
//
// El mes en curso NUNCA tiene IPC: INDEC publica ~2 semanas después del cierre.
type CPI struct {
	Month time.Time       `gorm:"primaryKey;column:month"`
	Value decimal.Decimal `gorm:"column:value"`
}

func (CPI) TableName() string { return "monthly_cpi" }
