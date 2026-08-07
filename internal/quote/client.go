package quote

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/constants"
)

// Las dos fuentes públicas, sin API key. argentinadatos tiene el histórico
// pero llega hasta AYER; dolarapi tiene el valor de hoy en vivo.
const (
	defaultArgentinaDatosURL = "https://api.argentinadatos.com"
	defaultDolarAPIURL       = "https://dolarapi.com"

	pathQuotesAll = "/v1/cotizaciones/dolares"
	pathCPI       = "/v1/finanzas/indices/inflacion"
	pathQuotesNow = "/v1/dolares"

	dateLayout  = "2006-01-02"
	pathLayout  = "2006/01/02"
	monthLayout = "2006-01"
)

type Config struct {
	ArgentinaDatosURL string
	DolarAPIURL       string
	TimeoutSeconds    int
}

type Client struct {
	cfg  Config
	http *http.Client
}

func NewClient(cfg Config) *Client {
	if cfg.ArgentinaDatosURL == "" {
		cfg.ArgentinaDatosURL = defaultArgentinaDatosURL
	}
	if cfg.DolarAPIURL == "" {
		cfg.DolarAPIURL = defaultDolarAPIURL
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second}}
}

// apiQuote es la forma que devuelven los tres endpoints de cotización. Los
// nombres son los de la fuente, en castellano, y mueren acá: toQuotes los
// traduce y del cliente para adentro todo es Quote. Sólo cambia el campo de
// fecha: argentinadatos manda `fecha` (2026-08-05) y dolarapi manda
// `fechaActualizacion` (RFC3339 UTC).
type apiQuote struct {
	Casa               string          `json:"casa"`
	Compra             decimal.Decimal `json:"compra"`
	Venta              decimal.Decimal `json:"venta"`
	Fecha              string          `json:"fecha"`
	FechaActualizacion string          `json:"fechaActualizacion"`
}

type apiCPI struct {
	Fecha string          `json:"fecha"`
	Valor decimal.Decimal `json:"valor"`
}

// get decodifica en out. notFound sale true en un 404, que para el endpoint
// por fecha significa "ese día no cotizó", no un fallo.
func (c *Client) get(url string, out any) (notFound bool, err error) {
	resp, err := c.http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("quote: %s devolvió %d", url, resp.StatusCode)
	}
	return false, json.NewDecoder(resp.Body).Decode(out)
}

// FetchAll trae la serie completa desde 2011 (~2.9 MB). Se usa una sola vez,
// cuando la tabla está vacía.
func (c *Client) FetchAll() ([]Quote, error) {
	var raw []apiQuote
	if _, err := c.get(c.cfg.ArgentinaDatosURL+pathQuotesAll, &raw); err != nil {
		return nil, err
	}
	return toQuotes(raw, ""), nil
}

// FetchDate trae todas las casas de un día pasado. Un 404 devuelve (nil, nil):
// ese día no tiene cotización y nunca la va a tener.
func (c *Client) FetchDate(d time.Time) ([]Quote, error) {
	url := fmt.Sprintf("%s%s/%s", c.cfg.ArgentinaDatosURL, pathQuotesAll, d.Format(pathLayout))
	var raw []apiQuote
	notFound, err := c.get(url, &raw)
	if err != nil || notFound {
		return nil, err
	}
	return toQuotes(raw, ""), nil
}

// FetchToday trae el valor en vivo. dolarapi no manda fecha calendaria sino un
// timestamp de actualización, así que la fecha se sella con el día ART de now.
func (c *Client) FetchToday(now time.Time) ([]Quote, error) {
	var raw []apiQuote
	if _, err := c.get(c.cfg.DolarAPIURL+pathQuotesNow, &raw); err != nil {
		return nil, err
	}
	return toQuotes(raw, now.In(constants.ArgentinaZone).Format(dateLayout)), nil
}

func (c *Client) FetchCPI() ([]CPI, error) {
	var raw []apiCPI
	if _, err := c.get(c.cfg.ArgentinaDatosURL+pathCPI, &raw); err != nil {
		return nil, err
	}
	cs := make([]CPI, 0, len(raw))
	for _, r := range raw {
		// La API manda el último día del mes; la PK es el primero, así que la
		// clave se deriva de cualquier fecha sin saber cuánto dura el mes.
		t, err := time.ParseInLocation(dateLayout, r.Fecha, constants.ArgentinaZone)
		if err != nil {
			continue
		}
		month, err := time.ParseInLocation(monthLayout, t.Format(monthLayout), constants.ArgentinaZone)
		if err != nil {
			continue
		}
		cs = append(cs, CPI{Month: month, Value: r.Valor})
	}
	return cs, nil
}

// toQuotes convierte la respuesta cruda. forceDate, cuando no está vacío, pisa
// la fecha de cada fila — es el caso de dolarapi, que no manda una.
func toQuotes(raw []apiQuote, forceDate string) []Quote {
	qs := make([]Quote, 0, len(raw))
	for _, r := range raw {
		ds := r.Fecha
		if forceDate != "" {
			ds = forceDate
		}
		d, err := time.ParseInLocation(dateLayout, ds, constants.ArgentinaZone)
		if err != nil {
			continue // una fila ilegible no tira abajo el resto del lote
		}
		qs = append(qs, Quote{Date: d, RateType: r.Casa, Bid: r.Compra, Ask: r.Venta})
	}
	return qs
}
