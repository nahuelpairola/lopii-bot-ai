package query

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

const (
	msgQueryNoRowsInRange = "sin movimientos en ese rango."

	msgSearchOutOfRangeFmt = "sin movimientos con «%s» entre %s y %s. " + msgOutOfRangeMark

	msgOutOfRangeMark = "Sí hay con ese texto en otras fechas."

	msgConsultedRangeFmt = "(consulté entre %s y %s)"

	msgSearchOnlyInternalFmt = "«%s» " + msgOnlyInternalMark + " —transferencias entre tus cuentas, saldos iniciales, ajustes—, que no entran en los totales de gastos e ingresos."

	msgOnlyInternalMark = "sólo aparece en movimientos internos"

	msgSearchNotFoundFmt = "no encontré nada que diga «%s»: no es una categoría, ni una subcategoría, ni aparece en ninguna descripción."
)

var (
	searchProbeFrom = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	searchProbeTo   = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
)

func describeEmptyResult(svc services, q movement.MovementQuery, args queryToolArgs) (string, error) {
	if q.Search == nil {
		return msgQueryNoRowsInRange, nil
	}
	term := *q.Search

	wide := q
	wide.From, wide.To = searchProbeFrom, searchProbeTo
	if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOutOfRangeFmt, term, args.From, args.To), nil
	}

	wide.OnlyReserved = true
	if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
	}

	if wide.Type == nil {
		t := constants.Transfer
		wide.Type = &t
		if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
			return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
		}
	}

	return "", fmt.Errorf(msgSearchNotFoundFmt, term)
}

func emptyResultNamesARange(out string) bool {
	return strings.Contains(out, msgQueryNoRowsInRange) || strings.Contains(out, msgOutOfRangeMark)
}

func appendConsultedRange(answer, from, to string) string {
	if from == "" || to == "" || strings.Contains(answer, from) {
		return answer
	}
	return strings.TrimSpace(answer) + "\n" + fmt.Sprintf(msgConsultedRangeFmt, from, to)
}

func friendlyDate(iso string) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		return iso
	}
	return t.Format("02/01/2006")
}

func reinstateAppVerdict(answer, verdict string) string {
	if verdict == "" || strings.Contains(strings.ToLower(answer), "internos") {
		return answer
	}
	return strings.TrimSpace(answer) + "\n\n" + verdict
}
