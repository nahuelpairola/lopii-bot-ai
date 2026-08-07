package quote

import (
	"time"

	"github.com/shopspring/decimal"
)

// Quote es la cotización del dólar de un tipo de cambio en una fecha. Sin
// created_at: es serie pública inmutable, no dato de usuario. La PK
// (date, rate_type) es lo que hace idempotente a toda escritura.
//
// RateType es el `casa` de la API: oficial, blue, bolsa (MEP),
// contadoconliqui, mayorista, cripto, tarjeta.
//
// Bid/Ask son su `compra`/`venta`, desde la óptica de la casa de cambio: te
// compra los dólares al Bid y te los vende al Ask, así que normalmente
// Bid <= Ask. Quien convierta ARS->USD divide por el Ask; USD->ARS multiplica
// por el Bid.
//
// "Normalmente" porque la fuente publica alguna fila con el spread dado vuelta
// (verificado: mayorista 2026-01-13 trae compra=1466 venta=1457, así, de
// origen). Se guarda tal cual, igual que los huecos: no se corrige del lado de
// la escritura. Quien calcule un spread se banca que pueda dar negativo.
//
// OJO: la serie NO es estrictamente de días hábiles. La fuente arrastra el
// último valor a algunos fines de semana y a otros no (verificado 2026-08-07
// contra la base: el sábado 2026-08-01 no existe, el domingo 2026-08-02 sí).
// Los huecos son irregulares: quien lee resuelve con "<= fecha", nunca
// "= fecha", y nunca se rellenan del lado de la escritura.
type Quote struct {
	Date     time.Time       `gorm:"primaryKey;column:date"`
	RateType string          `gorm:"primaryKey;column:rate_type"`
	Bid      decimal.Decimal `gorm:"column:bid"`
	Ask      decimal.Decimal `gorm:"column:ask"`
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
