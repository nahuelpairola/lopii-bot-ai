package orchestrator

const numberFormatRule = `
REGLA DE FORMATO NUMÉRICO (montos):
- El usuario escribe en convención argentina: el punto "." separa miles y la coma "," separa decimales. El monto puede venir con "$" o espacios.
- SIEMPRE devolvé el amount como un número plano parseable: punto "." como separador decimal, SIN separadores de miles, SIN "$", SIN espacios.
- Si hay una coma, la coma es el decimal y todos los puntos son miles: "1.500,50" se normaliza a "1500.50".
- Si NO hay coma y hay un punto seguido de exactamente 3 dígitos, ese punto es separador de miles: "1.500" se normaliza a "1500", "1.041.265" se normaliza a "1041265".
- Si NO hay coma y hay un punto seguido de 1 o 2 dígitos, ese punto es decimal: "1.5" se normaliza a "1.5".
- Las abreviaturas se expanden a su valor completo: "200k" se normaliza a "200000", "1.5m" se normaliza a "1500000".
Ejemplos: "$1.041.265" se normaliza a "1041265"; "1.500,50" se normaliza a "1500.50"; "2.000" se normaliza a "2000".
`
