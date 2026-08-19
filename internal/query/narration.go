// La narración del vacío: qué le dice la app al usuario cuando la consulta no
// devolvió filas, y qué le repone a la respuesta del modelo después.
//
// Vive aparte de query.go porque es el bloque que más se toca —los arreglos de
// narración caen todos acá— y porque arrastra un defecto abierto (la narración
// forzada que vuelve vacía, ver el comentario en query_eval_test.go).
package query

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

const (
	// Sin search, un cero es un cero honesto: no hubo movimientos en ese rango.
	// No hay filtro de texto que pueda no haber matcheado.
	msgQueryNoRowsInRange = "sin movimientos en ese rango."

	// El término existe en los datos del usuario, pero no en el rango pedido.
	// Es una AUSENCIA VERIFICADA: acá el modelo sí puede decir que no gastó.
	msgSearchOutOfRangeFmt = "sin movimientos con «%s» entre %s y %s. " + msgOutOfRangeMark

	// msgOutOfRangeMark identifica el mensaje anterior a la vuelta, para reponer
	// el rango que el modelo casi siempre tira. Const separada por lo mismo que
	// msgOnlyInternalMark: se usa para armarlo y para reconocerlo.
	msgOutOfRangeMark = "Sí hay con ese texto en otras fechas."

	// msgConsultedRangeFmt es la nota al pie que la app le agrega a una respuesta
	// que salió vacía. No la escribe el modelo: es la ventana que la app CONSULTÓ
	// de verdad, y es lo único que delata un año mal resuelto.
	msgConsultedRangeFmt = "(consulté entre %s y %s)"

	// El término sólo matchea movimientos de categorías reservadas, que apply()
	// esconde de todo total de gastos e ingresos. Sin este mensaje la app diría
	// que "transferencia" no existe, sobre 12 movimientos reales.
	msgSearchOnlyInternalFmt = "«%s» " + msgOnlyInternalMark + " —transferencias entre tus cuentas, saldos iniciales, ajustes—, que no entran en los totales de gastos e ingresos."

	// msgOnlyInternalMark es la parte del mensaje anterior que lo identifica, y
	// existe como const separada porque se usa DOS veces: para armarlo y para
	// reconocerlo a la vuelta en reinstateAppVerdict.
	msgOnlyInternalMark = "sólo aparece en movimientos internos"

	// El término no matchea NADA. Es lo único que habilita decir que no existe,
	// y va como ERROR para que el modelo lo pueda corregir en la ronda siguiente:
	// AnswerQuery reinyecta los errores del ejecutor en vez de abortar.
	msgSearchNotFoundFmt = "no encontré nada que diga «%s»: no es una categoría, ni una subcategoría, ni aparece en ninguna descripción."
)

// searchProbeFrom/To es el rango "todo el historial" de las sondas. Fechas fijas
// y absurdamente anchas a propósito: la sonda contesta "¿existe este término en
// algún lado?", y una ventana relativa a hoy haría que la respuesta cambiara
// sola con el paso del tiempo.
var (
	searchProbeFrom = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	searchProbeTo   = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
)

// describeEmptyResult decide qué decir cuando la consulta no devolvió filas.
// Devuelve (mensaje, nil) o ("", error) — el error es "el término no existe".
//
// Las sondas corren SÓLO acá, o sea sólo cuando el resultado ya vino vacío, son
// LIMIT 1, y cuestan cero tokens: son consultas a Postgres, no llamadas a Groq.
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

	// Sonda 2: las reservadas. apply() las excluye siempre salvo que se pidan, y
	// el ejecutor de query nunca las pide, así que la sonda 1 las esconde igual
	// que la consulta real. Sin esto, "transferencia" y "saldo inicial" —12 y 4
	// movimientos reales en la base local— se declararían inexistentes.
	wide.OnlyReserved = true
	if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
		return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
	}

	// Segunda pasada de la sonda 2, forzando el tipo. Las reservadas que MÁS
	// importan —Sistema | Transferencia y los saldos iniciales— son todas
	// type=transfer, y con Type nil apply agrega `type <> transfer`: las esconde
	// justo cuando hacen falta. Medido el 2026-08-14 contra la base: la sonda con
	// type=transfer encuentra 12 filas y la misma sonda con Type nil encuentra 0,
	// y esas 12 se declaraban inexistentes.
	//
	// Va como segunda consulta y no reemplazando a la de arriba porque las otras
	// reservadas —Ajuste de saldo, Rendimiento inversión— NO son transferencias:
	// una sola pasada, con o sin tipo, siempre deja afuera la mitad.
	if wide.Type == nil {
		t := constants.Transfer
		wide.Type = &t
		if rows, err := svc.QueryListMovements(wide, 1); err == nil && len(rows) > 0 {
			return fmt.Sprintf(msgSearchOnlyInternalFmt, term), nil
		}
	}

	return "", fmt.Errorf(msgSearchNotFoundFmt, term)
}

// reinstateAppVerdict devuelve la respuesta con el veredicto de la app pegado atrás,
// si el modelo lo perdió por el camino.
//
// Existe porque el 2026-08-14, en producción, el modelo INVIRTIÓ el veredicto: el
// ejecutor le entregó "«transferencia» sólo aparece en movimientos internos…" —con 12
// filas reales detrás, verificadas por la sonda— y el usuario leyó "No se encontraron
// movimientos que digan transferencia en agosto de 2026". No es un matiz perdido: es la
// afirmación contraria a la que hizo la app.
//
// Este es el único mensaje que se reinstala, y no todos, porque es el único donde el
// modelo puede leer un resultado vacío y concluir lo opuesto a lo que dice el texto. Los
// otros tres describen ausencias de verdad: si los aplasta, empobrece la respuesta pero
// no la vuelve falsa.
//
// Se pega SÓLO si la respuesta no habla ya de movimientos internos, para no repetir lo
// que el modelo sí supo decir.
// emptyResultNamesARange dice si un resultado del ejecutor es uno de los vacíos
// cuyo rango vale la pena reponer.
//
// Son dos de los cuatro desenlaces, los que hablan de una VENTANA: la app miró un
// período concreto y no encontró nada, así que el período es el dato sospechoso.
// Los otros dos son hechos sobre el TÉRMINO —"sólo aparece en movimientos
// internos", "no encontré nada que diga X"—, verdaderos en cualquier rango, y
// reponerles una ventana sólo agregaría ruido.
func emptyResultNamesARange(out string) bool {
	return strings.Contains(out, msgQueryNoRowsInRange) || strings.Contains(out, msgOutOfRangeMark)
}

// appendConsultedRange le pega a la respuesta la ventana que la app consultó,
// cuando la consulta volvió vacía.
//
// Una consulta que sale vacía porque el modelo resolvió mal el año es INVISIBLE:
// el ejecutor dice "sin movimientos con «Supermercado» entre 2024-08-01 y
// 2024-08-31", el modelo redacta "no gastaste en Supermercado", y el rango —el
// único dato que delata el error— no llega nunca al usuario. Esto no previene la
// resolución equivocada; la hace visible en el acto.
//
// Va como nota al pie propia de la app y no reponiendo el mensaje crudo del
// ejecutor, por dos razones: el mensaje crudo trae las fechas en ISO, que el
// prompt le prohíbe mostrar al modelo y por lo tanto la app tampoco puede colar
// por atrás; y una línea corta no compite con la respuesta que el usuario pidió.
func appendConsultedRange(answer, from, to string) string {
	if from == "" || to == "" || strings.Contains(answer, from) {
		return answer
	}
	return strings.TrimSpace(answer) + "\n" + fmt.Sprintf(msgConsultedRangeFmt, from, to)
}

// friendlyDate pasa una fecha ISO al formato argentino. Lo que no parsea vuelve
// tal cual: el rango es informativo, y nunca vale romper una respuesta que ya
// está lista por una fecha rara.
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
